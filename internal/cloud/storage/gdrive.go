package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// Google Drive API v3 endpoints (vars so tests can point them at fakes).
var (
	driveAPIBase    = "https://www.googleapis.com/drive/v3"
	driveUploadBase = "https://www.googleapis.com/upload/drive/v3"
)

const (
	driveFolderMime = "application/vnd.google-apps.folder"
	// driveSimpleUploadMax is the largest body sent via one multipart request;
	// bigger uploads switch to a resumable session (single full PUT).
	driveSimpleUploadMax = 5 << 20
)

// driveDirCacheTTL bounds the path→ID cache the way rclone's dir cache did
// (~5 min): renames made in the provider's own UI eventually resolve fresh.
const driveDirCacheTTL = 5 * time.Minute

// DriveBackend implements Backend against the Google Drive v3 REST API.
// Path semantics mirror the former rclone drive integration: slash-separated
// paths from the drive root, resolved to Drive file IDs on demand with a
// per-account TTL cache (invalidated on mutations through this instance).
type DriveBackend struct {
	client *http.Client

	mu   sync.Mutex
	dirs map[string]dirCacheEntry
}

type dirCacheEntry struct {
	id string
	at time.Time
}

// NewDriveBackend builds a Drive backend using ts for auth (auto-refresh).
func NewDriveBackend(ctx context.Context, ts oauth2.TokenSource) *DriveBackend {
	return &DriveBackend{
		client: &http.Client{Transport: &oauth2.Transport{Source: ts, Base: http.DefaultTransport}},
		dirs:   map[string]dirCacheEntry{"": {id: "root"}},
	}
}

// driveErr converts a non-2xx provider response into an error, mapping 404 to
// ErrNotFound.
func driveErr(resp *http.Response, body []byte, what string) error {
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	return fmt.Errorf("drive %s: status %d: %s", what, resp.StatusCode, truncateStr(string(body), 300))
}

// driveGet performs an authenticated GET and decodes the JSON body (16 MB cap
// — directory listings are the biggest responses).
func (d *DriveBackend) driveGet(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return driveErr(resp, body, strings.TrimPrefix(u, driveAPIBase))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// driveDo performs an authenticated request with a JSON body and decodes the
// (optional) response.
func (d *DriveBackend) driveDo(ctx context.Context, method, u string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(buf))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return driveErr(resp, respBody, u)
	}
	if out != nil && len(respBody) > 0 {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

// driveFile is the subset of the Drive file resource GoClaw uses.
type driveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	Size         string `json:"size"` // string per API
	ModifiedTime string `json:"modifiedTime"`
}

func (f *driveFile) isDir() bool { return f.MimeType == driveFolderMime }

func (f *driveFile) size() int64 {
	if f.Size == "" {
		return 0
	}
	var n int64
	fmt.Sscanf(f.Size, "%d", &n)
	return n
}

func (f *driveFile) entry() ListEntry {
	return ListEntry{Name: f.Name, IsDir: f.isDir(), Size: f.size(), ModTime: normModTime(f.ModifiedTime)}
}

// escapeQuery escapes a value inside a Drive q='...' literal.
func escapeQuery(s string) string {
	return strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(s)
}

// cachedDirID resolves a dir path to its folder ID via the TTL cache,
// falling back to a walk.
func (d *DriveBackend) cachedDirID(ctx context.Context, dir string) (string, error) {
	d.mu.Lock()
	if e, ok := d.dirs[dir]; ok && time.Since(e.at) < driveDirCacheTTL {
		d.mu.Unlock()
		return e.id, nil
	}
	d.mu.Unlock()
	id, err := d.resolveDir(ctx, dir)
	if err != nil {
		return "", err
	}
	d.mu.Lock()
	d.dirs[dir] = dirCacheEntry{id: id, at: time.Now()}
	d.mu.Unlock()
	return id, nil
}

