package methods

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// WS method names for the workspace.* surface (Paseo plan Phase 2,
// plans/GoClaw_Paseo_Web_Chat_Upgrade_Plan.md §23 subset). Declared locally
// until the orchestrator relocates them verbatim into pkg/protocol/methods.go
// next to the other Method* constants and classifies them in the RBAC policy.
const (
	MethodWorkspaceCreate = "workspace.create"
	MethodWorkspaceList   = "workspace.list"
	MethodWorkspaceGet    = "workspace.get"
	MethodWorkspaceUpdate = "workspace.update"
	MethodWorkspaceDelete = "workspace.delete"
)

// WorkspaceMethods implements the workspace.* WS surface: sandboxed working
// directories bound to a tenant/user, optionally backed by a git repo or a
// linked worktree (plan §41/§53).
type WorkspaceMethods struct {
	wsStore store.WorkspaceStore

	// basePath is the sandbox root under which relative rootPath values are
	// resolved and new workspaces are provisioned by default.
	basePath string
}

func NewWorkspaceMethods(wsStore store.WorkspaceStore, basePath string) *WorkspaceMethods {
	return &WorkspaceMethods{wsStore: wsStore, basePath: basePath}
}

// Register wires the workspace.* methods into the method router.
func (m *WorkspaceMethods) Register(router *gateway.MethodRouter) {
	router.Register(MethodWorkspaceCreate, m.handleCreate)
	router.Register(MethodWorkspaceList, m.handleList)
	router.Register(MethodWorkspaceGet, m.handleGet)
	router.Register(MethodWorkspaceUpdate, m.handleUpdate)
	router.Register(MethodWorkspaceDelete, m.handleDelete)
}

// workspaceJSON is the camelCase wire form of store.Workspace.
type workspaceJSON struct {
	ID           string    `json:"id"`
	TenantID     *string   `json:"tenantId"`
	OwnerID      string    `json:"ownerId"`
	Name         string    `json:"name"`
	RootPath     string    `json:"rootPath"`
	Description  *string   `json:"description"`
	Status       string    `json:"status"`
	RepoURL      *string   `json:"repoUrl"`
	Branch       *string   `json:"branch"`
	WorktreePath *string   `json:"worktreePath"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func toWorkspaceJSON(ws *store.Workspace) workspaceJSON {
	return workspaceJSON{
		ID:           ws.ID,
		TenantID:     ws.TenantID,
		OwnerID:      ws.OwnerID,
		Name:         ws.Name,
		RootPath:     ws.RootPath,
		Description:  ws.Description,
		Status:       ws.Status,
		RepoURL:      ws.RepoURL,
		Branch:       ws.Branch,
		WorktreePath: ws.WorktreePath,
		CreatedAt:    ws.CreatedAt,
		UpdatedAt:    ws.UpdatedAt,
	}
}

// sanitizeRootPath resolves the workspace root directory from client input.
// Empty input provisions <basePath>/<workspaceID>. A relative rootPath joins
// basePath; an absolute one is accepted as-is. Any ".." segment is rejected so
// callers cannot climb out of the sandbox base, and the resolved path must be
// absolute after cleaning. ok=false means the value was unsafe.
func sanitizeRootPath(basePath, workspaceID, raw string) (string, bool) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return filepath.Join(basePath, workspaceID), true
	}
	norm := filepath.ToSlash(p)
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return "", false
		}
	}
	cleaned := filepath.Clean(p)
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return "", false
	}
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(basePath, cleaned)
	}
	cleaned = filepath.Clean(cleaned)
	if !filepath.IsAbs(cleaned) {
		return "", false
	}
	return cleaned, true
}

// canManage reports whether the caller may touch this workspace: admins
// always can, everyone else only as the owning user.
func canManageWorkspace(client *gateway.Client, ws *store.Workspace) bool {
	if permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		return true
	}
	return ws.OwnerID != "" && ws.OwnerID == client.UserID()
}

// handleCreate provisions a workspace (workspace.create). Requires operator
// role; viewers are forbidden. Params: { name required, rootPath?, description?,
// repoUrl?, branch? }. An omitted rootPath derives <basePath>/<workspaceID>.
func (m *WorkspaceMethods) handleCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		// The router rejects non-connect frames before connect succeeds; an
		// unset role here is the defensive failure path.
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "workspace.create requires operator role")))
		return
	}
	var params struct {
		Name        string `json:"name"`
		RootPath    string `json:"rootPath"`
		Description string `json:"description"`
		RepoURL     string `json:"repoUrl"`
		Branch      string `json:"branch"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.Name == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "name")))
		return
	}
	if m.wsStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "workspace store not wired"))
		return
	}

	id := uuid.NewString()
	rootPath, ok := sanitizeRootPath(m.basePath, id, params.RootPath)
	if !ok {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "rootPath must be a safe absolute path")))
		return
	}

	ws := &store.Workspace{
		ID:       id,
		OwnerID:  client.UserID(),
		Name:     params.Name,
		RootPath: rootPath,
		Status:   "active",
	}
	if tid := tenantString(client); tid != "" {
		ws.TenantID = &tid
	}
	if params.Description != "" {
		d := params.Description
		ws.Description = &d
	}
	if params.RepoURL != "" {
		u := params.RepoURL
		ws.RepoURL = &u
	}
	if params.Branch != "" {
		b := params.Branch
		ws.Branch = &b
	}
	if err := m.wsStore.CreateWorkspace(ctx, ws); err != nil {
		slog.Warn("workspace.create_failed", "name", params.Name, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "create workspace")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"workspace": toWorkspaceJSON(ws)}))
}

