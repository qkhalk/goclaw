package browser

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/nextlevelbuilder/goclaw/internal/media"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// download.go — CDP-based file downloads for the multiplexed browser tool.
//
// The `download` action points a tenant-scoped page at a URL that triggers a
// browser download (Content-Disposition attachment, blob URL, etc.). Chrome is
// told via Browser.setDownloadBehavior to save into a fresh per-download
// staging directory; a polling loop watches the directory for the partial
// (.crdownload) file to settle and returns the staged file path so the caller
// (tool.go) can move it into the session media store.
//
// The polling approach is deliberately event-free: download progress events
// are routed differently across Chrome variants and are unreliable on the
// Lightpanda backend, while on-disk state is the one signal every backend
// shares. The real-download e2e path stays manual (no live browser in CI) —
// see download_test.go for the covered pure logic.

const (
	// DefaultDownloadTimeout bounds the whole download (navigate → settle).
	DefaultDownloadTimeout = 60 * time.Second
	// DefaultDownloadMaxBytes caps accepted downloads (50MB).
	DefaultDownloadMaxBytes = int64(50) << 20
	// downloadPollInterval is the staging-dir scan cadence.
	downloadPollInterval = 300 * time.Millisecond
	// downloadTriggerGrace is how long we wait for ANY file to appear before
	// concluding the URL rendered a normal page instead of a download.
	downloadTriggerGrace = 10 * time.Second
	// downloadStableScans is the number of consecutive unchanged scans a file
	// needs before it is considered completely written.
	downloadStableScans = 2
	// crdownloadSuffix marks Chrome partial downloads.
	crdownloadSuffix = ".crdownload"
)

// DownloadOpts configures a single Download call.
type DownloadOpts struct {
	// Timeout bounds the whole download. Defaults to DefaultDownloadTimeout.
	Timeout time.Duration
	// MaxBytes rejects downloads larger than this. Defaults to
	// DefaultDownloadMaxBytes; <= 0 means default, negative values are
	// clamped. Explicit 0 is treated as default (a 0-byte cap is useless).
	MaxBytes int64
}

// withDefaults returns a copy with zero values resolved.
func (o DownloadOpts) withDefaults() DownloadOpts {
	if o.Timeout <= 0 {
		o.Timeout = DefaultDownloadTimeout
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultDownloadMaxBytes
	}
	return o
}

// DownloadResult describes a finished download still in its staging dir.
type DownloadResult struct {
	// FilePath is the absolute path of the downloaded file inside the
	// staging directory.
	FilePath string
	// FileName is the base name Chrome saved the file under.
	FileName string
	// Bytes is the final file size.
	Bytes int64
}

// downloadBehavior builds the CDP call that directs Chrome to save downloads
// into dir. Behavior "allow" keeps original filenames (agents rely on the
// extension for mime detection); BrowserContextID scopes the override to the
// tenant's incognito context (empty = default context, lightpanda conns are
// their own browser).
func downloadBehavior(browserCtxID proto.BrowserBrowserContextID, dir string) proto.BrowserSetDownloadBehavior {
	return proto.BrowserSetDownloadBehavior{
		Behavior:         proto.BrowserSetDownloadBehaviorBehaviorAllow,
		BrowserContextID: browserCtxID,
		DownloadPath:     dir,
	}
}

// resetDownloadBehavior restores default download handling for the context.
func resetDownloadBehavior(browserCtxID proto.BrowserBrowserContextID) proto.BrowserSetDownloadBehavior {
	return proto.BrowserSetDownloadBehavior{
		Behavior:         proto.BrowserSetDownloadBehaviorBehaviorDefault,
		BrowserContextID: browserCtxID,
	}
}

// stagingScan is one snapshot of the staging directory contents.
type stagingScan struct {
	// sizes maps file name → current size (partial files included).
	sizes map[string]int64
	// active is true while any *.crdownload partial exists.
	active bool
	// totalBytes sums all file sizes (used for the max-size guard).
	totalBytes int64
}

// scanStaging reads the staging directory. A missing directory yields an
// empty scan (Chrome creates files lazily after the response headers arrive).
func scanStaging(dir string) (*stagingScan, error) {
	scan := &stagingScan{sizes: make(map[string]int64)}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return scan, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // raced with Chrome rename — skip this cycle
		}
		name := e.Name()
		scan.sizes[name] = info.Size()
		scan.totalBytes += info.Size()
		if strings.HasSuffix(strings.ToLower(name), crdownloadSuffix) {
			scan.active = true
		}
	}
	return scan, nil
}

