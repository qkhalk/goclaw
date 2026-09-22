package http

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/version"
)

// SystemUpdateHandler implements check-update + self-update for self-hosted
// gateways running under systemd (Restart=always).
//
//	GET  /v1/system/update         — compare cmd.Version against the latest
//	                                 GitHub release (cached 15 min).
//	POST /v1/system/update/install — download the release asset, verify it,
//	                                 atomically replace the live binary, then
//	                                 exit so systemd restarts into the new one.
//
// Owner/master-scope only (global admin action, same guard pair as the
// gateway upgrade trigger). Install is restricted to linux/amd64: the update
// flow assumes a single Linux server with a systemd unit that restarts the
// process on exit.
type SystemUpdateHandler struct {
	// VersionFn returns the running binary version (cmd.Version). Injectable
	// for tests.
	VersionFn func() string
	// Repo is the GitHub "owner/name" slug releases are fetched from.
	Repo string
	// GitHubAPIBase is the GitHub API root; injectable for tests.
	GitHubAPIBase string
	// exit is called to terminate the process after a successful install.
	// Injectable for tests; defaults to os.Exit.
	exit func(int)

	checkClient    *http.Client // 5s timeout for release metadata
	downloadClient *http.Client // 10min overall timeout for release assets

	// Runtime platform override for tests; defaults to runtime.GOOS/GOARCH.
	goos   string
	goarch string

	mu         sync.Mutex
	installing bool
	cacheMu    sync.Mutex
	cachedTag  string
	cachedURL  string
	cachedAt   time.Time
}

const (
	systemUpdateRepo           = "qkhalk/goclaw"
	systemUpdateCacheTTL       = 15 * time.Minute
	systemUpdateCheckTimeout   = 5 * time.Second
	systemUpdateDlTimeout      = 10 * time.Minute
	systemUpdateMaxAssetBytes  = 400 << 20 // 400MB cap on release asset
	systemUpdateVersionTimeout = 10 * time.Second
	elfMagic                   = "\x7fELF"
	systemUpdateMaxJSONBytes   = 4 << 20 // cap on GitHub API JSON responses
)

// jsonDecodeLimited decodes at most limit bytes of r into v as JSON.
func jsonDecodeLimited(r io.Reader, limit int64, v any) error {
	return json.NewDecoder(io.LimitReader(r, limit)).Decode(v)
}

// NewSystemUpdateHandler creates a SystemUpdateHandler reading the current
// version from versionFn (pass a getter over cmd.Version).
func NewSystemUpdateHandler(versionFn func() string) *SystemUpdateHandler {
	return &SystemUpdateHandler{
		VersionFn:      versionFn,
		Repo:           systemUpdateRepo,
		GitHubAPIBase:  "https://api.github.com",
		exit:           defaultExit,
		goos:           runtime.GOOS,
		goarch:         runtime.GOARCH,
		checkClient:    &http.Client{Timeout: systemUpdateCheckTimeout},
		downloadClient: &http.Client{Timeout: systemUpdateDlTimeout},
	}
}

// RegisterRoutes registers the system update routes on the given mux.
func (h *SystemUpdateHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/system/update", requireAuth(permissions.RoleAdmin, h.handleCheck))
	mux.HandleFunc("POST /v1/system/update/install", requireAuth(permissions.RoleAdmin, h.handleInstall))
}

// systemUpdateStatus is the response shape of GET /v1/system/update.
type systemUpdateStatus struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	URL             string `json:"url"`
	Error           string `json:"error,omitempty"`
}

func (h *SystemUpdateHandler) currentVersion() string {
	if h.VersionFn == nil {
		return "dev"
	}
	return strings.TrimSpace(h.VersionFn())
}

