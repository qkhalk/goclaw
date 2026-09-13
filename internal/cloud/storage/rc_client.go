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
)

// RCClient is a minimal rclone rc API client (POST JSON, basic auth).
// Only the operations GoClaw needs are wrapped; core/command and config/dump
// are deliberately absent (shell-equivalent / credential-leaking endpoints).
type RCClient struct {
	base string
	user string
	pass string
	http *http.Client
}

// NewRCClient creates an rc client for an rcd endpoint.
func NewRCClient(base, user, pass string) *RCClient {
	return &RCClient{
		base: strings.TrimRight(base, "/"),
		user: user,
		pass: pass,
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// do performs an rc call: POST /<path> with a JSON object body.
func (c *RCClient) do(ctx context.Context, path string, params map[string]any, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	buf, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rclone rc %s status %d: %s", path, resp.StatusCode, truncate(string(data), 200))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// CoreVersion probes liveness (core/version).
func (c *RCClient) CoreVersion(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.do(ctx, "core/version", nil, &out)
	return out, err
}

// ConfigListRemotes returns configured remote names.
func (c *RCClient) ConfigListRemotes(ctx context.Context) ([]string, error) {
	var out struct {
		Remotes []string `json:"remotes"`
	}
	err := c.do(ctx, "config/listremotes", nil, &out)
	return out.Remotes, err
}

// ConfigCreate creates a remote (rc config/create). parameters carries the
// backend options — they must be NESTED under the "parameters" key per the
// rc API, not sent flat: {"name", "type", "parameters": {token, client_id,
// ...}, "opt": {...}}.
func (c *RCClient) ConfigCreate(ctx context.Context, name, remoteType string, parameters map[string]any) error {
	return c.do(ctx, "config/create", map[string]any{
		"name":       name,
		"type":       remoteType,
		"parameters": parameters,
		"opt":        map[string]any{"obscure": true, "no_obscure": false},
	}, nil)
}

// ConfigDelete removes a remote.
func (c *RCClient) ConfigDelete(ctx context.Context, name string) error {
	return c.do(ctx, "config/delete", map[string]any{"name": name}, nil)
}

// ListEntry is one row of operations/list output.
type ListEntry struct {
	Name    string `json:"Name"`
	IsDir   bool   `json:"IsDir"`
	Size    int64  `json:"Size"`
	ModTime string `json:"ModTime"`
}

// remoteSpec joins a remote name and path into the fs spec rclone expects:
// "<remote>:" for the root (an empty/"/" remote) or "<remote>:<path>".
func remoteSpec(fs, remote string) string {
	remote = strings.Trim(remote, "/")
	if remote == "" {
		return fs + ":"
	}
	return fs + ":" + remote
}

// OperationsList lists a remote path (non-recursive by default).
func (c *RCClient) OperationsList(ctx context.Context, fs, remote string, maxEntries int) ([]ListEntry, error) {
	if maxEntries <= 0 || maxEntries > 1000 {
		maxEntries = 100
	}
	var out struct {
		List []ListEntry `json:"list"`
	}
	err := c.do(ctx, "operations/list", map[string]any{
		"fs":     fs + ":",
		"remote": strings.Trim(remote, "/"),
		"opt":    map[string]any{"recurse": false, "maxDepth": 1, "limit": maxEntries},
	}, &out)
	return out.List, err
}

// StatInfo is operations/stat output.
type StatInfo struct {
	Name    string `json:"Name"`
	Size    int64  `json:"Size"`
	MimeType string `json:"MimeType"`
	ModTime string `json:"ModTime"`
	IsDir   bool   `json:"IsDir"`
}

// OperationsStat stats one path.
func (c *RCClient) OperationsStat(ctx context.Context, fs, remote string) (*StatInfo, error) {
	var out struct {
		Item *StatInfo `json:"item"`
	}
	if err := c.do(ctx, "operations/stat", map[string]any{"fs": fs + ":", "remote": strings.Trim(remote, "/")}, &out); err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, fmt.Errorf("not found: %s%s", fs, remote)
	}
	return out.Item, nil
}

// AboutInfo is operations/about (quota) output.
type AboutInfo struct {
	Total int64 `json:"Total"`
	Used  int64 `json:"Used"`
	Free  int64 `json:"Free"`
}

// OperationsAbout returns storage quota for a remote.
func (c *RCClient) OperationsAbout(ctx context.Context, fs string) (*AboutInfo, error) {
	var out AboutInfo
	if err := c.do(ctx, "operations/about", map[string]any{"fs": remoteSpec(fs, "")}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// OperationsCopyFile copies one file between remotes (drive: → local temp or
// workspace). Both fs and remoteSrc/remoteDst are rclone path pairs.
func (c *RCClient) OperationsCopyFile(ctx context.Context, srcFS, srcRemote, dstFS, dstRemote string) error {
	return c.do(ctx, "operations/copyfile", map[string]any{
		"srcFs": srcFS, "srcRemote": srcRemote,
		"dstFs": dstFS, "dstRemote": dstRemote,
	}, nil)
}

// Single-path rc operations (mkdir/rmdir/deletefile/publiclink) take the
// remote name and the path as SEPARATE params: fs="<remote>:" (the root fs)
// and remote="<path>". Composing the path into the fs string instead breaks
// file paths: rclone's fs cache returns fs.ErrorIsFile for them and the rc
// handler fails before reaching the operation (rc.GetFsAndRemote semantics).

// OperationsMkdir creates the directory <fs>:<remote>.
func (c *RCClient) OperationsMkdir(ctx context.Context, fs, remote string) error {
	return c.do(ctx, "operations/mkdir", map[string]any{
		"fs": fs + ":", "remote": strings.Trim(remote, "/"),
	}, nil)
}

// OperationsRmdir removes an EMPTY directory <fs>:<remote> (rclone refuses
// non-empty dirs — deleting a populated tree is deliberately not exposed).
func (c *RCClient) OperationsRmdir(ctx context.Context, fs, remote string) error {
	return c.do(ctx, "operations/rmdir", map[string]any{
		"fs": fs + ":", "remote": strings.Trim(remote, "/"),
	}, nil)
}

// OperationsDeleteFile removes the single file <fs>:<remote> — PERMANENT on
// Drive (no trash via rc). Callers must have confirmed with the user.
func (c *RCClient) OperationsDeleteFile(ctx context.Context, fs, remote string) error {
	return c.do(ctx, "operations/deletefile", map[string]any{
		"fs": fs + ":", "remote": strings.Trim(remote, "/"),
	}, nil)
}

// OperationsMoveFile moves/renames one file (rename = move within the same
// fs). Same path-pair convention as OperationsCopyFile.
func (c *RCClient) OperationsMoveFile(ctx context.Context, srcFS, srcRemote, dstFS, dstRemote string) error {
	return c.do(ctx, "operations/movefile", map[string]any{
		"srcFs": srcFS, "srcRemote": srcRemote,
		"dstFs": dstFS, "dstRemote": dstRemote,
	}, nil)
}

// OperationsCopyURL fetches url (server side, via rclone) into the file
// <dstFS>:<dstRemote>. The destination remote is the FULL file path including
// name; the fs param is the bare remote root.
func (c *RCClient) OperationsCopyURL(ctx context.Context, url, dstFS, dstRemote string) error {
	return c.do(ctx, "operations/copyurl", map[string]any{
		"url": url,
		"fs":  dstFS + ":", "remote": dstRemote,
	}, nil)
}

// PublicLinkInfo is operations/publiclink output.
type PublicLinkInfo struct {
	URL string `json:"url"`
}

// OperationsPublicLink creates (or retrieves) the public share link for
// <fs>:<remote>. The link is public and unexpired until the provider says
// otherwise — handlers must treat this as a write-level operation.
func (c *RCClient) OperationsPublicLink(ctx context.Context, fs, remote string) (*PublicLinkInfo, error) {
	var out PublicLinkInfo
	if err := c.do(ctx, "operations/publiclink", map[string]any{
		"fs": fs + ":", "remote": strings.Trim(remote, "/"),
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SyncCopy copies srcFS:srcRemote → dstFS:dstRemote skipping identical files.
// sync/copy takes ONLY srcFs/dstFs (no remote params — the path is part of
// the fs spec, rc.GetFsNamed semantics); passing the path separately would
// silently sync the remote's ENTIRE root. async=true queues the job in rcd
// (mandatory for big trees — the rc client times out at 60s) and returns the
// rclone job ID (0 when synchronous).
func (c *RCClient) SyncCopy(ctx context.Context, srcFS, srcRemote, dstFS, dstRemote string, async bool) (int64, error) {
	var out struct {
		JobID int64 `json:"jobid"`
	}
	err := c.do(ctx, "sync/copy", map[string]any{
		"srcFs": remoteSpec(srcFS, srcRemote),
		"dstFs": remoteSpec(dstFS, dstRemote),
		"_async": async,
	}, &out)
	return out.JobID, err
}

// JobInfo is job/status output (fields GoClaw polls on).
type JobInfo struct {
	ID       int64  `json:"id"`
	Finished bool   `json:"finished"`
	Success  bool   `json:"success"`
	Error    string `json:"error"`
}

// JobStatus polls one async rclone job.
func (c *RCClient) JobStatus(ctx context.Context, jobID int64) (*JobInfo, error) {
	var out JobInfo
	if err := c.do(ctx, "job/status", map[string]any{"jobid": jobID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