// settledFiles returns complete (non-partial) files whose size stayed
// unchanged for two consecutive scans, keyed by name with their size.
func (s *stagingScan) settledFiles(prev *stagingScan) map[string]int64 {
	settled := make(map[string]int64)
	if prev == nil {
		return settled
	}
	for name, size := range s.sizes {
		if strings.HasSuffix(strings.ToLower(name), crdownloadSuffix) {
			continue
		}
		if prevSize, ok := prev.sizes[name]; ok && prevSize == size && size >= 0 {
			settled[name] = size
		}
	}
	return settled
}

// largestFile picks the biggest entry of a name→size map (deterministic tiebreak
// by name). Returns empty string when the map is empty.
func largestFile(files map[string]int64) (string, int64) {
	bestName, bestSize := "", int64(-1)
	for name, size := range files {
		if size > bestSize || (size == bestSize && name < bestName) {
			bestName, bestSize = name, size
		}
	}
	return bestName, bestSize
}

// safeStagedFile joins dir and name, rejecting names that escape the staging
// directory. Chrome sanitizes suggested filenames itself; this is defense in
// depth before the file is handed to the media store.
func safeStagedFile(dir, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("browser download: empty filename")
	}
	cleanDir := filepath.Clean(dir)
	p := filepath.Clean(filepath.Join(cleanDir, name))
	if p != cleanDir && !strings.HasPrefix(p, cleanDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("browser download: filename %q escapes staging dir", name)
	}
	return p, nil
}

// Download navigates the given tab to url, captures the browser-triggered
// download into a fresh staging directory, and waits for the file to settle.
// The caller owns staging cleanup after moving the file into persistent
// storage (see tool.go handleDownload).
func (m *Manager) Download(ctx context.Context, targetID, rawURL string, opts DownloadOpts) (*DownloadResult, error) {
	opts = opts.withDefaults()

	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("download requires a valid http(s) URL, got %q", rawURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("download URL is missing a host: %q", rawURL)
	}

	tenantID := tenantIDFromCtx(ctx)
	m.mu.Lock()
	if !m.isRunningLocked() {
		m.mu.Unlock()
		return nil, fmt.Errorf("browser not running")
	}
	page, err := m.getPageForTenant(targetID, tenantID)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	// Downloads are saved through the browser-level CDP call. Tenant-scoped
	// Chrome pages override their own incognito context; Lightpanda tabs are
	// per-connection browsers, so the conn itself is the target.
	var behaviorTarget *rod.Browser
	var ctxID proto.BrowserBrowserContextID
	if m.backend == BackendLightpanda {
		conn, ok := m.pageConns[targetID]
		if !ok {
			m.mu.Unlock()
			return nil, fmt.Errorf("lightpanda connection for tab %s not found", targetID)
		}
		behaviorTarget = conn
	} else {
		tb, terr := m.tenantBrowserLocked(tenantID)
		if terr != nil {
			m.mu.Unlock()
			return nil, terr
		}
		behaviorTarget = tb
		ctxID = tb.BrowserContextID
	}
	m.mu.Unlock()

	staging, err := os.MkdirTemp("", "goclaw-dl-")
	if err != nil {
		return nil, fmt.Errorf("create download staging dir: %w", err)
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(staging)
		}
	}()

	// Scope downloads of this context into the staging dir for the duration.
	if err := downloadBehavior(ctxID, staging).Call(behaviorTarget); err != nil {
		return nil, fmt.Errorf("set download behavior: %w", err)
	}
	defer func() {
		if rerr := resetDownloadBehavior(ctxID).Call(behaviorTarget); rerr != nil {
			m.logger.Warn("browser download: reset behavior failed", "error", rerr)
		}
	}()

	// Watchdog: close the page if ctx is cancelled mid-navigation so pending
	// CDP calls unblock (same pattern as Navigate).
	stop := watchPageClose(ctx, page)
	defer stop()

	if err := page.Navigate(rawURL); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("navigate for download: %w", err)
	}

	res, err := m.awaitDownload(ctx, staging, opts)
	if err != nil {
		return nil, err
	}
	cleanupStaging = false // file handed to the caller
	return res, nil
}