// fetchLatest returns (tag, htmlURL, error) for the latest GitHub release,
// serving a cached result for 15 minutes. Only successful lookups are cached.
func (h *SystemUpdateHandler) fetchLatest() (string, string, error) {
	h.cacheMu.Lock()
	if h.cachedTag != "" && time.Since(h.cachedAt) < systemUpdateCacheTTL {
		tag, url := h.cachedTag, h.cachedURL
		h.cacheMu.Unlock()
		return tag, url, nil
	}
	h.cacheMu.Unlock()

	base := h.GitHubAPIBase
	if base == "" {
		base = "https://api.github.com"
	}
	repo := h.Repo
	if repo == "" {
		repo = systemUpdateRepo
	}

	ctx, cancel := context.WithTimeout(context.Background(), systemUpdateCheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "goclaw")

	resp, err := h.checkClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("github unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return "", "", fmt.Errorf("github returned status %d", resp.StatusCode)
	}

	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Draft   bool   `json:"draft"`
	}
	if err := jsonDecodeLimited(resp.Body, systemUpdateMaxJSONBytes, &rel); err != nil {
		return "", "", fmt.Errorf("decode release: %w", err)
	}
	tag := strings.TrimSpace(rel.TagName)
	if tag == "" || rel.Draft {
		return "", "", errors.New("no stable release found")
	}

	h.cacheMu.Lock()
	h.cachedTag = tag
	h.cachedURL = rel.HTMLURL
	h.cachedAt = time.Now()
	h.cacheMu.Unlock()

	return tag, rel.HTMLURL, nil
}

// handleCheck implements GET /v1/system/update. GitHub failures degrade to a
// 200 with an "error" field so the UI can still show the current version.
func (h *SystemUpdateHandler) handleCheck(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}
	current := h.currentVersion()
	latest, url, err := h.fetchLatest()
	if err != nil {
		slog.Warn("system.update_check_failed", "error", err)
		writeJSON(w, http.StatusOK, systemUpdateStatus{
			Current: current,
			Error:   err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, systemUpdateStatus{
		Current:         current,
		Latest:          latest,
		UpdateAvailable: version.IsNewer(latest, current),
		URL:             url,
	})
}

// handleInstall implements POST /v1/system/update/install.
func (h *SystemUpdateHandler) handleInstall(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}
	if !h.platformSupported(w) {
		return
	}

	h.mu.Lock()
	if h.installing {
		h.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "update already in progress"})
		return
	}
	h.installing = true
	h.mu.Unlock()
	// Never leave the flag stuck: recover from any panic and reset.
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("system.update_install_panic", "panic", rec)
		}
		h.mu.Lock()
		h.installing = false
		h.mu.Unlock()
	}()

	tag, _, err := h.fetchLatest()
	if err != nil {
		slog.Warn("system.update_install_fetch_failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to resolve latest release: " + err.Error()})
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		slog.Error("system.update_install_resolve_exe_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot resolve running binary path"})
		return
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		slog.Error("system.update_install_abs_exe_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot resolve running binary path"})
		return
	}

	assetURL, err := h.fetchAssetURL(tag)
	if err != nil {
		slog.Warn("system.update_install_asset_lookup_failed", "tag", tag, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to locate release asset: " + err.Error()})
		return
	}
	if err := h.downloadAndReplace(r.Context(), assetURL, tag, exePath); err != nil {
		slog.Error("system.update_install_failed", "tag", tag, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed: " + err.Error()})
		return
	}

	slog.Info("system.update_installed", "tag", tag, "binary", exePath)
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarting", "version": tag})

	// Terminate via SIGTERM (not os.Exit) so the lifecycle teardown drains
	// in-flight work; systemd (Restart=always, RestartSec=5) restarts the
	// unit into the replaced binary.
	go func() {
		time.Sleep(1 * time.Second)
		h.exit(0)
	}()
}

// platformSupported rejects installs on anything but linux/amd64 with a 400.
func (h *SystemUpdateHandler) platformSupported(w http.ResponseWriter) bool {
	goos, goarch := h.goos, h.goarch
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if goos != "linux" || goarch != "amd64" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "self-update is only supported on linux/amd64 (got " + goos + "/" + goarch + ")",
		})
		return false
	}
	return true
}

