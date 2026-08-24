package store

import (
	"context"
	"time"
)

// Workspace lifecycle status values. Archived workspaces stay on disk but
// disappear from default listings until restored.
const (
	WorkspaceStatusActive   = "active"
	WorkspaceStatusArchived = "archived"
)

// Workspace is a sandboxed root directory owned by a user within a tenant
// scope (Paseo plan Phase 2, plan §3/§41), optionally bound to a git
// repository/branch and/or a linked git worktree checkout (plan §53).
//
// TenantID == nil means master/global scope — the same convention as
// node_leases and api_keys (identity != connection != workspace).
type Workspace struct {
	ID           string // canonical workspace_id, uuid string
	TenantID     *string
	OwnerID      string
	Name         string
	RootPath     string
	Description  *string
	Status       string
	RepoURL      *string
	Branch       *string
	WorktreePath *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ValidWorkspaceStatus reports whether s is a known workspace status.
func ValidWorkspaceStatus(s string) bool {
	switch s {
	case WorkspaceStatusActive, WorkspaceStatusArchived:
		return true
	}
	return false
}

// WorkspaceStore persists workspaces. Implementations must scope reads and
// writes to the tenant from context where the row carries one; nil TenantID
// rows are master/global and only reachable from master scope.
type WorkspaceStore interface {
	// CreateWorkspace inserts a new workspace. The ID, timestamps and status
	// are defaulted by the implementation when zero-valued.
	CreateWorkspace(ctx context.Context, ws *Workspace) error
	// GetWorkspace resolves a workspace by canonical id. Returns
	// sql.ErrNoRows when the workspace does not exist or is not visible from
	// the caller's tenant scope.
	GetWorkspace(ctx context.Context, id string) (*Workspace, error)
	// ListWorkspaces returns the caller's workspaces ordered by updated_at
	// DESC. tenantID selects the scope (nil = master/global rows); archived
	// rows are excluded unless includeArchived is set.
	ListWorkspaces(ctx context.Context, tenantID *string, ownerID string, includeArchived bool) ([]*Workspace, error)
	// UpdateWorkspace persists mutable fields (name/description/status plus
	// the git binding columns). The updated_at column is refreshed by the
	// implementation.
	UpdateWorkspace(ctx context.Context, ws *Workspace) error
	// DeleteWorkspace hard-deletes a workspace row; callers enforce admin
	// authorization. Returns sql.ErrNoRows when nothing matched the caller's
	// scope.
	DeleteWorkspace(ctx context.Context, id string) error
}
