package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/cloud"
	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- cloud storage tools: browse/read/write Drive via native backends ---

// CloudStorageProvider is the surface the storage tools need (implemented by
// *cloud.StorageService, wired in cmd). Every operation resolves the account
// through AgentAccount first, which enforces the per-account agent access
// level (none | read | write | full) set by tenant admins on the Clouds page.
type CloudStorageProvider interface {
	// AgentAccount resolves + permission-checks the account for agent use.
	AgentAccount(ctx context.Context, account string, min cloud.AgentAccess) (*store.CloudAccount, error)
	ListAccount(ctx context.Context, acct *store.CloudAccount, path string, max int) ([]storage.ListEntry, error)
	AboutAccount(ctx context.Context, acct *store.CloudAccount) (*storage.AboutInfo, error)
	FetchAccount(ctx context.Context, acct *store.CloudAccount, remotePath, workspaceDir string, sizeCapMB int64) (string, error)
	MkdirAccount(ctx context.Context, acct *store.CloudAccount, dir string) error
	WriteAccount(ctx context.Context, acct *store.CloudAccount, remotePath, content string) error
	UploadAccount(ctx context.Context, acct *store.CloudAccount, localDir, localName, remotePath string) error
	CopyAccount(ctx context.Context, acct *store.CloudAccount, from, to string) error
	MoveAccount(ctx context.Context, acct *store.CloudAccount, from, to string) error
	DeleteAccount(ctx context.Context, acct *store.CloudAccount, path string, isDir bool) error
	PublicLinkAccount(ctx context.Context, acct *store.CloudAccount, path string) (*storage.PublicLinkInfo, error)
}

// CloudStorageTools groups the storage agent tools.
type CloudStorageTools struct {
	provider   CloudStorageProvider
	workspace  string
	fetchCapMB int64
}

// NewCloudStorageTools builds the storage toolset.
func NewCloudStorageTools(provider CloudStorageProvider, workspace string, fetchCapMB int64) *CloudStorageTools {
	if fetchCapMB <= 0 {
		fetchCapMB = 100
	}
	return &CloudStorageTools{provider: provider, workspace: workspace, fetchCapMB: fetchCapMB}
}

// callerWorkspace resolves the calling session's layered workspace (per
// agent/channel/user) so fetched files land where the session's file tools
// can actually reach them; the instance workspace is only the fallback.
func (t *CloudStorageTools) callerWorkspace(ctx context.Context) string {
	if ws := ToolWorkspaceFromCtx(ctx); ws != "" {
		return ws
	}
	return t.workspace
}

// Tools returns the individual tools for registry registration.
func (t *CloudStorageTools) Tools() []Tool {
	return []Tool{
		&cloudLsTool{parent: t},
		&cloudReadTool{parent: t},
		&cloudFetchTool{parent: t},
		&cloudAboutTool{parent: t},
		&cloudWriteTool{parent: t},
		&cloudUploadTool{parent: t},
		&cloudMkdirTool{parent: t},
		&cloudCopyTool{parent: t},
		&cloudMoveTool{parent: t},
		&cloudDeleteTool{parent: t},
		&cloudShareTool{parent: t},
	}
}

// resolve is the shared preflight: resolve the account for THIS agent and
// enforce the minimum access level before any provider call.
func (t *CloudStorageTools) resolve(ctx context.Context, account string, min cloud.AgentAccess) (*store.CloudAccount, *Result) {
	acct, err := t.provider.AgentAccount(ctx, account, min)
	if err != nil {
		return nil, ErrorResult(err.Error())
	}
	return acct, nil
}

// --- cloud_ls (read) ---

type cloudLsTool struct{ parent *CloudStorageTools }

func (t *cloudLsTool) Name() string { return "cloud_ls" }
func (t *cloudLsTool) Description() string {
	return "List a folder in a connected cloud drive (Google Drive / OneDrive). Returns name, dir/file, size, mod_time. " +
		"Non-recursive; path \"\" or \"/\" = root."
}
func (t *cloudLsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Folder path (default root)."},
			"account": map[string]any{"type": "string", "description": "Account email (from cloud_accounts; optional with one account)."},
			"max":     map[string]any{"type": "integer", "description": "Max entries (default 100, cap 1000)."},
		},
	}
}
func (t *cloudLsTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	account, _ := args["account"].(string)
	max, _ := args["max"].(float64)
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessRead)
	if errResult != nil {
		return errResult
	}
	entries, err := t.parent.provider.ListAccount(ctx, acct, path, int(max))
	if err != nil {
		return ErrorResult(err.Error())
	}
	if entries == nil {
		entries = []storage.ListEntry{}
	}
	data, _ := json.Marshal(entries)
	return NewResult(string(data))
}