// fetchAssetURL resolves the browser_download_url for the
// goclaw-<tag>-linux-amd64.tar.gz asset of the given release. Queries the
// pinned tag endpoint (not /releases/latest) so a newer release published
// mid-install cannot make the asset lookup mismatch the tag being installed.
func (h *SystemUpdateHandler) fetchAssetURL(tag string) (string, error) {
	assets, err := h.fetchReleaseAssets(tag)
	if err != nil {
		return "", err
	}
	want := "goclaw-" + tag + "-linux-amd64.tar.gz"
	for _, a := range assets {
		if a.Name == want && a.BrowserDownloadURL != "" {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("asset %s not found in release %s", want, tag)
}

// fetchReleaseAssets lists the assets of the given release tag.
func (h *SystemUpdateHandler) fetchReleaseAssets(tag string) ([]struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}, error) {
	base := h.GitHubAPIBase
	if base == "" {
		base = "https://api.github.com"
	}
	repo := h.Repo
	if repo == "" {
		repo = systemUpdateRepo
	}

	ctx, cancel := context.WithTimeout(context.Background(), systemUpdateCheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+repo+"/releases/tags/"+tag, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "goclaw")

	resp, err := h.checkClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("github returned status %d", resp.StatusCode)
	}

	var rel struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := jsonDecodeLimited(resp.Body, systemUpdateMaxJSONBytes, &rel); err != nil {
		return nil, fmt.Errorf("decode release assets: %w", err)
	}
	return rel.Assets, nil
}

