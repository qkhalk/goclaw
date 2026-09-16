package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Microsoft Graph v1.0 endpoints (OneDrive).
const (
	graphAPIBase         = "https://graph.microsoft.com/v1.0"
	graphSimpleUploadMax = 4 << 20 // simple PUT cap; larger goes through an upload session
	// graphChunkSize must be a multiple of 320 KiB per the upload session spec.
	graphChunkSize = 10 << 20
)

// GraphBackend implements Backend against Microsoft Graph for one OneDrive
// drive. Graph is path-addressed natively (root:/a/b:/children), so no ID
// cache is needed.
type GraphBackend struct {
	client  *http.Client // authenticated (Bearer via ts)
	base    string       // .../drives/{driveID}
	driveID string
}

// NewGraphBackend builds a OneDrive backend for driveID using ts for auth.
func NewGraphBackend(ctx context.Context, ts oauth2.TokenSource, driveID string) *GraphBackend {
	return &GraphBackend{
		client:  &http.Client{Transport: &oauth2.Transport{Source: ts, Base: http.DefaultTransport}},
		base:    graphAPIBase + "/drives/" + url.PathEscape(driveID),
		driveID: driveID,
	}
}

// graphFile is the file facet (nil for folders).
type graphFile struct {
	MimeType string `json:"mimeType"`
}

// graphFolder is the folder facet (nil for files).
type graphFolder struct {
	ChildCount int `json:"childCount"`
}