// --- cloud_read (read) ---

type cloudReadTool struct{ parent *CloudStorageTools }

func (t *cloudReadTool) Name() string { return "cloud_read" }
func (t *cloudReadTool) Description() string {
	return "Read a small text file (<=256KB) from a connected cloud drive into the workspace, then use read_file on it. " +
		"For binary/large files use cloud_fetch instead."
}
func (t *cloudReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path in the drive."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"path"},
	}
}
func (t *cloudReadTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessRead)
	if errResult != nil {
		return errResult
	}
	got, err := t.parent.provider.FetchAccount(ctx, acct, path, t.parent.callerWorkspace(ctx), 1) // 1MB read cap
	if err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult(fmt.Sprintf("fetched to workspace:%s (use read_file on this path)", got))
}

// --- cloud_fetch (read) ---

type cloudFetchTool struct{ parent *CloudStorageTools }

func (t *cloudFetchTool) Name() string { return "cloud_fetch" }
func (t *cloudFetchTool) Description() string {
	return "Download a file from a connected cloud drive into the workspace (cloud/<name>), so read_file and " +
		"other workspace tools can process it. Size-capped; directories must be fetched file by file."
}
func (t *cloudFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path in the drive."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"path"},
	}
}
func (t *cloudFetchTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessRead)
	if errResult != nil {
		return errResult
	}
	got, err := t.parent.provider.FetchAccount(ctx, acct, path, t.parent.callerWorkspace(ctx), t.parent.fetchCapMB)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult("downloaded to workspace:" + got)
}

// --- cloud_about (read) ---

type cloudAboutTool struct{ parent *CloudStorageTools }

func (t *cloudAboutTool) Name() string { return "cloud_about" }
func (t *cloudAboutTool) Description() string {
	return "Show storage quota (used/total/free) for a connected cloud drive."
}
func (t *cloudAboutTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
	}
}
func (t *cloudAboutTool) Execute(ctx context.Context, args map[string]any) *Result {
	account, _ := args["account"].(string)
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessRead)
	if errResult != nil {
		return errResult
	}
	info, err := t.parent.provider.AboutAccount(ctx, acct)
	if err != nil {
		return ErrorResult(err.Error())
	}
	data, _ := json.Marshal(info)
	return NewResult(string(data))
}

// --- cloud_write (write) ---

type cloudWriteTool struct{ parent *CloudStorageTools }

func (t *cloudWriteTool) Name() string { return "cloud_write" }
func (t *cloudWriteTool) Description() string {
	return "Create or overwrite a text file in a connected cloud drive (e.g. notes, reports, configs). " +
		"Requires write agent access on the account. The full destination path including the file name is required."
}
func (t *cloudWriteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Full destination file path (folder + name)."},
			"content": map[string]any{"type": "string", "description": "Text content to write (replaces any existing file)."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"path", "content"},
	}
}
func (t *cloudWriteTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessWrite)
	if errResult != nil {
		return errResult
	}
	if err := t.parent.provider.WriteAccount(ctx, acct, path, content); err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult(fmt.Sprintf("wrote %d bytes to %s", len(content), path))
}

// --- cloud_upload (write; binary-safe) ---

type cloudUploadTool struct{ parent *CloudStorageTools }

func (t *cloudUploadTool) Name() string { return "cloud_upload" }
func (t *cloudUploadTool) Description() string {
	return "Upload a local workspace file to a connected cloud drive — binary-safe " +
		"(videos, images, audio, archives, PDFs; anything, not just text). " +
		"Requires write agent access on the account. The full destination path " +
		"including the file name is required."
}
func (t *cloudUploadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":        map[string]any{"type": "string", "description": "Workspace path of the local file to upload."},
			"remote_path": map[string]any{"type": "string", "description": "Full destination path on the drive (folder + name)."},
			"account":     map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"path", "remote_path"},
	}
}

func (t *cloudUploadTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	remotePath, _ := args["remote_path"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}
	if strings.TrimSpace(remotePath) == "" {
		return ErrorResult("remote_path is required")
	}
	// Workspace-boundary safe: reject symlink/traversal escapes before any I/O.
	localPath, err := resolvePath(path, t.parent.callerWorkspace(ctx), true)
	if err != nil {
		return ErrorResult(err.Error())
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("local file not found: %s", path))
	}
	if !info.Mode().IsRegular() {
		return ErrorResult(fmt.Sprintf("%s is not a regular file", path))
	}
	if t.parent.fetchCapMB > 0 && info.Size() > t.parent.fetchCapMB<<20 {
		return ErrorResult(fmt.Sprintf("file is %d MB — over the %d MB upload cap", info.Size()>>20, t.parent.fetchCapMB))
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessWrite)
	if errResult != nil {
		return errResult
	}
	if err := t.parent.provider.UploadAccount(ctx, acct, filepath.Dir(localPath), filepath.Base(localPath), remotePath); err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult(fmt.Sprintf("uploaded %s (%d bytes) to %s", filepath.Base(localPath), info.Size(), remotePath))
}