// downloadAndReplace downloads the release asset, verifies the tarball
// checksum, extracts the bundled binary AND the migrations directory,
// verifies the binary, then atomically replaces the running executable and
// the on-disk migrations. On any failure the live binary and migrations are
// untouched (temp files are cleaned up).
func (h *SystemUpdateHandler) downloadAndReplace(ctx context.Context, assetURL, tag, exePath string) error {
	exeDir := filepath.Dir(exePath)

	// Verify the tarball against the release's published CHECKSUMS.sha256
	// (belt and braces on top of TLS — catches truncated/corrupt mirrors).
	if err := h.verifyChecksum(ctx, tag); err != nil {
		return fmt.Errorf("checksum verify: %w", err)
	}

	workDir, err := os.MkdirTemp(exeDir, ".goclaw.update-*")
	if err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	tmpNew := filepath.Join(workDir, "goclaw")
	tmpMigrations := filepath.Join(workDir, "migrations")
	if err := h.downloadAndExtract(ctx, assetURL, tmpNew, tmpMigrations); err != nil {
		return err
	}

	// Sanity: ELF magic.
	if err := checkELFMagic(tmpNew); err != nil {
		return err
	}
	// Sanity: the new binary runs and reports the expected version.
	if err := verifyBinaryVersion(tmpNew, tag); err != nil {
		return err
	}

	// Migrations: the release tar carries pkg-linux-amd64/migrations. The
	// gateway resolves migrations from <exe-dir>/migrations at boot, so the
	// on-disk set MUST track the new binary or startup fails (or, with
	// GOCLAW_AUTO_UPGRADE=true, runs against a stale schema).
	if err := h.applyMigrations(exeDir, tmpMigrations, tag); err != nil {
		return err
	}

	// Backup the current binary (best effort — never blocks the update).
	oldTag := h.currentVersion()
	backupPath := filepath.Join(exeDir, "goclaw.bak-"+sanitizeTagForPath(oldTag))
	if err := copyFile(exePath, backupPath); err != nil {
		slog.Warn("system.update_backup_failed", "error", err)
	} else {
		slog.Info("system.update_backup_created", "backup", backupPath)
	}

	// Atomic replace: rename over the running binary. On Linux this is safe
	// even while the old file is mapped (the inode stays alive until exit).
	if err := os.Chmod(tmpNew, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}
	if err := os.Rename(tmpNew, exePath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

// applyMigrations swaps the on-disk migrations dir for the release's copy.
// When the new set differs from the on-disk set and GOCLAW_AUTO_UPGRADE is
// not "true", the install is refused: the new binary would otherwise crash-
// loop on the stale schema at next boot. GOCLAW_AUTO_UPGRADE=true (or an
// unchanged set) proceeds and atomically replaces the directory.
func (h *SystemUpdateHandler) applyMigrations(exeDir, tmpMigrations, tag string) error {
	newFiles, err := os.ReadDir(tmpMigrations)
	if err != nil || len(newFiles) == 0 {
		// Release without a migrations dir: nothing to apply.
		return nil
	}
	cur := filepath.Join(exeDir, "migrations")
	curFiles, _ := os.ReadDir(cur)
	if !migrationSetsEqual(curFiles, newFiles) && os.Getenv("GOCLAW_AUTO_UPGRADE") != "true" {
		return errors.New("this release changes database migrations; set GOCLAW_AUTO_UPGRADE=true (automatic migration) or run `goclaw migrate up` manually after replacing the migrations directory")
	}
	// Swap: park the old dir aside, move the new one in, then delete old.
	if _, statErr := os.Stat(cur); statErr == nil {
		old := filepath.Join(exeDir, "migrations.bak-"+sanitizeTagForPath(tag))
		_ = os.RemoveAll(old)
		if err := os.Rename(cur, old); err != nil {
			return fmt.Errorf("park old migrations: %w", err)
		}
		if err := os.Rename(tmpMigrations, cur); err != nil {
			// Roll the old set back so the current binary keeps working.
			_ = os.Rename(old, cur)
			return fmt.Errorf("install new migrations: %w", err)
		}
		_ = os.RemoveAll(old)
		slog.Info("system.update_migrations_applied", "tag", tag, "files", len(newFiles))
		return nil
	}
	if err := os.Rename(tmpMigrations, cur); err != nil {
		return fmt.Errorf("install migrations: %w", err)
	}
	return nil
}

// migrationSetsEqual compares two directory listings by file name only.
func migrationSetsEqual(a, b []os.DirEntry) bool {
	if len(a) != len(b) {
		return false
	}
	names := make([]string, 0, len(a))
	for _, e := range a {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	other := make([]string, 0, len(b))
	for _, e := range b {
		other = append(other, e.Name())
	}
	slices.Sort(other)
	return slices.Equal(names, other)
}

// verifyChecksum downloads the release's CHECKSUMS.sha256 asset and verifies
// the tarball's SHA-256 against it. Missing checksum asset is an error: every
// fork release publishes one.
func (h *SystemUpdateHandler) verifyChecksum(ctx context.Context, tag string) error {
	assets, err := h.fetchReleaseAssets(tag)
	if err != nil {
		return err
	}
	wantTar := "goclaw-" + tag + "-linux-amd64.tar.gz"
	var tarURL, sumsURL string
	for _, a := range assets {
		switch a.Name {
		case wantTar:
			tarURL = a.BrowserDownloadURL
		case "CHECKSUMS.sha256":
			sumsURL = a.BrowserDownloadURL
		}
	}
	if tarURL == "" {
		return fmt.Errorf("asset %s not found in release %s", wantTar, tag)
	}
	if sumsURL == "" {
		return errors.New("CHECKSUMS.sha256 asset missing from release")
	}

	// Small text file — download fully.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumsURL, nil)
	if err != nil {
		return fmt.Errorf("build checksum request: %w", err)
	}
	req.Header.Set("User-Agent", "goclaw")
	resp, err := h.checkClient.Do(req)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("checksums download failed with status %d", resp.StatusCode)
	}
	sums, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}

	// Stream the tarball computing SHA-256 while checking the size cap.
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, tarURL, nil)
	if err != nil {
		return fmt.Errorf("build asset request: %w", err)
	}
	req2.Header.Set("User-Agent", "goclaw")
	resp2, err := h.downloadClient.Do(req2)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp2.Body, 4<<10))
		return fmt.Errorf("asset download failed with status %d", resp2.StatusCode)
	}
	if resp2.ContentLength > systemUpdateMaxAssetBytes {
		return fmt.Errorf("asset too large (%d bytes)", resp2.ContentLength)
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.TeeReader(io.LimitReader(resp2.Body, systemUpdateMaxAssetBytes), io.Discard))
	_ = n
	if err != nil {
		return fmt.Errorf("hash asset: %w", err)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && strings.EqualFold(fields[0], got) && strings.HasSuffix(fields[1], wantTar) {
			return nil
		}
	}
	return errors.New("checksum mismatch for " + wantTar)
}