// resolveDir walks the dir path segments to a folder ID (ErrNotFound when any
// segment is missing, folder constraint keeps intermediate steps on-track).
func (d *DriveBackend) resolveDir(ctx context.Context, dir string) (string, error) {
	parent := "root"
	if dir == "" {
		return parent, nil
	}
	for _, seg := range strings.Split(dir, "/") {
		id, err := d.childID(ctx, parent, seg, true)
		if err != nil {
			return "", err
		}
		parent = id
	}
	return parent, nil
}

// childID finds one child by name under parentID; requireFolder constrains
// intermediate walk steps to folders. Newest wins (Drive permits duplicate
// names; the freshest copy is the one prior uploads meant to leave).
func (d *DriveBackend) childID(ctx context.Context, parentID, name string, requireFolder bool) (string, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", escapeQuery(name), escapeQuery(parentID))
	if requireFolder {
		q += fmt.Sprintf(" and mimeType = '%s'", driveFolderMime)
	}
	var out struct {
		Files []driveFile `json:"files"`
	}
	u := driveAPIBase + "/files?q=" + url.QueryEscape(q) +
		"&fields=" + url.QueryEscape("files(id,name,mimeType)") +
		"&orderBy=" + url.QueryEscape("modifiedTime desc") + "&pageSize=2"
	if err := d.driveGet(ctx, u, &out); err != nil {
		return "", err
	}
	if len(out.Files) == 0 {
		return "", ErrNotFound
	}
	return out.Files[0].ID, nil
}

// resolvePath returns (fileOrDirID, parentID, name) for path; the root itself
// resolves to ("root", "root", "").
func (d *DriveBackend) resolvePath(ctx context.Context, path string) (id, parentID, name string, err error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return "root", "root", "", nil
	}
	dir, base := splitDirBase(path)
	parentID, err = d.cachedDirID(ctx, dir)
	if err != nil {
		return "", "", "", err
	}
	id, err = d.childID(ctx, parentID, base, false)
	if err != nil {
		return "", "", "", err
	}
	return id, parentID, base, nil
}

func splitDirBase(path string) (dir, base string) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

// invalidate drops the dir cache entry for path's parent (and any descendants
// of path when it names a directory) — mutations through this instance are
// the only invalidation source, matching the per-account cache lifetime.
func (d *DriveBackend) invalidate(path string) {
	path = strings.Trim(path, "/")
	dir, _ := splitDirBase(path)
	d.mu.Lock()
	delete(d.dirs, path) // when path names a dir, its own cached subtree root
	delete(d.dirs, dir)
	for k := range d.dirs {
		if strings.HasPrefix(k, path+"/") {
			delete(d.dirs, k)
		}
	}
	d.mu.Unlock()
}