// handleList lists the caller's workspaces (workspace.list), scoped to the
// caller's tenant and user. Archived rows are excluded unless
// includeArchived is set.
func (m *WorkspaceMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if m.wsStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "workspace store not wired"))
		return
	}
	var params struct {
		IncludeArchived bool `json:"includeArchived"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	var tenantID *string
	if tid := tenantString(client); tid != "" {
		tenantID = &tid
	}
	items, err := m.wsStore.ListWorkspaces(ctx, tenantID, client.UserID(), params.IncludeArchived)
	if err != nil {
		slog.Warn("workspace.list_failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "list workspaces")))
		return
	}
	out := make([]workspaceJSON, 0, len(items))
	for _, ws := range items {
		out = append(out, toWorkspaceJSON(ws))
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"workspaces": out}))
}

// workspaceIDParams is the shared { workspaceId } parameter block.
type workspaceIDParams struct {
	WorkspaceID string `json:"workspaceId"`
}

// parseWorkspaceID decodes and validates { workspaceId }, sending the error
// response itself; ok=false means the caller must stop.
func parseWorkspaceID(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) (string, bool) {
	locale := store.LocaleFromContext(ctx)
	var params workspaceIDParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return "", false
		}
	}
	if params.WorkspaceID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "workspaceId")))
		return "", false
	}
	return params.WorkspaceID, true
}

// fetchOwnedWorkspace loads a workspace and enforces ownership: a mismatch
// answers not-found (not forbidden) to avoid leaking existence, matching the
// gateway convention used by sessions/runs/skills.
func (m *WorkspaceMethods) fetchOwnedWorkspace(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, id string) (*store.Workspace, bool) {
	locale := store.LocaleFromContext(ctx)
	notFound := func() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "workspace", id)))
	}
	ws, err := m.wsStore.GetWorkspace(ctx, id)
	if err != nil || ws == nil {
		slog.Debug("workspace.get_failed", "workspace_id", id, "error", err)
		notFound()
		return nil, false
	}
	if !canManageWorkspace(client, ws) {
		notFound()
		return nil, false
	}
	return ws, true
}

// handleGet reads one workspace (workspace.get). Params: { workspaceId }.
func (m *WorkspaceMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if m.wsStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "workspace store not wired"))
		return
	}
	id, ok := parseWorkspaceID(ctx, client, req)
	if !ok {
		return
	}
	ws, ok := m.fetchOwnedWorkspace(ctx, client, req, id)
	if !ok {
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"workspace": toWorkspaceJSON(ws)}))
}

// handleUpdate mutates name/description/status (workspace.update). Params:
// { workspaceId, name?, description?, status? }; status must be active|archived.
func (m *WorkspaceMethods) handleUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if m.wsStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "workspace store not wired"))
		return
	}
	var params struct {
		WorkspaceID string  `json:"workspaceId"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Status      string  `json:"status"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.WorkspaceID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "workspaceId")))
		return
	}
	switch params.Status {
	case "", "active", "archived":
	default:
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "status must be active or archived")))
		return
	}
	ws, ok := m.fetchOwnedWorkspace(ctx, client, req, params.WorkspaceID)
	if !ok {
		return
	}
	if params.Name != nil && *params.Name != "" {
		ws.Name = *params.Name
	}
	if params.Description != nil {
		ws.Description = params.Description
	}
	if params.Status != "" {
		ws.Status = params.Status
	}
	if err := m.wsStore.UpdateWorkspace(ctx, ws); err != nil {
		slog.Warn("workspace.update_failed", "workspace_id", params.WorkspaceID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "update workspace")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"workspace": toWorkspaceJSON(ws)}))
}

// handleDelete removes a workspace permanently (workspace.delete).
// Params: { workspaceId }. Owner or admin only; mismatch reports not-found.
func (m *WorkspaceMethods) handleDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if m.wsStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "workspace store not wired"))
		return
	}
	id, ok := parseWorkspaceID(ctx, client, req)
	if !ok {
		return
	}
	if _, ok := m.fetchOwnedWorkspace(ctx, client, req, id); !ok {
		return
	}
	if err := m.wsStore.DeleteWorkspace(ctx, id); err != nil {
		slog.Warn("workspace.delete_failed", "workspace_id", id, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "delete workspace")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"deleted": true}))
}