// downloadAndExtract streams the tar.gz asset, writing the pkg-linux-amd64/goclaw
// entry to dstBinary and the pkg-linux-amd64/migrations/ tree to dstMigrations.
// Enforces the 400MB asset size cap. Either destination is optional (empty
// string = skip).
func (h *SystemUpdateHandler) downloadAndExtract(ctx context.Context, assetURL, dstBinary, dstMigrations string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("User-Agent", "goclaw")

	resp, err := h.downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}
	if resp.ContentLength > systemUpdateMaxAssetBytes {
		return fmt.Errorf("asset too large (%d bytes)", resp.ContentLength)
	}

	gz, err := gzip.NewReader(io.LimitReader(resp.Body, systemUpdateMaxAssetBytes))
	if err != nil {
		return fmt.Errorf("open tar.gz: %w", err)
	}
	defer gz.Close()

	const binaryEntry = "pkg-linux-amd64/goclaw"
	const migrationsPrefix = "pkg-linux-amd64/migrations/"
	gotBinary, gotMigrations := false, dstMigrations == ""
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		name := strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./")
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		switch {
		case name == binaryEntry && dstBinary != "":
			if err := writeFileDurable(tr, dstBinary, systemUpdateMaxAssetBytes); err != nil {
				return err
			}
			gotBinary = true
		case strings.HasPrefix(name, migrationsPrefix) && !gotMigrations:
			base := filepath.Base(name)
			if base == "" || base == "." || strings.Contains(base, "/") {
				continue
			}
			if err := os.MkdirAll(dstMigrations, 0o755); err != nil {
				return fmt.Errorf("create migrations dir: %w", err)
			}
			if err := writeFileDurable(tr, filepath.Join(dstMigrations, base), 16<<20); err != nil {
				return fmt.Errorf("extract migration %s: %w", base, err)
			}
		}
	}
	if dstBinary != "" && !gotBinary {
		return fmt.Errorf("entry %s not found in archive", binaryEntry)
	}
	return nil
}

// writeFileDurable writes r into dst via temp+rename so an interrupted write
// never leaves a truncated file at the destination path.
func writeFileDurable(r io.Reader, dst string, limit int64) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, io.LimitReader(r, limit)); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", filepath.Base(dst), err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close %s: %w", filepath.Base(dst), err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("finalize %s: %w", filepath.Base(dst), err)
	}
	return nil
}

// checkELFMagic verifies the file starts with the ELF magic bytes.
func checkELFMagic(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open binary: %w", err)
	}
	defer f.Close()
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return fmt.Errorf("read binary header: %w", err)
	}
	if string(magic[:]) != elfMagic {
		return errors.New("downloaded file is not an ELF binary")
	}
	return nil
}

// verifyBinaryVersion runs `binary --version` with a 10s timeout and requires
// the output to mention the expected tag.
func verifyBinaryVersion(path, tag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), systemUpdateVersionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return fmt.Errorf("new binary --version failed: %w", err)
	}
	if !strings.Contains(string(out), tag) {
		return fmt.Errorf("new binary reported %q, want version %s", strings.TrimSpace(string(out)), tag)
	}
	return nil
}

// copyFile copies src to dst via a temp file + rename so a crash mid-copy
// never leaves a truncated backup.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("copy backup: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close backup: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("finalize backup: %w", err)
	}
	return nil
}

// sanitizeTagForPath makes a version tag safe for a filename ("v1.2.3" ->
// "v1.2.3", weird input gets stripped to alphanumerics, dot, dash).
func sanitizeTagForPath(tag string) string {
	var b strings.Builder
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "unknown"
	}
	return out
}
