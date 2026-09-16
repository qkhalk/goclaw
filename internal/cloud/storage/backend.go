// Package storage provides native, in-process cloud storage backends for the
// providers GoClaw connects (Google Drive, Microsoft OneDrive). It replaces
// the former exec-rclone-rcd integration: the same operations now go straight
// to the provider REST APIs (Drive v3 / Microsoft Graph) using the OAuth
// token sources the cloud Manager already maintains — no external binary, no
// child process, and native byte-range downloads for file previews.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/security"
)

// ErrNotFound is returned when a remote path does not exist.
var ErrNotFound = errors.New("cloud storage: path not found")

// ErrNotDir is returned when a directory operation hits a file path.
var ErrNotDir = errors.New("cloud storage: path is not a directory")

// ErrDirNotEmpty is returned when deleting a directory that still has
// children (parity with the former rclone rmdir semantics — wiping a
// populated tree is deliberately not exposed).
var ErrDirNotEmpty = errors.New("cloud storage: directory not empty")

// ListEntry is one row of a directory listing.
type ListEntry struct {
	Name    string `json:"Name"`
	IsDir   bool   `json:"IsDir"`
	Size    int64  `json:"Size"`
	ModTime string `json:"ModTime"`
}

// StatInfo is one file/dir stat.
type StatInfo struct {
	Name     string `json:"Name"`
	Size     int64  `json:"Size"`
	MimeType string `json:"MimeType"`
	ModTime  string `json:"ModTime"`
	IsDir    bool   `json:"IsDir"`
}

// AboutInfo is the storage quota of an account.
type AboutInfo struct {
	Total int64 `json:"Total"`
	Used  int64 `json:"Used"`
	Free  int64 `json:"Free"`
}

// PublicLinkInfo is the share-link result for one path.
type PublicLinkInfo struct {
	URL string `json:"url"`
}

// JobInfo is the progress snapshot of an async copy job. FilesTotal is the
// count discovered when the walk finishes (0 while still listing).
type JobInfo struct {
	ID         int64  `json:"id"`
	Finished   bool   `json:"finished"`
	Success    bool   `json:"success"`
	Error      string `json:"error"`
	FilesDone  int64  `json:"files_done"`
	FilesTotal int64  `json:"files_total"`
}

// Content is an opened remote file body. Status relays the provider's HTTP
// status (200 full body / 206 range) so callers can mirror range semantics;
// Length is the Content-Length of THIS body (-1 when unknown) and
// ContentRange the provider's Content-Range header when present.
type Content struct {
	io.ReadCloser
	Status       int
	Length       int64
	MimeType     string
	ContentRange string
}

// RangeHeader is the raw HTTP Range header to relay ("" = full body).
type RangeHeader string

// Backend is the native per-account storage surface. Implementations are
// safe for concurrent use.
type Backend interface {
	List(ctx context.Context, dir string, limit int) ([]ListEntry, error)
	Stat(ctx context.Context, path string) (*StatInfo, error)
	About(ctx context.Context) (*AboutInfo, error)
	Mkdir(ctx context.Context, dir string) error
	// Delete removes one file or one EMPTY directory (isDir).
	Delete(ctx context.Context, path string, isDir bool) error
	// Move renames within a folder and/or moves between folders.
	Move(ctx context.Context, from, to string) error
	// Copy copies one file to another path in the same account (server-side).
	Copy(ctx context.Context, from, to string) error
	// CopyURL fetches an http(s) URL server-side into path.
	CopyURL(ctx context.Context, rawURL, path string) error
	// PublicLink creates/retrieves the anonymous share link for path.
	PublicLink(ctx context.Context, path string) (*PublicLinkInfo, error)
	// Open streams the file body, relaying rangeHdr when set.
	Open(ctx context.Context, path string, rangeHdr RangeHeader) (*Content, error)
	// Upload writes src (size bytes, size >= 0 when known) to the full path,
	// overwriting an existing file at the same path and stamping modTime
	// (zero = provider now) so sync mirrors can skip identical files.
	Upload(ctx context.Context, path string, src io.Reader, size int64, contentType string, modTime time.Time) error
}

// maxRedirectHops bounds the manual redirect walk of the SSRF-guarded fetch.
const maxRedirectHops = 5

// safeGet performs an SSRF-guarded GET of rawURL: every hop is re-validated
// against the loopback/private-IP guard and dialed pinned to the resolved IP
// (the house SafeClient refuses redirects, so hops are followed manually).
func safeGet(ctx context.Context, rawURL string) (*http.Response, error) {
	current := rawURL
	for hop := 0; hop < maxRedirectHops; hop++ {
		_, pinnedIP, err := security.Validate(current)
		if err != nil {
			return nil, fmt.Errorf("url rejected by SSRF guard: %w", err)
		}
		req, err := http.NewRequestWithContext(security.WithPinnedIP(ctx, pinnedIP), http.MethodGet, current, nil)
		if err != nil {
			return nil, err
		}
		resp, err := security.NewSafeClient(60 * time.Second).Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			next := resp.Header.Get("Location")
			if next == "" {
				return resp, nil
			}
			resp.Body.Close()
			base, perr := url.Parse(current)
			ref, rerr := url.Parse(next)
			if perr != nil || rerr != nil {
				return nil, fmt.Errorf("copyurl: bad redirect target")
			}
			current = base.ResolveReference(ref).String()
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("copyurl: too many redirects (max %d)", maxRedirectHops)
}

// spoolUnknownSize copies a size-unknown stream into a temp file (providers
// require a known length for upload sessions) and returns it plus the size.
// The caller owns Close + Remove.
func spoolUnknownSize(r io.Reader) (*os.File, int64, error) {
	tmp, err := os.CreateTemp("", "goclaw-copyurl-")
	if err != nil {
		return nil, 0, err
	}
	n, err := io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return nil, 0, err
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		os.Remove(tmp.Name())
		return nil, 0, err
	}
	return f, n, nil
}

// trimURLScheme reports whether rawURL is an absolute http(s) URL.
func trimURLScheme(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// normModTime normalizes a provider timestamp to RFC3339 (second precision,
// UTC) so the wire format stays stable across providers; unparseable input is
// returned unchanged.
func normModTime(raw string) string {
	if raw == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return raw
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