// --- cloud_mkdir (write) ---

type cloudMkdirTool struct{ parent *CloudStorageTools }

func (t *cloudMkdirTool) Name() string { return "cloud_mkdir" }
func (t *cloudMkdirTool) Description() string {
	return "Create a folder in a connected cloud drive. Requires write agent access on the account."
}
func (t *cloudMkdirTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Folder path to create."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"path"},
	}
}
func (t *cloudMkdirTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessWrite)
	if errResult != nil {
		return errResult
	}
	if err := t.parent.provider.MkdirAccount(ctx, acct, path); err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult("created folder " + path)
}

// --- cloud_copy (write) ---

type cloudCopyTool struct{ parent *CloudStorageTools }

func (t *cloudCopyTool) Name() string { return "cloud_copy" }
func (t *cloudCopyTool) Description() string {
	return "Copy a file to another path within one connected cloud drive (server-side). Requires write agent access."
}
func (t *cloudCopyTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from":    map[string]any{"type": "string", "description": "Source file path."},
			"to":      map[string]any{"type": "string", "description": "Destination file path (folder + name)."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"from", "to"},
	}
}
func (t *cloudCopyTool) Execute(ctx context.Context, args map[string]any) *Result {
	from, _ := args["from"].(string)
	to, _ := args["to"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return ErrorResult("from and to are required")
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessWrite)
	if errResult != nil {
		return errResult
	}
	if err := t.parent.provider.CopyAccount(ctx, acct, from, to); err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult("copied " + from + " to " + to)
}

// --- cloud_move (full) ---

type cloudMoveTool struct{ parent *CloudStorageTools }

func (t *cloudMoveTool) Name() string { return "cloud_move" }
func (t *cloudMoveTool) Description() string {
	return "Rename or move a file within one connected cloud drive. Requires full agent access on the account."
}
func (t *cloudMoveTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from":    map[string]any{"type": "string", "description": "Source file path."},
			"to":      map[string]any{"type": "string", "description": "Destination path (folder + name)."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"from", "to"},
	}
}
func (t *cloudMoveTool) Execute(ctx context.Context, args map[string]any) *Result {
	from, _ := args["from"].(string)
	to, _ := args["to"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return ErrorResult("from and to are required")
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessFull)
	if errResult != nil {
		return errResult
	}
	if err := t.parent.provider.MoveAccount(ctx, acct, from, to); err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult("moved " + from + " to " + to)
}

// --- cloud_delete (full) ---

type cloudDeleteTool struct{ parent *CloudStorageTools }

func (t *cloudDeleteTool) Name() string { return "cloud_delete" }
func (t *cloudDeleteTool) Description() string {
	return "Delete one file, or one EMPTY folder, from a connected cloud drive. PERMANENT on Google Drive (no trash). " +
		"Requires full agent access; confirm with the user (ask_options) before deleting anything they have not explicitly named."
}
func (t *cloudDeleteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File or (empty) folder path."},
			"is_dir":  map[string]any{"type": "boolean", "description": "true when deleting an empty folder."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"path"},
	}
}
func (t *cloudDeleteTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	isDir, _ := args["is_dir"].(bool)
	account, _ := args["account"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessFull)
	if errResult != nil {
		return errResult
	}
	if err := t.parent.provider.DeleteAccount(ctx, acct, path, isDir); err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult("deleted " + path)
}

// --- cloud_share (full) ---

type cloudShareTool struct{ parent *CloudStorageTools }

func (t *cloudShareTool) Name() string { return "cloud_share" }
func (t *cloudShareTool) Description() string {
	return "Create or retrieve the PUBLIC share link for a file in a connected cloud drive. The link is anonymous and " +
		"does not expire on its own — always confirm with the user (ask_options) before sharing. Requires full agent access."
}
func (t *cloudShareTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path to share."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"path"},
	}
}
func (t *cloudShareTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}
	acct, errResult := t.parent.resolve(ctx, account, cloud.AgentAccessFull)
	if errResult != nil {
		return errResult
	}
	link, err := t.parent.provider.PublicLinkAccount(ctx, acct, path)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult(link.URL)
}