// graphItem is the subset of the driveItem resource GoClaw uses.
type graphItem struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Size                 int64  `json:"size"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
	ParentReference      *struct {
		ID string `json:"id"`
	} `json:"parentReference"`
	File   *graphFile   `json:"file"`
	Folder *graphFolder `json:"folder"`
}

func (i *graphItem) isDir() bool { return i.Folder != nil }

func (i *graphItem) stat() *StatInfo {
	mime := ""
	if i.File != nil {
		mime = i.File.MimeType
	}
	return &StatInfo{
		Name: i.Name, Size: i.Size, MimeType: mime,
		ModTime: normModTime(i.LastModifiedDateTime), IsDir: i.isDir(),
	}
}

// escapedPath joins the path segments individually percent-escaped (segment
// escaping keeps literal "/" separators intact while encoding "#", "%", and
// the path-addressing ":" which would otherwise be ambiguous).
func escapedPath(path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		if s != "" {
			out = append(out, strings.ReplaceAll(url.PathEscape(s), ":", "%3A"))
		}
	}
	return strings.Join(out, "/")
}

// itemURL builds the path-addressed item URL ("/root" for the drive root).
func (g *GraphBackend) itemURL(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return g.base + "/root"
	}
	return g.base + "/root:/" + escapedPath(path) + ":"
}

// graphErr maps a Graph error response; 404 / itemNotFound → ErrNotFound.
func graphErr(status int, body []byte, what string) error {
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	msg := strings.TrimSpace(string(body))
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Code != "" {
		msg = parsed.Error.Code + ": " + parsed.Error.Message
	}
	return fmt.Errorf("graph %s: status %d: %s", what, status, truncateStr(msg, 300))
}

// do performs an authenticated Graph request; when out is nil the body is
// drained. respOut receives the raw response for status/Location access.
func (g *GraphBackend) do(ctx context.Context, method, u string, body any, out any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(buf))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	resp.Body.Close()
	if readErr != nil {
		return resp, readErr
	}
	if resp.StatusCode >= 300 {
		return resp, graphErr(resp.StatusCode, respBody, strings.TrimPrefix(u, graphAPIBase))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

// itemByPath fetches one item by path.
func (g *GraphBackend) itemByPath(ctx context.Context, path string) (*graphItem, error) {
	var item graphItem
	if _, err := g.do(ctx, http.MethodGet, g.itemURL(path), nil, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (g *GraphBackend) List(ctx context.Context, dir string, limit int) ([]ListEntry, error) {
	childrenURL := g.itemURL(dir) + "/children?$top=200"
	if limit > 0 && limit < 200 {
		childrenURL = g.itemURL(dir) + fmt.Sprintf("/children?$top=%d", limit)
	}
	var out []ListEntry
	for childrenURL != "" && (limit <= 0 || len(out) < limit) {
		var page struct {
			NextLink string      `json:"@odata.nextLink"`
			Value    []graphItem `json:"value"`
		}
		if _, err := g.do(ctx, http.MethodGet, childrenURL, nil, &page); err != nil {
			return nil, err
		}
		for i := range page.Value {
			it := &page.Value[i]
			out = append(out, ListEntry{
				Name: it.Name, IsDir: it.isDir(), Size: it.Size,
				ModTime: normModTime(it.LastModifiedDateTime),
			})
		}
		childrenURL = page.NextLink
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (g *GraphBackend) Stat(ctx context.Context, path string) (*StatInfo, error) {
	item, err := g.itemByPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return item.stat(), nil
}

func (g *GraphBackend) About(ctx context.Context) (*AboutInfo, error) {
	var drive struct {
		Quota struct {
			Total     int64 `json:"total"`
			Used      int64 `json:"used"`
			Remaining int64 `json:"remaining"`
		} `json:"quota"`
	}
	if _, err := g.do(ctx, http.MethodGet, g.base, nil, &drive); err != nil {
		return nil, err
	}
	return &AboutInfo{Total: drive.Quota.Total, Used: drive.Quota.Used, Free: drive.Quota.Remaining}, nil
}

func (g *GraphBackend) Mkdir(ctx context.Context, dir string) error {
	dir = strings.Trim(dir, "/")
	if dir == "" {
		return nil
	}
	// Idempotent (rclone mkdir parity): an existing folder is a no-op.
	if _, err := g.itemByPath(ctx, dir); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	parent, name := splitDirBase(dir)
	_, err := g.do(ctx, http.MethodPost, g.itemURL(parent)+"/children", map[string]any{
		"name": name, "folder": &graphFolder{},
	}, nil)
	return err
}

func (g *GraphBackend) Delete(ctx context.Context, path string, isDir bool) error {
	item, err := g.itemByPath(ctx, path)
	if err != nil {
		return err
	}
	// Type guard BEFORE deleting — the API contract is "one file, or one
	// EMPTY directory", never a populated tree.
	if item.isDir() && !isDir {
		return fmt.Errorf("%w: %s is a directory (delete with is_dir=true)", ErrNotDir, path)
	}
	if !item.isDir() && isDir {
		return fmt.Errorf("%w: %s is a file", ErrNotDir, path)
	}
	if item.Folder != nil && item.Folder.ChildCount > 0 {
		return ErrDirNotEmpty
	}
	_, err = g.do(ctx, http.MethodDelete, g.base+"/items/"+url.PathEscape(item.ID), nil, nil)
	return err
}

func (g *GraphBackend) Move(ctx context.Context, from, to string) error {
	item, err := g.itemByPath(ctx, from)
	if err != nil {
		return err
	}
	toDir, toName := splitDirBase(strings.Trim(to, "/"))
	patch := map[string]any{}
	if toName != item.Name {
		patch["name"] = toName
	}
	// Only re-parent when the destination differs — compare via the destination
	// parent item when both paths exist, else resolve dst parent path.
	if dstParent, derr := g.itemByPath(ctx, toDir); derr == nil {
		if item.ParentReference != nil && dstParent.ID == item.ParentReference.ID && toName == item.Name {
			return nil // no-op
		}
		if item.ParentReference == nil || dstParent.ID != item.ParentReference.ID {
			patch["parentReference"] = map[string]string{"id": dstParent.ID}
		}
	} else if !errors.Is(derr, ErrNotFound) {
		return derr
	} else {
		return ErrNotFound // destination parent missing
	}
	if len(patch) == 0 {
		return nil
	}
	_, err = g.do(ctx, http.MethodPatch, g.base+"/items/"+url.PathEscape(item.ID), patch, nil)
	return err
}

func (g *GraphBackend) Copy(ctx context.Context, from, to string) error {
	item, err := g.itemByPath(ctx, from)
	if err != nil {
		return err
	}
	toDir, toName := splitDirBase(strings.Trim(to, "/"))
	dstParent, err := g.itemByPath(ctx, toDir)
	if err != nil {
		return err
	}
	body := map[string]any{
		"parentReference": map[string]string{"driveId": g.driveID, "id": dstParent.ID},
	}
	// Graph renames only when the name differs from the source item name.
	if toName != item.Name {
		body["name"] = toName
	}
	resp, err := g.do(ctx, http.MethodPost, g.base+"/items/"+url.PathEscape(item.ID)+"/copy", body, nil)
	if err != nil {
		return err
	}
	return pollAsyncOperation(ctx, resp.Header.Get("Location"))
}

// pollAsyncOperation waits for a Graph long-running operation (202 loop with
// a monitor URL; final 200/204). No auth needed on the monitor URL. The cap
// is generous (10 min) — large server-side copies keep running even when a
// poller gives up, so only truly stuck operations error out.
func pollAsyncOperation(ctx context.Context, monitor string) error {
	if monitor == "" {
		return fmt.Errorf("graph: async operation returned no monitor URL")
	}
	plain := &http.Client{}
	deadline := 10 * time.Minute
	start := time.Now()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, monitor, nil)
		if err != nil {
			return err
		}
		resp, err := plain.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		status := resp.StatusCode
		resp.Body.Close()
		switch {
		case status == http.StatusAccepted: // still running
		case status >= 200 && status < 300:
			return nil // completed
		default:
			return graphErr(status, body, "async operation")
		}
		if time.Since(start) > deadline {
			return fmt.Errorf("graph: async operation timed out after %s", deadline)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (g *GraphBackend) CopyURL(ctx context.Context, rawURL, path string) error {
	if !trimURLScheme(rawURL) {
		return fmt.Errorf("graph copyurl: url must be absolute http(s)")
	}
	resp, err := safeGet(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("graph copyurl: fetch source: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graph copyurl: source status %d", resp.StatusCode)
	}
	var src io.Reader = resp.Body
	size := resp.ContentLength
	if size < 0 {
		// Providers require a known length — spool chunked sources to disk.
		f, n, serr := spoolUnknownSize(resp.Body)
		if serr != nil {
			return fmt.Errorf("graph copyurl: spool: %w", serr)
		}
		defer func() { f.Close(); os.Remove(f.Name()) }()
		src = f
		size = n
	}
	modTime := parseHTTPTime(resp.Header.Get("Last-Modified"))
	return g.Upload(ctx, path, src, size, resp.Header.Get("Content-Type"), modTime)
}

func (g *GraphBackend) PublicLink(ctx context.Context, path string) (*PublicLinkInfo, error) {
	item, err := g.itemByPath(ctx, path)
	if err != nil {
		return nil, err
	}
	var out struct {
		Link *struct {
			WebURL string `json:"webUrl"`
		} `json:"link"`
	}
	if _, err := g.do(ctx, http.MethodPost, g.base+"/items/"+url.PathEscape(item.ID)+"/createLink", map[string]any{
		"type": "view", "scope": "anonymous",
	}, &out); err != nil {
		return nil, err
	}
	if out.Link == nil || out.Link.WebURL == "" {
		return nil, fmt.Errorf("graph: createLink returned no url")
	}
	return &PublicLinkInfo{URL: out.Link.WebURL}, nil
}

func (g *GraphBackend) Open(ctx context.Context, path string, rangeHdr RangeHeader) (*Content, error) {
	item, err := g.itemByPath(ctx, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.base+"/items/"+url.PathEscape(item.ID)+"/content", nil)
	if err != nil {
		return nil, err
	}
	if rangeHdr != "" {
		req.Header.Set("Range", string(rangeHdr))
	}
	// The client follows the 302 to the preauthenticated download URL; Go
	// strips the Authorization header on the cross-host hop automatically.
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, graphErr(resp.StatusCode, body, "download")
	}
	return &Content{
		ReadCloser:   resp.Body,
		Status:       resp.StatusCode,
		Length:       resp.ContentLength,
		MimeType:     resp.Header.Get("Content-Type"),
		ContentRange: resp.Header.Get("Content-Range"),
	}, nil
}

func (g *GraphBackend) Upload(ctx context.Context, path string, src io.Reader, size int64, contentType string, modTime time.Time) error {
	if size < 0 {
		return fmt.Errorf("graph upload: unknown size — spool the source first")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	itemID, err := g.uploadBody(ctx, path, src, size, contentType)
	if err != nil {
		return err
	}
	// Stamp the source modification time (rclone parity — sync mirrors skip
	// identical files by size+mtime; Graph servers always stamp "now").
	if !modTime.IsZero() && itemID != "" {
		_ = g.setModTime(ctx, itemID, modTime)
	}
	return nil
}

// uploadBody performs the actual upload (simple PUT ≤4 MB or a chunked
// upload session) and returns the resulting item id ("" when unavailable).
func (g *GraphBackend) uploadBody(ctx context.Context, path string, src io.Reader, size int64, contentType string) (string, error) {
	// Simple PUT replaces the file wholesale (conflictBehavior=replace keeps
	// rclone-parity: uploads overwrite instead of "file (1)" copies).
	if size <= graphSimpleUploadMax {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			g.itemURL(path)+"/content?"+url.Values{"@microsoft.graph.conflictBehavior": {"replace"}}.Encode(), src)
		if err != nil {
			return "", err
		}
		req.ContentLength = size
		req.Header.Set("Content-Type", contentType)
		resp, err := g.client.Do(req)
		if err != nil {
			return "", err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return "", graphErr(resp.StatusCode, body, "upload")
		}
		var item graphItem
		_ = json.Unmarshal(body, &item)
		return item.ID, nil
	}
	return g.uploadSession(ctx, path, src, size, contentType)
}

// setModTime stamps fileSystemInfo.lastModifiedDateTime after an upload.
// Best effort — a failure here must not fail the upload itself.
func (g *GraphBackend) setModTime(ctx context.Context, itemID string, modTime time.Time) error {
	_, err := g.do(ctx, http.MethodPatch, g.base+"/items/"+url.PathEscape(itemID), map[string]any{
		"fileSystemInfo": map[string]string{
			"lastModifiedDateTime": modTime.UTC().Format("2006-01-02T15:04:05.999Z"),
		},
	}, nil)
	return err
}

// uploadSession implements the Graph resumable upload protocol for big files:
// create a session, then PUT consecutive chunks (Content-Range per chunk,
// chunk sizes multiple of 320 KiB, final chunk carries /total). Returns the
// completed item id when the completion response carries one.
func (g *GraphBackend) uploadSession(ctx context.Context, path string, src io.Reader, size int64, contentType string) (string, error) {
	var session struct {
		UploadURL string `json:"uploadUrl"`
	}
	body := map[string]any{
		"item": map[string]string{"@microsoft.graph.conflictBehavior": "replace"},
	}
	if _, err := g.do(ctx, http.MethodPost, g.itemURL(path)+"/createUploadSession", body, &session); err != nil {
		return "", err
	}
	if session.UploadURL == "" {
		return "", fmt.Errorf("graph: upload session has no uploadUrl")
	}
	plain := &http.Client{}
	buf := make([]byte, graphChunkSize)
	var offset int64
	for {
		n, rerr := io.ReadFull(src, buf)
		if rerr == io.ErrUnexpectedEOF || rerr == io.EOF {
			// final chunk (n may be 0 when size is a multiple of the chunk size)
		} else if rerr != nil {
			return "", rerr
		}
		if n == 0 && offset >= size {
			// Terminating zero-byte PUT (the session's last chunk ended exactly
			// at the file size but Graph answered 202 instead of 201).
			req, err := http.NewRequestWithContext(ctx, http.MethodPut, session.UploadURL, nil)
			if err != nil {
				return "", err
			}
			req.ContentLength = 0
			req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-/%d", size, size))
			resp, err := plain.Do(req)
			if err != nil {
				return "", err
			}
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				return "", graphErr(resp.StatusCode, respBody, "upload finalize")
			}
			return "", nil
		}
		chunk := buf[:n]
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, session.UploadURL, strings.NewReader(string(chunk)))
		if err != nil {
			return "", err
		}
		req.ContentLength = int64(len(chunk))
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+int64(len(chunk))-1, size))
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := plain.Do(req)
		if err != nil {
			return "", err
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return "", graphErr(resp.StatusCode, respBody, "upload chunk")
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			var item graphItem
			_ = json.Unmarshal(respBody, &item)
			return item.ID, nil // 201/200 = session complete
		}
		offset += int64(len(chunk))
		if offset >= size {
			// The final chunk usually completes the session (201 above); a 202
			// here means Graph still expects an empty terminating PUT.
			if resp.StatusCode == http.StatusAccepted {
				continue
			}
			return "", nil
		}
	}
}
