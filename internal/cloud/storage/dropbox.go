package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Dropbox REST roots (kept local — the cloud package imports storage, not
// the reverse).
const (
	DropboxAPIBaseURL     = "https://api.dropboxapi.com/2"
	DropboxContentBaseURL = "https://content.dropboxapi.com/2"
)

// DropboxBackend implements Backend against the Dropbox REST API (paths are
// the API — one namespace, no drive id). RPC calls POST JSON to
// api.dropboxapi.com/2; content calls POST to content.dropboxapi.com/2 with
// parameters serialized into the Dropbox-API-Arg header.
type DropboxBackend struct {
	client *http.Client
}

// NewDropboxBackend builds a Dropbox backend using ts for auth.
func NewDropboxBackend(_ context.Context, ts oauth2.TokenSource) *DropboxBackend {
	return &DropboxBackend{client: &http.Client{Transport: &oauth2.Transport{Source: ts, Base: http.DefaultTransport}}}
}

// dropboxPath normalizes our "/"-rooted paths into Dropbox API paths
// (Dropbox wants "" for the root and "/a/b" elsewhere).
func dropboxPath(p string) string {
	if p == "" || p == "/" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// dropboxAPIError carries the error_summary Dropbox returns on failures.
type dropboxAPIError struct {
	ErrorSummary string `json:"error_summary"`
}

// apiPost performs an RPC call (JSON body → JSON response).
func (b *DropboxBackend) apiPost(ctx context.Context, path string, args any, out any) error {
	raw, err := json.Marshal(args)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DropboxAPIBaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		var apiErr dropboxAPIError
		_ = json.Unmarshal(body, &apiErr)
		return classifyDropboxError(resp.StatusCode, apiErr.ErrorSummary)
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// contentPost performs a content-endpoint call with Dropbox-API-Arg headers.
func (b *DropboxBackend) contentPost(ctx context.Context, path, apiArg string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DropboxContentBaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Dropbox-API-Arg", apiArg)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var apiErr dropboxAPIError
		_ = json.Unmarshal(raw, &apiErr)
		return nil, classifyDropboxError(resp.StatusCode, apiErr.ErrorSummary)
	}
	return resp, nil
}

func classifyDropboxError(status int, summary string) error {
	low := strings.ToLower(summary)
	if status == http.StatusNotFound || strings.Contains(low, "not_found") || strings.Contains(low, "path_lookup") {
		return ErrNotFound
	}
	if strings.Contains(low, "not_empty") {
		return ErrDirNotEmpty
	}
	if strings.Contains(low, "is_folder") {
		return ErrNotDir
	}
	if summary == "" {
		return fmt.Errorf("dropbox status %d", status)
	}
	return fmt.Errorf("dropbox: %s", summary)
}

// dropboxEntry is one metadata object (files/list_folder, get_metadata).
type dropboxEntry struct {
	Tag            string `json:".tag"`
	Name           string `json:"name"`
	PathLower      string `json:"path_lower"`
	Size           int64  `json:"size"`
	ServerModified string `json:"server_modified"`
}

func (e dropboxEntry) isDir() bool { return e.Tag == "folder" }

type dropboxListResult struct {
	Entries []dropboxEntry `json:"entries"`
	Cursor  string         `json:"cursor"`
	HasMore bool           `json:"has_more"`
}

// List lists one directory (bounded by limit; continues pages while under).
func (b *DropboxBackend) List(ctx context.Context, dir string, limit int) ([]ListEntry, error) {
	args := map[string]any{"path": dropboxPath(dir), "include_media_info": false}
	if limit > 0 {
		args["limit"] = min(limit, 2000)
	}
	var page dropboxListResult
	if err := b.apiPost(ctx, "/files/list_folder", args, &page); err != nil {
		return nil, err
	}
	entries := page.Entries
	for page.HasMore && (limit <= 0 || len(entries) < limit) && page.Cursor != "" {
		var next dropboxListResult
		if err := b.apiPost(ctx, "/files/list_folder/continue", map[string]string{"cursor": page.Cursor}, &next); err != nil {
			break
		}
		entries = append(entries, next.Entries...)
		page = next
	}
	out := make([]ListEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, ListEntry{
			Name:    e.Name,
			IsDir:   e.isDir(),
			Size:    e.Size,
			ModTime: normModTime(e.ServerModified),
		})
	}
	return out, nil
}

// Stat stats one path.
func (b *DropboxBackend) Stat(ctx context.Context, path string) (*StatInfo, error) {
	var e dropboxEntry
	if err := b.apiPost(ctx, "/files/get_metadata", map[string]any{"path": dropboxPath(path)}, &e); err != nil {
		return nil, err
	}
	return &StatInfo{
		Name:     e.Name,
		Size:     e.Size,
		ModTime:  normModTime(e.ServerModified),
		IsDir:    e.isDir(),
		MimeType: "",
	}, nil
}

// About returns the account's space usage.
func (b *DropboxBackend) About(ctx context.Context) (*AboutInfo, error) {
	var raw struct {
		Used       int64 `json:"used"`
		Allocation struct {
			Allocated    int64 `json:"allocated"`
			SpaceType    string `json:".tag"`
			TeamsAllocation *struct {
				Allocated int64 `json:"allocated"`
			} `json:"allocated"`
		} `json:"allocation"`
	}
	if err := b.apiPost(ctx, "/users/get_space_usage", map[string]any{}, &raw); err != nil {
		return nil, err
	}
	total := raw.Allocation.Allocated
	if total == 0 && raw.Allocation.TeamsAllocation != nil {
		total = raw.Allocation.TeamsAllocation.Allocated
	}
	return &AboutInfo{Total: total, Used: raw.Used, Free: total - raw.Used}, nil
}

// Mkdir creates a folder.
func (b *DropboxBackend) Mkdir(ctx context.Context, dir string) error {
	return b.apiPost(ctx, "/files/create_folder_v2", map[string]any{"path": dropboxPath(dir), "autorename": false}, nil)
}

// Delete removes a file or an EMPTY directory (Dropbox's delete_v2 is
// recursive, so the emptiness pre-check preserves rclone rmdir semantics).
func (b *DropboxBackend) Delete(ctx context.Context, path string, isDir bool) error {
	if isDir {
		entries, err := b.List(ctx, path, 2)
		if err == nil && len(entries) > 0 {
			return ErrDirNotEmpty
		}
		if err != nil && err != ErrNotFound {
			return err
		}
	}
	return b.apiPost(ctx, "/files/delete_v2", map[string]any{"path": dropboxPath(path)}, nil)
}

// Move renames/moves one path.
func (b *DropboxBackend) Move(ctx context.Context, from, to string) error {
	return b.apiPost(ctx, "/files/move_v2", map[string]any{
		"from_path": dropboxPath(from), "to_path": dropboxPath(to), "autorename": false,
	}, nil)
}

// Copy copies one path server-side.
func (b *DropboxBackend) Copy(ctx context.Context, from, to string) error {
	return b.apiPost(ctx, "/files/copy_v2", map[string]any{
		"from_path": dropboxPath(from), "to_path": dropboxPath(to), "autorename": false,
	}, nil)
}

// CopyURL pulls an http(s) URL into the path (Dropbox runs the fetch).
func (b *DropboxBackend) CopyURL(ctx context.Context, rawURL, path string) error {
	if !trimURLScheme(rawURL) {
		return fmt.Errorf("dropbox save_url: only http(s) URLs are supported")
	}
	var job struct {
		AsyncJobID string `json:"async_job_id"`
	}
	if err := b.apiPost(ctx, "/files/save_url", map[string]any{
		"path": dropboxPath(path), "url": rawURL,
	}, &job); err != nil {
		return err
	}
	if job.AsyncJobID == "" {
		return nil // completed synchronously
	}
	for tries := 0; tries < 30; tries++ {
		var status struct {
			Tag    string `json:".tag"`
			Failed string `json:"failed"`
		}
		if err := b.apiPost(ctx, "/files/save_url/check_job", map[string]string{"async_job_id": job.AsyncJobID}, &status); err != nil {
			return err
		}
		switch status.Tag {
		case "complete":
			return nil
		case "async":
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		case "failed":
			return fmt.Errorf("dropbox save_url failed: %s", status.Failed)
		default:
			return fmt.Errorf("dropbox save_url: unknown status %q", status.Tag)
		}
	}
	return fmt.Errorf("dropbox save_url: timed out waiting for completion")
}

// PublicLink creates (or fetches) the anonymous share link, forced to
// direct-download (dl=1) like rclone's url backend convention.
func (b *DropboxBackend) PublicLink(ctx context.Context, path string) (*PublicLinkInfo, error) {
	var link struct {
		URL string `json:"url"`
	}
	err := b.apiPost(ctx, "/sharing/create_shared_link_with_settings",
		map[string]any{"path": dropboxPath(path), "settings": map[string]any{"requested_visibility": "public"}}, &link)
	if err != nil {
		// Already shared: look the existing direct link up.
		if !strings.Contains(strings.ToLower(fmt.Sprint(err)), "shared_link_already_exists") {
			return nil, err
		}
		var list struct {
			Links []struct {
				URL string `json:"url"`
			} `json:"links"`
		}
		if lerr := b.apiPost(ctx, "/sharing/list_shared_links",
			map[string]any{"path": dropboxPath(path), "direct_only": true}, &list); lerr != nil || len(list.Links) == 0 {
			return nil, err
		}
		link.URL = list.Links[0].URL
	}
	// Dropbox share pages default to a preview (?dl=0); dl=1 streams bytes.
	dl := strings.Replace(link.URL, "?dl=0", "?dl=1", 1)
	if dl == link.URL && link.URL != "" {
		dl += "?dl=1"
	}
	return &PublicLinkInfo{URL: dl}, nil
}

// Open streams the file body, relaying rangeHdr (Dropbox honors Range on
// downloads).
func (b *DropboxBackend) Open(ctx context.Context, path string, rangeHdr RangeHeader) (*Content, error) {
	arg, _ := json.Marshal(map[string]string{"path": dropboxPath(path)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DropboxContentBaseURL+"/files/download", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Dropbox-API-Arg", string(arg))
	if rangeHdr != "" {
		req.Header.Set("Range", string(rangeHdr))
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var apiErr dropboxAPIError
		_ = json.Unmarshal(raw, &apiErr)
		return nil, classifyDropboxError(resp.StatusCode, apiErr.ErrorSummary)
	}
	return &Content{
		ReadCloser:   resp.Body,
		Status:       resp.StatusCode,
		Length:       resp.ContentLength,
		ContentRange: resp.Header.Get("Content-Range"),
	}, nil
}

// Upload writes src to the full path (single-shot; the 150MB cap matches
// Dropbox's plain upload endpoint — larger payloads go through sync's
// chunked session, which UploadAccount paths don't hit today).
func (b *DropboxBackend) Upload(ctx context.Context, path string, src io.Reader, size int64, contentType string, modTime time.Time) error {
	args := map[string]any{"path": dropboxPath(path), "mode": "overwrite", "mute": true}
	if !modTime.IsZero() {
		args["client_modified"] = modTime.UTC().Format(time.RFC3339)
	}
	arg, _ := json.Marshal(args)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DropboxContentBaseURL+"/files/upload", src)
	if err != nil {
		return err
	}
	req.Header.Set("Dropbox-API-Arg", string(arg))
	req.Header.Set("Content-Type", "application/octet-stream")
	if size >= 0 {
		req.ContentLength = size
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var apiErr dropboxAPIError
		_ = json.Unmarshal(raw, &apiErr)
		return classifyDropboxError(resp.StatusCode, apiErr.ErrorSummary)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return nil
}

// Compile-time interface guard.
var _ Backend = (*DropboxBackend)(nil)