func (d *DriveBackend) List(ctx context.Context, dir string, limit int) ([]ListEntry, error) {
	dir = strings.Trim(dir, "/")
	parentID, err := d.cachedDirID(ctx, dir)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf("'%s' in parents and trashed = false", escapeQuery(parentID))
	pageSize := 200
	if limit > 0 && limit < pageSize {
		pageSize = limit
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	var out []ListEntry
	pageToken := ""
	for {
		u := driveAPIBase + "/files?q=" + url.QueryEscape(q) +
			"&fields=" + url.QueryEscape("nextPageToken,files(id,name,mimeType,size,modifiedTime)") +
			fmt.Sprintf("&pageSize=%d&orderBy=folder,name", pageSize)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		var page struct {
			NextPageToken string      `json:"nextPageToken"`
			Files         []driveFile `json:"files"`
		}
		if err := d.driveGet(ctx, u, &page); err != nil {
			return nil, err
		}
		for i := range page.Files {
			out = append(out, page.Files[i].entry())
		}
		if page.NextPageToken == "" || (limit > 0 && len(out) >= limit) {
			break
		}
		pageToken = page.NextPageToken
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (d *DriveBackend) Stat(ctx context.Context, path string) (*StatInfo, error) {
	id, _, _, err := d.resolvePath(ctx, path)
	if err != nil {
		return nil, err
	}
	var f driveFile
	u := driveAPIBase + "/files/" + url.PathEscape(id) + "?fields=" + url.QueryEscape("id,name,mimeType,size,modifiedTime")
	if err := d.driveGet(ctx, u, &f); err != nil {
		return nil, err
	}
	return &StatInfo{
		Name: f.Name, Size: f.size(), MimeType: f.MimeType,
		ModTime: normModTime(f.ModifiedTime), IsDir: f.isDir(),
	}, nil
}

func (d *DriveBackend) About(ctx context.Context) (*AboutInfo, error) {
	var out struct {
		StorageQuota *struct {
			Limit        string `json:"limit"`
			UsageInDrive string `json:"usageInDrive"`
		} `json:"storageQuota"`
	}
	if err := d.driveGet(ctx, driveAPIBase+"/about?fields="+url.QueryEscape("storageQuota"), &out); err != nil {
		return nil, err
	}
	info := AboutInfo{}
	if out.StorageQuota == nil {
		return &info, nil
	}
	fmt.Sscanf(out.StorageQuota.Limit, "%d", &info.Total)
	fmt.Sscanf(out.StorageQuota.UsageInDrive, "%d", &info.Used)
	if info.Total > 0 {
		info.Free = info.Total - info.Used
	}
	return &info, nil
}

func (d *DriveBackend) Mkdir(ctx context.Context, dir string) error {
	dir = strings.Trim(dir, "/")
	if dir == "" {
		return nil
	}
	parent, name := splitDirBase(dir)
	parentID, err := d.cachedDirID(ctx, parent)
	if err != nil {
		return err
	}
	// Idempotent (rclone mkdir parity): an existing folder is a no-op — Drive
	// would otherwise silently create duplicate names.
	if _, err := d.childID(ctx, parentID, name, true); err == nil {
		return nil
	} else if err != ErrNotFound {
		return err
	}
	if err := d.driveDo(ctx, http.MethodPost, driveAPIBase+"/files?fields=id", map[string]any{
		"name": name, "parents": []string{parentID}, "mimeType": driveFolderMime,
	}, nil); err != nil {
		return err
	}
	d.invalidate(dir)
	return nil
}

func (d *DriveBackend) Delete(ctx context.Context, path string, isDir bool) error {
	id, _, _, err := d.resolvePath(ctx, path)
	if err != nil {
		return err
	}
	// Type guard BEFORE deleting: the API contract is "one file, or one EMPTY
	// directory" — Drive's DELETE on a folder id would wipe the whole tree.
	var f driveFile
	if err := d.driveGet(ctx, driveAPIBase+"/files/"+url.PathEscape(id)+"?fields="+url.QueryEscape("id,mimeType"), &f); err != nil {
		return err
	}
	if f.isDir() && !isDir {
		return fmt.Errorf("%w: %s is a directory (delete with is_dir=true)", ErrNotDir, path)
	}
	if !f.isDir() && isDir {
		return fmt.Errorf("%w: %s is a file", ErrNotDir, path)
	}
	if f.isDir() {
		q := fmt.Sprintf("'%s' in parents and trashed = false", escapeQuery(id))
		var out struct {
			Files []driveFile `json:"files"`
		}
		u := driveAPIBase + "/files?q=" + url.QueryEscape(q) + "&pageSize=1&fields=" + url.QueryEscape("files(id)")
		if err := d.driveGet(ctx, u, &out); err != nil {
			return err
		}
		if len(out.Files) > 0 {
			return ErrDirNotEmpty
		}
	}
	// Trash (recoverable) instead of permanent delete — the former rclone
	// integration ran with --drive-use-trash defaulting to true.
	if err := d.driveDo(ctx, http.MethodPatch, driveAPIBase+"/files/"+url.PathEscape(id)+"?fields=id", map[string]any{
		"trashed": true,
	}, nil); err != nil {
		return err
	}
	d.invalidate(path)
	return nil
}

func (d *DriveBackend) Move(ctx context.Context, from, to string) error {
	id, fromParentID, fromName, err := d.resolvePath(ctx, from)
	if err != nil {
		return err
	}
	toDir, toName := splitDirBase(strings.Trim(to, "/"))
	toParentID, err := d.cachedDirID(ctx, toDir)
	if err != nil {
		return err
	}
	if toParentID == fromParentID && toName == fromName {
		return nil // no-op
	}
	u := driveAPIBase + "/files/" + url.PathEscape(id) + "?fields=id"
	if toParentID != fromParentID {
		u += "&addParents=" + url.QueryEscape(toParentID) + "&removeParents=" + url.QueryEscape(fromParentID)
	}
	patch := map[string]any{}
	if toName != fromName {
		patch["name"] = toName
	}
	if err := d.driveDo(ctx, http.MethodPatch, u, patch, nil); err != nil {
		return err
	}
	d.invalidate(from)
	d.invalidate(to)
	return nil
}

func (d *DriveBackend) Copy(ctx context.Context, from, to string) error {
	id, _, _, err := d.resolvePath(ctx, from)
	if err != nil {
		return err
	}
	toDir, toName := splitDirBase(strings.Trim(to, "/"))
	toParentID, err := d.cachedDirID(ctx, toDir)
	if err != nil {
		return err
	}
	return d.driveDo(ctx, http.MethodPost, driveAPIBase+"/files/"+url.PathEscape(id)+"/copy?fields=id", map[string]any{
		"name": toName, "parents": []string{toParentID},
	}, nil)
}

func (d *DriveBackend) CopyURL(ctx context.Context, rawURL, path string) error {
	if !trimURLScheme(rawURL) {
		return fmt.Errorf("drive copyurl: url must be absolute http(s)")
	}
	resp, err := safeGet(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("drive copyurl: fetch source: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("drive copyurl: source status %d", resp.StatusCode)
	}
	var src io.Reader = resp.Body
	size := resp.ContentLength
	if size < 0 {
		// Providers require a known length — spool chunked sources to disk.
		f, n, err := spoolUnknownSize(resp.Body)
		if err != nil {
			return fmt.Errorf("drive copyurl: spool: %w", err)
		}
		defer func() { f.Close(); os.Remove(f.Name()) }()
		src = f
		size = n
	}
	modTime := parseHTTPTime(resp.Header.Get("Last-Modified"))
	return d.Upload(ctx, path, src, size, resp.Header.Get("Content-Type"), modTime)
}

// drivePublicLink tolerance: only duplicate-permission conflicts are
// non-fatal (the grant already exists); anything else fails loudly.
func (d *DriveBackend) PublicLink(ctx context.Context, path string) (*PublicLinkInfo, error) {
	id, _, _, err := d.resolvePath(ctx, path)
	if err != nil {
		return nil, err
	}
	perr := d.driveDo(ctx, http.MethodPost, driveAPIBase+"/files/"+url.PathEscape(id)+"/permissions", map[string]any{
		"role": "reader", "type": "anyone",
	}, nil)
	if perr != nil && !strings.Contains(perr.Error(), "status 409") {
		return nil, perr
	}
	return &PublicLinkInfo{URL: "https://drive.google.com/uc?id=" + id + "&export=download"}, nil
}

func (d *DriveBackend) Open(ctx context.Context, path string, rangeHdr RangeHeader) (*Content, error) {
	id, _, _, err := d.resolvePath(ctx, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		driveAPIBase+"/files/"+url.PathEscape(id)+"?alt=media", nil)
	if err != nil {
		return nil, err
	}
	if rangeHdr != "" {
		req.Header.Set("Range", string(rangeHdr))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, driveErr(resp, body, "download")
	}
	return &Content{
		ReadCloser:   resp.Body,
		Status:       resp.StatusCode,
		Length:       resp.ContentLength,
		MimeType:     resp.Header.Get("Content-Type"),
		ContentRange: resp.Header.Get("Content-Range"),
	}, nil
}

func (d *DriveBackend) Upload(ctx context.Context, path string, src io.Reader, size int64, contentType string, modTime time.Time) error {
	if size < 0 {
		return fmt.Errorf("drive upload: unknown size — spool the source first")
	}
	path = strings.Trim(path, "/")
	dir, name := splitDirBase(path)
	parentID, err := d.cachedDirID(ctx, dir)
	if err != nil {
		return err
	}
	meta, _ := json.Marshal(d.uploadMeta(name, parentID, modTime))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Overwrite when a file already exists at this path (Drive permits
	// duplicate names — a blind create would accumulate copies).
	existingID, err := d.childID(ctx, parentID, name, false)
	if err != nil && err != ErrNotFound {
		return err
	}
	if existingID != "" && err == nil {
		// PATCH multipart (metadata + media) updates content AND modifiedTime.
		body, total := multipartBody(string(meta), contentType, src, size)
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPatch,
			driveUploadBase+"/files/"+url.PathEscape(existingID)+"?uploadType=multipart&fields=id", body)
		if rerr != nil {
			return rerr
		}
		req.ContentLength = total
		req.Header.Set("Content-Type", relatedContentType)
		resp, derr := d.client.Do(req)
		return finishUploadResp(resp, derr)
	}

	if size <= driveSimpleUploadMax {
		// Multipart/related upload (metadata + body in one bounded request).
		body, total := multipartBody(string(meta), contentType, src, size)
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost,
			driveUploadBase+"/files?uploadType=multipart&fields=id", body)
		if rerr != nil {
			return rerr
		}
		req.ContentLength = total
		req.Header.Set("Content-Type", relatedContentType)
		resp, derr := d.client.Do(req)
		return finishUploadResp(resp, derr)
	}

	// Resumable session with a single full PUT (supports any size; Google
	// requires the metadata request first, then the body at the session URL).
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		driveUploadBase+"/files?uploadType=resumable&fields=id", strings.NewReader(string(meta)))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(meta))
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", contentType)
	req.Header.Set("X-Upload-Content-Length", fmt.Sprint(size))
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	rbody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return driveErr(resp, rbody, "upload session")
	}
	sessionURL := resp.Header.Get("Location")
	if sessionURL == "" {
		return fmt.Errorf("drive upload: session response has no Location")
	}
	put, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURL, src)
	if err != nil {
		return err
	}
	put.ContentLength = size
	put.Header.Set("Content-Type", contentType)
	putResp, err := d.client.Do(put)
	return finishUploadResp(putResp, err)
}

