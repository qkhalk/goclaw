package methods

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// WorkspaceFilesMethods implements the workspace.files.* WS surface (Paseo
// plan Phase 3 / §24): lazy per-directory listing plus bounded content reads
// and mutations, all confined to the workspace root. Listing is lazy — the
// client expands one directory at a time; the whole tree is never sent.
// Mutations require operator role; viewers may list/read.
type WorkspaceFilesMethods struct {
	wsStore store.WorkspaceStore
}

func NewWorkspaceFilesMethods(wsStore store.WorkspaceStore) *WorkspaceFilesMethods {
	return &WorkspaceFilesMethods{wsStore: wsStore}
}

// Register wires the workspace.files.* methods into the method router.
func (m *WorkspaceFilesMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodWorkspaceFilesList, m.handleList)
	router.Register(protocol.MethodWorkspaceFilesRead, m.handleRead)
	router.Register(protocol.MethodWorkspaceFilesWrite, m.handleWrite)
	router.Register(protocol.MethodWorkspaceFilesDelete, m.handleDelete)
	router.Register(protocol.MethodWorkspaceFilesMkdir, m.handleMkdir)
}

// fileEntryJSON is one row of a directory listing. Path is root-relative,
// slash-separated regardless of host OS so clients can treat it opaquely.
type fileEntryJSON struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Type    string `json:"type"` // "file" | "dir"
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

const (
	// maxListEntries caps a single directory listing so a huge tree cannot
	// flood the socket; truncated=true tells the client to stop expanding.
	maxListEntries = 2000
	// maxFileBytes caps workspace.files.read payloads (~2 MiB).
	maxFileBytes = 2 << 20
)

// wsFilesParams is the common { workspaceId, path } parameter block.
type wsFilesParams struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

var errEscapesRoot = errors.New("path escapes workspace root")

// safeJoin joins rel (slash-separated, client-facing) onto root and rejects
// any result that escapes the root. Empty rel resolves to the root itself.
func safeJoin(root, rel string) (string, error) {
	p := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(rel)), "/")
	if p == "" {
		return filepath.Clean(root), nil
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", errEscapesRoot
		}
	}
	cleaned := filepath.Clean(filepath.Join(root, filepath.FromSlash(p)))
	if cleaned != root && !strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
		return "", errEscapesRoot
	}
	return cleaned, nil
}

// contained reports whether p is root itself or lives under it.
func contained(root, p string) bool {
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}

// decodeWsFilesParams unmarshals the request params into dst, answering the
// error response itself; ok=false means the caller must stop.
func decodeWsFilesParams(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, dst *wsFilesParams) bool {
	locale := store.LocaleFromContext(ctx)
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, dst); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return false
		}
	}
	if dst.WorkspaceID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "workspaceId")))
		return false
	}
	return true
}

// resolveForFiles loads the target workspace and enforces access. Mutations
// pass needOperator=true; reads allow any authenticated role that can see the
// workspace (admin or owner — same visibility rule as workspace.get).
// root is the effective filesystem root (worktree checkout wins over rootPath).
// abs is root joined with params.Path. ok=false means a response was sent.
func (m *WorkspaceFilesMethods) resolveForFiles(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, p *wsFilesParams, needOperator bool) (root, abs string, ok bool) {
	locale := store.LocaleFromContext(ctx)
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return "", "", false
	}
	if m.wsStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "workspace store not wired"))
		return "", "", false
	}
	if needOperator && !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "workspace.files mutation requires operator role")))
		return "", "", false
	}
	ws, err := m.wsStore.GetWorkspace(ctx, p.WorkspaceID)
	if err != nil || ws == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "workspace", p.WorkspaceID)))
		return "", "", false
	}
	if !canManageWorkspace(client, ws) {
		// Same existence-hiding as workspace.get: answer not-found.
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "workspace", p.WorkspaceID)))
		return "", "", false
	}
	root = ws.RootPath
	if ws.WorktreePath != nil && *ws.WorktreePath != "" {
		// An active worktree checkout supersedes the registry rootPath.
		root = *ws.WorktreePath
	}
	abs, err = safeJoin(root, p.Path)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidPath)))
		return "", "", false
	}
	return root, abs, true
}

