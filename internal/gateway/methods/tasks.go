package methods

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// NOTE(orchestrator): these method-name constants are declared locally for the
// phase 2 wave 2 cutover and will be relocated to pkg/protocol (same flow as
// workspace.go in wave 1).
const (
	MethodTasksTree         = "tasks.tree"
	MethodTasksCreate       = "tasks.create"
	MethodTasksUpdateStatus = "tasks.updateStatus"
)

// validTaskStatuses is the closed set accepted by tasks.updateStatus.
var validTaskStatuses = map[string]bool{
	"pending":   true,
	"running":   true,
	"blocked":   true,
	"done":      true,
	"failed":    true,
	"cancelled": true,
}

// TasksMethods implements the tasks.* WS surface over the task graph
// (plan §22): reading the per-workspace dependency tree plus operator-gated
// creation and status updates.
type TasksMethods struct {
	tasks store.TaskGraphStore
}

func NewTasksMethods(tasks store.TaskGraphStore) *TasksMethods {
	return &TasksMethods{tasks: tasks}
}

// Register wires the tasks.* methods into the method router.
func (m *TasksMethods) Register(router *gateway.MethodRouter) {
	router.Register(MethodTasksTree, m.handleTree)
	router.Register(MethodTasksCreate, m.handleCreate)
	router.Register(MethodTasksUpdateStatus, m.handleUpdateStatus)
}

// taskJSON is the camelCase wire form of store.TaskNode.
type taskJSON struct {
	ID           string    `json:"id"`
	TenantID     *string   `json:"tenantId"`
	WorkspaceID  *string   `json:"workspaceId"`
	ParentID     *string   `json:"parentId"`
	OwnerAgentID *string   `json:"ownerAgentId"`
	SessionKey   *string   `json:"sessionKey"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	Priority     int       `json:"priority"`
	DependsOn    []string  `json:"dependsOn"`
	ResultRef    *string   `json:"resultRef"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// handleTree returns the whole task tree for a workspace (tasks.tree).
// Params: { workspaceId }. Ordering (roots first, priority DESC, created ASC)
// is owned by the store.
func (m *TasksMethods) handleTree(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	if m.tasks == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "task graph store not wired"))
		return
	}
	workspaceID, ok := parseWorkspaceID(ctx, client, req)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(ctx)
	items, err := m.tasks.ListTasks(ctx, workspaceID)
	if err != nil {
		slog.Warn("tasks.tree_failed", "workspace_id", workspaceID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "list tasks")))
		return
	}
	out := make([]taskJSON, 0, len(items))
	for _, t := range items {
		out = append(out, toTaskJSON(t))
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"tasks": out}))
}

// handleCreate appends a node to the task graph (tasks.create). Requires
// operator role; viewers are forbidden. Params:
// { title required, workspaceId?, parentId?, priority?, dependsOn?[] }.
// The caller-scoped tenant is stamped from the client, matching
// workspace.create.
func (m *TasksMethods) handleCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tasks.create requires operator role")))
		return
	}
	var params struct {
		WorkspaceID string   `json:"workspaceId"`
		ParentID    string   `json:"parentId"`
		Title       string   `json:"title"`
		Priority    int      `json:"priority"`
		DependsOn   []string `json:"dependsOn"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.Title == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "title")))
		return
	}
	if len(params.DependsOn) == 0 {
		params.DependsOn = []string{}
	}
	if m.tasks == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "task graph store not wired"))
		return
	}
	task := &store.TaskNode{
		ID:        uuid.NewString(),
		Title:     params.Title,
		Status:    "pending",
		Priority:  params.Priority,
		DependsOn: params.DependsOn,
	}
	if tid := tenantString(client); tid != "" {
		task.TenantID = &tid
	}
	if params.WorkspaceID != "" {
		wid := params.WorkspaceID
		task.WorkspaceID = &wid
	}
	if params.ParentID != "" {
		pid := params.ParentID
		task.ParentID = &pid
	}
	if err := m.tasks.CreateTask(ctx, task); err != nil {
		slog.Warn("tasks.create_failed", "title", params.Title, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "create task")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"task": toTaskJSON(task)}))
}

// handleUpdateStatus mutates a node status (tasks.updateStatus). Requires
// operator role; viewers are forbidden. Params:
// { taskId, status required valid, resultRef? }. The store scopes the update
// to the caller's tenant via ctx; the fresh row is re-fetched afterwards so
// the response reflects server-side timestamps.
func (m *TasksMethods) handleUpdateStatus(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tasks.updateStatus requires operator role")))
		return
	}
	if m.tasks == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "task graph store not wired"))
		return
	}
	var params struct {
		TaskID    string  `json:"taskId"`
		Status    string  `json:"status"`
		ResultRef *string `json:"resultRef"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.TaskID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "taskId")))
		return
	}
	if !validTaskStatuses[params.Status] {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "status must be pending|running|blocked|done|failed|cancelled")))
		return
	}
	if err := m.tasks.UpdateTaskStatus(ctx, params.TaskID, params.Status, params.ResultRef); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "task", params.TaskID)))
			return
		}
		slog.Warn("tasks.update_status_failed", "task_id", params.TaskID, "status", params.Status, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "update task status")))
		return
	}
	task, ok := m.fetchTask(ctx, client, req, params.TaskID)
	if !ok {
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"task": toTaskJSON(task)}))
}

// fetchTask loads one task through the tenant-scoped store. Any miss or error
// answers not-found (matching fetchOwnedWorkspace's convention of not
// distinguishing errors from absence); ok=false means the caller must stop.
func (m *TasksMethods) fetchTask(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, id string) (*store.TaskNode, bool) {
	locale := store.LocaleFromContext(ctx)
	task, err := m.tasks.GetTask(ctx, id)
	if err != nil || task == nil {
		slog.Debug("tasks.get_failed", "task_id", id, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "task", id)))
		return nil, false
	}

	return task, true
}

// toTaskJSON renders a task node with camelCase wire keys.
func toTaskJSON(t *store.TaskNode) taskJSON {
	dependsOn := t.DependsOn
	if dependsOn == nil {
		dependsOn = []string{}
	}
	return taskJSON{
		ID:           t.ID,
		TenantID:     t.TenantID,
		WorkspaceID:  t.WorkspaceID,
		ParentID:     t.ParentID,
		OwnerAgentID: t.OwnerAgentID,
		SessionKey:   t.SessionKey,
		Title:        t.Title,
		Status:       t.Status,
		Priority:     t.Priority,
		DependsOn:    dependsOn,
		ResultRef:    t.ResultRef,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}