// awaitDownload polls the staging directory until a download completes,
// errors out, or the deadline hits. Split from Download for testability.
func (m *Manager) awaitDownload(ctx context.Context, staging string, opts DownloadOpts) (*DownloadResult, error) {
	deadline := time.Now().Add(opts.Timeout)
	grace := time.Now().Add(downloadTriggerGrace)

	var prev *stagingScan
	ticker := time.NewTicker(downloadPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}

		scan, err := scanStaging(staging)
		if err != nil {
			return nil, fmt.Errorf("scan staging dir: %w", err)
		}
		if scan.totalBytes > opts.MaxBytes {
			return nil, fmt.Errorf("download exceeds max size (%d > %d bytes)", scan.totalBytes, opts.MaxBytes)
		}

		if settled := scan.settledFiles(prev); len(settled) > 0 {
			name, size := largestFile(settled)
			path, serr := safeStagedFile(staging, name)
			if serr != nil {
				return nil, serr
			}
			if size > opts.MaxBytes {
				return nil, fmt.Errorf("download exceeds max size (%d > %d bytes)", size, opts.MaxBytes)
			}
			m.logger.Debug("browser download settled", "file", name, "bytes", size)
			return &DownloadResult{FilePath: path, FileName: name, Bytes: size}, nil
		}

		if len(scan.sizes) == 0 && time.Now().After(grace) {
			return nil, fmt.Errorf("URL did not trigger a download; use navigate/snapshot for regular pages")
		}
		prev = scan

		if time.Now().After(deadline) {
			if scan.active {
				return nil, fmt.Errorf("download timed out after %s (still in progress)", opts.Timeout)
			}
			return nil, fmt.Errorf("download timed out after %s", opts.Timeout)
		}
	}
}

// CleanupStaging removes a staging directory left behind by Download when the
// caller did not stage the file anywhere (error paths, fallbacks).
func CleanupStaging(result *DownloadResult) {
	if result == nil || result.FilePath == "" {
		return
	}
	dir := filepath.Dir(result.FilePath)
	if base := filepath.Base(dir); strings.HasPrefix(base, "goclaw-dl-") { // only our temp dirs
		_ = os.RemoveAll(dir)
	}
}

// StageDownloadedFile moves a finished download into the session media store
// rooted at <workspace>/.media (same layout the gateway uses), returning the
// persistent path. When no workspace is configured the original staging path
// is returned unchanged and the staging dir is left for the agent to read;
// the caller must not remove it in that case.
func StageDownloadedFile(ctx context.Context, result *DownloadResult) string {
	if result == nil || result.FilePath == "" {
		return ""
	}
	ws := tools.ToolWorkspaceFromCtx(ctx)
	if ws == "" {
		return result.FilePath
	}
	// Same root the gateway's media store uses (gateway_managed.go) — safe to
	// re-open: NewStore only MkdirAlls an existing directory.
	store, err := media.NewStore(filepath.Join(ws, ".media"))
	if err != nil {
		slog.Warn("browser download: media store unavailable, keeping staging path", "error", err)
		return result.FilePath
	}
	mimeType := mimeTypeForFile(result.FileName)
	id, dst, err := store.SaveFile(tools.ToolSessionKeyFromCtx(ctx), result.FilePath, mimeType)
	if err != nil {
		slog.Warn("browser download: staging into media store failed", "error", err)
		return result.FilePath
	}
	slog.Debug("browser download staged", "media_id", id, "path", dst)
	return dst
}

// mimeTypeForFile guesses the mime from the file extension, defaulting to
// application/octet-stream (media.Store falls back to the source extension
// when the mime has no known mapping). The local table runs first so results
// are deterministic across platforms; the OS mime registry is the fallback.
func mimeTypeForFile(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != "" {
		if m := defaultMimeByExt(ext); m != "" {
			return m
		}
		if m := mime.TypeByExtension(ext); m != "" {
			return m
		}
	}
	return "application/octet-stream"
}

// defaultMimeByExt covers common download extensions; kept local so the
// browser package does not depend on a mime registry at init time.
func defaultMimeByExt(ext string) string {
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".gz":
		return "application/gzip"
	case ".tar":
		return "application/x-tar"
	case ".csv":
		return "text/csv"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return ""
	}
}