// handleList lists ONE directory (workspace.files.list). Params:
// { workspaceId, path? }. Returns { entries, truncated }, directories first.
// Symlinked directories are followed only when the resolved target stays
// inside the workspace root; non-regular files (sockets, fifos, devices) and
// escaping symlinks are omitted.
func (m *WorkspaceFilesMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	var p wsFilesParams
	if !decodeWsFilesParams(ctx, client, req, &p) {
		return
	}
	root, abs, ok := m.resolveForFiles(ctx, client, req, &p, false)
	if !ok {
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			// Listing the root sends an empty p.Path — surface the actual
			// root so the operator sees WHICH directory is missing.
			pathArg := p.Path
			if pathArg == "" {
				pathArg = root
			}
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "directory", pathArg)))
		} else {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "stat")))
		}
		return
	}
	if !info.IsDir() {
		// A file path was given: list its parent for convenience.
		abs = filepath.Dir(abs)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "read directory")))
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		ei, ej := entries[i], entries[j]
		if ei.IsDir() != ej.IsDir() {
			return ei.IsDir() // directories first
		}
		return ei.Name() < ej.Name()
	})
	truncated := len(entries) > maxListEntries
	if truncated {
		entries = entries[:maxListEntries]
	}
	out := make([]fileEntryJSON, 0, len(entries))
	for _, e := range entries {
		full := filepath.Join(abs, e.Name())
		info, err := os.Stat(full) // follows symlink for size/mtime/type
		if err != nil {
			continue // vanished mid-scan or unreadable: omit silently
		}
		switch {
		case info.IsDir():
			resolved, err := filepath.EvalSymlinks(full)
			if err != nil || !contained(root, resolved) {
				continue // symlink escaping the root: hide it entirely
			}
		case e.Type().IsRegular():
			// plain file: keep
		default:
			continue // sockets, fifos, devices: never expose
		}
		entryPath := ""
		if rel, err := filepath.Rel(abs, full); err == nil {
			entryPath = filepath.ToSlash(rel)
		} else {
			entryPath = e.Name()
		}
		typ := "file"
		if info.IsDir() {
			typ = "dir"
		}
		out = append(out, fileEntryJSON{
			Name:    e.Name(),
			Path:    entryPath,
			Type:    typ,
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"entries": out, "truncated": truncated}))
}

// handleRead returns file content (workspace.files.read). Params:
// { workspaceId, path }. Files larger than maxFileBytes are rejected — the
// client should page through large artifacts via the agent instead.
func (m *WorkspaceFilesMethods) handleRead(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	var p wsFilesParams
	if !decodeWsFilesParams(ctx, client, req, &p) {
		return
	}
	root, abs, ok := m.resolveForFiles(ctx, client, req, &p, false)
	if !ok {
		return
	}
	if _, err := os.Lstat(abs); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "file", p.Path)))
		return
	}
	// Symlinks may only be read when the resolved target stays inside the
	// workspace root (EvalSymlinks resolves every path component).
	resolved, rerr := filepath.EvalSymlinks(abs)
	if rerr != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "file", p.Path)))
		return
	}
	if !contained(root, resolved) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidPath)))
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "stat")))
		return
	}
	if info.IsDir() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidPath)))
		return
	}
	if info.Size() > maxFileBytes {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInternalError, "file too large")))
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "read file")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"path":    strings.TrimPrefix(filepath.ToSlash(p.Path), "/"),
		"content": string(data),
		"size":    info.Size(),
	}))
}

// handleWrite writes file content (workspace.files.write, operator-only).
// Params: { workspaceId, path, content }. Parent directories are created;
// paths resolving outside the root were already rejected by resolveForFiles.
func (m *WorkspaceFilesMethods) handleWrite(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	var p struct {
		wsFilesParams
		Content string `json:"content"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if p.WorkspaceID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "workspaceId")))
		return
	}
	_, abs, ok := m.resolveForFiles(ctx, client, req, &p.wsFilesParams, true)
	if !ok {
		return
	}
	if d := filepath.Dir(abs); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "create parent directory")))
			return
		}
	}
	if err := os.WriteFile(abs, []byte(p.Content), 0o644); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "write file")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"written": true}))
}

// handleDelete removes a file or empty directory (workspace.files.delete,
// operator-only). Params: { workspaceId, path }. Recursive deletion is
// deliberately unsupported over RPC.
func (m *WorkspaceFilesMethods) handleDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	var p wsFilesParams
	if !decodeWsFilesParams(ctx, client, req, &p) {
		return
	}
	_, abs, ok := m.resolveForFiles(ctx, client, req, &p, true)
	if !ok {
		return
	}
	if strings.TrimSpace(p.Path) == "" {
		// Deleting the bare workspace root makes no sense; require a path.
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "path")))
		return
	}
	if err := os.Remove(abs); err != nil {
		switch {
		case os.IsNotExist(err):
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "path", p.Path)))
		case errors.Is(err, fs.ErrPermission):
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "permission denied")))
		default:
			// Non-empty directory lands here too.
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "delete")))
		}
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"deleted": true}))
}

// handleMkdir creates a directory hierarchy (workspace.files.mkdir,
// operator-only). Params: { workspaceId, path }. Equivalent to mkdir -p.
func (m *WorkspaceFilesMethods) handleMkdir(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	var p wsFilesParams
	if !decodeWsFilesParams(ctx, client, req, &p) {
		return
	}
	if strings.TrimSpace(p.Path) == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "path")))
		return
	}
	_, abs, ok := m.resolveForFiles(ctx, client, req, &p, true)
	if !ok {
		return
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "create directory")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"created": true}))
}
