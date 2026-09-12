package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
)

// --- cloud storage tools: browse/read Drive via rclone ---

// CloudStorageProvider is the narrow surface the storage tools need
// (implemented by *cloud.StorageService, wired in cmd).
type CloudStorageProvider interface {
	List(ctx context.Context, account, path string, max int) ([]storage.ListEntry, error)
	Stat(ctx context.Context, account, path string) (*storage.StatInfo, error)
	About(ctx context.Context, account string) (*storage.AboutInfo, error)
	Fetch(ctx context.Context, account, remotePath, workspaceDir string, sizeCapMB int64) (string, error)
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

// Tools returns the individual tools for registry registration.
func (t *CloudStorageTools) Tools() []Tool {
	return []Tool{
		&cloudLsTool{parent: t},
		&cloudReadTool{parent: t},
		&cloudFetchTool{parent: t},
		&cloudAboutTool{parent: t},
	}
}

// --- cloud_ls ---

type cloudLsTool struct{ parent *CloudStorageTools }

func (t *cloudLsTool) Name() string { return "cloud_ls" }
func (t *cloudLsTool) Description() string {
	return "List a folder in the user's connected Google Drive. Returns name, dir/file, size, mod_time. " +
		"Non-recursive; path \"\" or \"/\" = root."
}
func (t *cloudLsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Folder path (default root)."},
			"account": map[string]any{"type": "string", "description": "Account email (optional with one account)."},
			"max":     map[string]any{"type": "integer", "description": "Max entries (default 100, cap 1000)."},
		},
	}
}
func (t *cloudLsTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	account, _ := args["account"].(string)
	max, _ := args["max"].(float64)
	entries, err := t.parent.provider.List(ctx, account, path, int(max))
	if err != nil {
		return ErrorResult(err.Error())
	}
	if entries == nil {
		entries = []storage.ListEntry{}
	}
	data, _ := json.Marshal(entries)
	return NewResult(string(data))
}

// --- cloud_read ---

type cloudReadTool struct{ parent *CloudStorageTools }

func (t *cloudReadTool) Name() string { return "cloud_read" }
func (t *cloudReadTool) Description() string {
	return "Read a small text file from the user's Google Drive (<=256KB). For binary/large files or to keep a " +
		"copy in the workspace, use cloud_fetch instead."
}
func (t *cloudReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path in Drive."},
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
	// v1 implements read via fetch into a temp workspace subdir; the copy path
	// is the only transfer mechanism rclone rc exposes for files.
	got, err := t.parent.provider.Fetch(ctx, account, path, t.parent.workspace, 1) // 1MB read cap
	if err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult(fmt.Sprintf("fetched to workspace:%s (use read_file on this path)", got))
}

// --- cloud_fetch ---

type cloudFetchTool struct{ parent *CloudStorageTools }

func (t *cloudFetchTool) Name() string { return "cloud_fetch" }
func (t *cloudFetchTool) Description() string {
	return "Download a file from the user's Google Drive into the workspace (cloud/<name>), so read_file and " +
		"other workspace tools can process it. Size-capped; directories must be fetched file by file."
}
func (t *cloudFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path in Drive."},
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
	got, err := t.parent.provider.Fetch(ctx, account, path, t.parent.workspace, t.parent.fetchCapMB)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return NewResult("downloaded to workspace:" + got)
}

// --- cloud_about ---

type cloudAboutTool struct{ parent *CloudStorageTools }

func (t *cloudAboutTool) Name() string { return "cloud_about" }
func (t *cloudAboutTool) Description() string {
	return "Show storage quota (used/total/free) for the user's connected Google Drive."
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
	info, err := t.parent.provider.About(ctx, account)
	if err != nil {
		return ErrorResult(err.Error())
	}
	data, _ := json.Marshal(info)
	return NewResult(string(data))
}
