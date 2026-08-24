package store

import (
	"context"
	"time"
)

// Task graph node lifecycle status values (Paseo plan §22): pending work
// waits to start, running nodes are actively executing, blocked nodes wait
// on a dependency, and done/failed/cancelled are terminal.
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusBlocked   = "blocked"
	TaskStatusDone      = "done"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
)

// TaskNode is one node in the lightweight parent/child task tree kept per
// workspace (Paseo plan §22). Parent/child edges form the hierarchy;
// DependsOn holds ids of other nodes that must finish first — it is stored
// as a JSON array of task id strings (JSONB in PG, TEXT in SQLite).
//
// TenantID == nil means master/global scope — the same convention as
// node_leases, api_keys, and workspaces.
type TaskNode struct {
	ID           string // canonical task id, uuid string
	TenantID     *string
	WorkspaceID  *string
	ParentID     *string // nil for roots; children cascade on delete
	OwnerAgentID *string
	SessionKey   *string
	Title        string
	Status       string
	Priority     int
	DependsOn    []string
	ResultRef    *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ValidTaskStatus reports whether s is a known task node status.
func ValidTaskStatus(s string) bool {
	switch s {
	case TaskStatusPending, TaskStatusRunning, TaskStatusBlocked,
		TaskStatusDone, TaskStatusFailed, TaskStatusCancelled:
		return true
	}
	return false
}

// TaskGraphStore persists the hierarchical task tree with dependencies.
// Implementations must scope reads and writes to the tenant from context
// where the row carries one; nil TenantID rows are master/global and only
// reachable from master scope.
type TaskGraphStore interface {
	// CreateTask inserts a new task node. The ID, timestamps and status are
	// defaulted by the implementation when zero-valued. DependsOn may be nil
	// (stored as an empty JSON array).
	CreateTask(ctx context.Context, t *TaskNode) error
	// GetTask resolves a task by canonical id. Returns sql.ErrNoRows when
	// the task does not exist or is not visible from the caller's tenant
	// scope.
	GetTask(ctx context.Context, id string) (*TaskNode, error)
	// ListTasks returns the whole task tree for a workspace: roots first
	// (parent_id IS NULL), then deeper levels grouped under their parents,
	// priority DESC within each group, oldest created_at first inside a
	// group so sibling order is stable.
	ListTasks(ctx context.Context, workspaceID string) ([]*TaskNode, error)
	// UpdateTaskStatus transitions a task's status and optionally records a
	// result reference (result_ref is left untouched when resultRef is nil).
	// The target status must be a known status; updated_at is refreshed.
	UpdateTaskStatus(ctx context.Context, id, status string, resultRef *string) error
	// DeleteTask hard-deletes a task node; child nodes cascade via the
	// parent_id self-FK. Returns an error wrapping sql.ErrNoRows when
	// nothing matched the caller's scope.
	DeleteTask(ctx context.Context, id string) error
}