// uploadMeta builds the Drive metadata for create/update with an explicit
// modification time so sync mirrors can skip identical files.
func (d *DriveBackend) uploadMeta(name, parentID string, modTime time.Time) map[string]any {
	meta := map[string]any{"name": name, "parents": []string{parentID}}
	if !modTime.IsZero() {
		meta["modifiedTime"] = modTime.UTC().Format(time.RFC3339)
	}
	return meta
}

// multipartBody assembles the multipart/related body (metadata part + media
// part) with its exact total length — Google rejects chunked multipart.
func multipartBody(metaJSON, contentType string, src io.Reader, size int64) (io.Reader, int64) {
	preamble := fmt.Sprintf("--%s\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n%s\r\n--%s\r\nContent-Type: %s\r\n\r\n",
		relatedBoundary, metaJSON, relatedBoundary, contentType)
	epilogue := fmt.Sprintf("\r\n--%s--\r\n", relatedBoundary)
	total := int64(len(preamble)) + size + int64(len(epilogue))
	return io.MultiReader(strings.NewReader(preamble), src, strings.NewReader(epilogue)), total
}

func finishUploadResp(resp *http.Response, err error) error {
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return driveErr(resp, body, "upload")
	}
	return nil
}

// httpGetFollow performs a plain (unauthenticated) GET following redirects.
func httpGetFollow(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

// --- multipart/related body (Drive multipart upload) ---

const relatedBoundary = "goclawcloudboundary7f3a"

var relatedContentType = "multipart/related; boundary=" + relatedBoundary

// parseHTTPTime parses an HTTP Date header (zero when absent/invalid).
func parseHTTPTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	if t, err := http.ParseTime(raw); err == nil {
		return t
	}
	return time.Time{}
}
