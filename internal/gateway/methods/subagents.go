package methods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/sessions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// SubagentCancelFunc attempts to cancel the live runtime handle of a
// non-terminal subagent task. Returns true when a live run was cancelled.
// False means no in-memory handle exists (e.g. the gateway restarted since the
// task was spawned) — the caller then marks the row cancelled as an orphan.
type SubagentCancelFunc func(ctx context.Context, task *store.SubagentTaskData) bool

// SubagentMethods implements the subagents.* WS surface over the durable
// subagent task store (platform expansion Phase 5): list/get/archive/cancel of
// persisted subagent tasks. The in-memory SubagentManager stays the source of
// truth for live runs; cancel is delegated to it through an injected
// SubagentCancelFunc so this package keeps no dependency on the manager.
type SubagentMethods struct {
	tasks      store.SubagentTaskStore
	agentStore store.AgentStore
	cfg        *config.Config
	cancelFn   SubagentCancelFunc
}

// NewSubagentMethods creates the subagents.* handlers. cfg may be nil in tests
// (owner-ID visibility checks then degrade to role checks only).
func NewSubagentMethods(cfg *config.Config, tasks store.SubagentTaskStore, agentStore store.AgentStore) *SubagentMethods {
	return &SubagentMethods{cfg: cfg, tasks: tasks, agentStore: agentStore}
}

// SetCancelFn wires live-run cancellation (SubagentManager.CancelTask by
// runtime task ID). Nil-safe: without it, cancelling a non-terminal task falls
// back to the orphan path (durable mark-cancelled).
func (m *SubagentMethods) SetCancelFn(fn SubagentCancelFunc) { m.cancelFn = fn }

func (m *SubagentMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodSubagentsList, m.handleList)
	router.Register(protocol.MethodSubagentsGet, m.handleGet)
	router.Register(protocol.MethodSubagentsArchive, m.requireOperator(m.handleArchive))
	router.Register(protocol.MethodSubagentsArchiveCompleted, m.requireOperator(m.handleArchiveCompleted))
	router.Register(protocol.MethodSubagentsCancel, m.requireOperator(m.handleCancel))
}

// requireOperator rejects unauthenticated callers and viewers from the
// mutating subagents.* methods (same minimum role as jobs.cancel).
func (m *SubagentMethods) requireOperator(next gateway.MethodHandler) gateway.MethodHandler {
	return func(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
		locale := store.LocaleFromContext(ctx)
		if client.Role() == "" {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgUnauthorized)))
			return
		}
		if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized,
				i18n.T(locale, i18n.MsgPermissionDenied, req.Method+" requires operator role")))
			return
		}
		next(ctx, client, req)
	}
}

// subagentTaskJSON is the camelCase wire form of store.SubagentTaskData rows
// returned by subagents.list.
type subagentTaskJSON struct {
	TaskID      string     `json:"taskId"`
	Label       string     `json:"label"`
	Status      string     `json:"status"`
	Model       *string    `json:"model,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	ArchivedAt  *time.Time `json:"archivedAt,omitempty"`
	Summary     *string    `json:"summary,omitempty"`
	Error       *string    `json:"error,omitempty"`
}

// subagentTaskDetailJSON is the single-task wire form returned by
// subagents.get — superset of the list row.
type subagentTaskDetailJSON struct {
	subagentTaskJSON
	SessionKey  *string `json:"sessionKey,omitempty"`
	Description string  `json:"description,omitempty"`
	Provider    *string `json:"provider,omitempty"`
	Depth       int     `json:"depth"`
	Iterations  int     `json:"iterations"`
	InputTokens int64   `json:"inputTokens"`
	OutputToken int64   `json:"outputTokens"`
}

const subagentSummaryMaxRunes = 500

func toSubagentTaskJSON(t *store.SubagentTaskData) subagentTaskJSON {
	row := subagentTaskJSON{
		TaskID:      t.ID.String(),
		Label:       t.Subject,
		Status:      t.Status,
		Model:       t.Model,
		CreatedAt:   t.CreatedAt,
		CompletedAt: t.CompletedAt,
		ArchivedAt:  t.ArchivedAt,
	}
	if t.Result != nil && store.IsTerminalSubagentTaskStatus(t.Status) {
		summary := truncateRunes(*t.Result, subagentSummaryMaxRunes)
		row.Summary = &summary
		if t.Status == "failed" {
			row.Error = &summary
		}
	}
	return row
}

func toSubagentTaskDetailJSON(t *store.SubagentTaskData) subagentTaskDetailJSON {
	return subagentTaskDetailJSON{
		subagentTaskJSON: toSubagentTaskJSON(t),
		SessionKey:       t.SessionKey,
		Description:      t.Description,
		Provider:         t.Provider,
		Depth:            t.Depth,
		Iterations:       t.Iterations,
		InputTokens:      t.InputTokens,
		OutputToken:      t.OutputTokens,
	}
}

// truncateRunes caps a string at max runes (UTF-8 safe), appending "..." when
// truncation happened (the marker fits inside the cap).
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max > 3 {
		return string(r[:max-3]) + "..."
	}
	return string(r[:max])
}

// --- subagents.list ---

func (m *SubagentMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgUnauthorized)))
		return
	}
	if m.tasks == nil || m.agentStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "subagent task store not wired"))
		return
	}
	var params struct {
		AgentID         string `json:"agentId"`
		SessionKey      string `json:"sessionKey"`
		Status          string `json:"status"`
		IncludeArchived bool   `json:"includeArchived"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	agent := m.resolveAgent(ctx, client, req, params.AgentID, params.SessionKey)
	if agent == nil {
		return
	}
	if !m.agentAccessible(ctx, client, agent) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized,
			i18n.T(locale, i18n.MsgPermissionDenied, "subagent tasks")))
		return
	}

	tasks, err := m.tasks.ListByParent(ctx, agent.ID, params.Status, params.IncludeArchived)
	if err != nil {
		slog.Warn("subagents.list_failed", "agent_id", agent.ID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "subagent tasks")))
		return
	}
	rows := make([]subagentTaskJSON, 0, len(tasks))
	for i := range tasks {
		rows = append(rows, toSubagentTaskJSON(&tasks[i]))
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"tasks": rows,
		"count": len(rows),
	}))
}

// --- subagents.get ---

func (m *SubagentMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgUnauthorized)))
		return
	}
	if m.tasks == nil || m.agentStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "subagent task store not wired"))
		return
	}
	var params struct {
		TaskID string `json:"taskId"`
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
	taskID, err := uuid.Parse(params.TaskID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "task")))
		return
	}

	task, ok := m.fetchOwnedTask(ctx, client, req, taskID)
	if !ok {
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"task": toSubagentTaskDetailJSON(task),
	}))
}

// --- subagents.archive ---

func (m *SubagentMethods) handleArchive(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if m.tasks == nil || m.agentStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "subagent task store not wired"))
		return
	}
	var params struct {
		TaskID string `json:"taskId"`
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
	taskID, err := uuid.Parse(params.TaskID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "task")))
		return
	}

	task, ok := m.fetchOwnedTask(ctx, client, req, taskID)
	if !ok {
		return
	}
	if !store.IsTerminalSubagentTaskStatus(task.Status) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
			i18n.T(locale, i18n.MsgInvalidRequest,
				fmt.Sprintf("subagent task is still %s — only completed, failed, or cancelled tasks can be archived", task.Status))))
		return
	}
	if err := m.tasks.ArchiveByID(ctx, taskID); err != nil {
		switch {
		case errors.Is(err, store.ErrSubagentTaskNotFound):
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "subagent task", params.TaskID)))
		case errors.Is(err, store.ErrSubagentTaskNotTerminal):
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
				i18n.T(locale, i18n.MsgInvalidRequest,
					fmt.Sprintf("subagent task is still %s — only completed, failed, or cancelled tasks can be archived", task.Status))))
		default:
			slog.Warn("subagents.archive_failed", "task_id", taskID, "error", err)
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "archive subagent task")))
		}
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"taskId":   taskID.String(),
		"archived": true,
	}))
}

// --- subagents.archive_completed ---

func (m *SubagentMethods) handleArchiveCompleted(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if m.tasks == nil || m.agentStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "subagent task store not wired"))
		return
	}
	var params struct {
		AgentID    string `json:"agentId"`
		SessionKey string `json:"sessionKey"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	agent := m.resolveAgent(ctx, client, req, params.AgentID, params.SessionKey)
	if agent == nil {
		return
	}
	if !m.agentAccessible(ctx, client, agent) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized,
			i18n.T(locale, i18n.MsgPermissionDenied, "subagent tasks")))
		return
	}

	archived, err := m.tasks.ArchiveCompletedForParent(ctx, agent.ID)
	if err != nil {
		if errors.Is(err, store.ErrSubagentRootAgentIDRequired) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "agentId")))
			return
		}
		slog.Warn("subagents.archive_completed_failed", "agent_id", agent.ID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "archive subagent tasks")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"agentId":  agent.ID.String(),
		"archived": archived,
	}))
}

// --- subagents.cancel ---

func (m *SubagentMethods) handleCancel(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if m.tasks == nil || m.agentStore == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "subagent task store not wired"))
		return
	}
	var params struct {
		TaskID string `json:"taskId"`
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
	taskID, err := uuid.Parse(params.TaskID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "task")))
		return
	}

	task, ok := m.fetchOwnedTask(ctx, client, req, taskID)
	if !ok {
		return
	}
	if store.IsTerminalSubagentTaskStatus(task.Status) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
			i18n.T(locale, i18n.MsgInvalidRequest,
				fmt.Sprintf("subagent task already finished with status %s — cannot cancel", task.Status))))
		return
	}

	// Live path: the manager still holds a runtime handle for this task.
	if m.cancelFn != nil && m.cancelFn(ctx, task) {
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
			"taskId":    taskID.String(),
			"cancelled": true,
		}))
		return
	}

	// Orphan path: no live handle (gateway restarted since spawn, or the cancel
	// fn is not wired). The durable row is marked cancelled so it stops
	// counting as active; recovery semantics mirror RecoverInterrupted.
	orphanResult := "cancelled by user (no live run handle)"
	slog.Warn("security.subagent_orphan_cancel",
		"task_id", task.ID, "root_agent_id", task.RootAgentID, "status", task.Status)
	if err := m.tasks.UpdateStatus(ctx, task.RootAgentID, task.ID, "cancelled", &orphanResult,
		task.Iterations, task.InputTokens, task.OutputTokens); err != nil {
		if errors.Is(err, store.ErrSubagentTaskNotFound) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "subagent task", params.TaskID)))
			return
		}
		slog.Warn("subagents.cancel_failed", "task_id", taskID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "cancel subagent task")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"taskId":    taskID.String(),
		"cancelled": true,
		"orphan":    true,
	}))
}

// --- helpers ---

// resolveAgent resolves the root agent for a request: explicit agentId (UUID
// or agent_key) wins, otherwise the agentKey is parsed out of sessionKey
// (chat.go's convention). Sends the error response and returns nil on failure.
func (m *SubagentMethods) resolveAgent(
	ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, agentID, sessionKey string,
) *store.AgentData {
	locale := store.LocaleFromContext(ctx)
	if agentID == "" && sessionKey != "" {
		if key, _ := sessions.ParseSessionKey(sessionKey); key != "" {
			agentID = key
		}
	}
	if agentID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "agentId")))
		return nil
	}
	agent, err := resolveAgentInfo(ctx, m.agentStore, agentID)
	if err != nil || agent == nil {
		slog.Debug("subagents agent resolve failed", "agent", agentID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "agent", agentID)))
		return nil
	}
	return agent
}

// fetchOwnedTask loads a task by ID (tenant-scoped) and enforces that the
// calling user can access the task's root agent. Sends the error response and
// returns ok=false when the caller must stop.
func (m *SubagentMethods) fetchOwnedTask(
	ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, taskID uuid.UUID,
) (*store.SubagentTaskData, bool) {
	locale := store.LocaleFromContext(ctx)
	task, err := m.tasks.GetByID(ctx, taskID)
	if err != nil {
		slog.Warn("subagents.get_task_failed", "task_id", taskID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "load subagent task")))
		return nil, false
	}
	if task == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "subagent task", taskID.String())))
		return nil, false
	}
	agent, err := m.agentStore.GetByID(ctx, task.RootAgentID)
	if err != nil || agent == nil {
		// Root agent deleted: the task is an unowned orphan — treat as absent.
		slog.Debug("subagents task root agent missing", "task_id", taskID, "root_agent_id", task.RootAgentID)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "subagent task", taskID.String())))
		return nil, false
	}
	if !m.agentAccessible(ctx, client, agent) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized,
			i18n.T(locale, i18n.MsgPermissionDenied, "subagent tasks")))
		return nil, false
	}
	return task, true
}

// agentAccessible reports whether the WS user may see the agent's subagent
// tasks. Mirrors agents.list visibility: admins and configured owners see
// everything; everyone else must reach the agent through ListAccessible
// (owner_id, default agent, explicit shares, or channel allow_from).
func (m *SubagentMethods) agentAccessible(ctx context.Context, client *gateway.Client, agent *store.AgentData) bool {
	if agent == nil {
		return false
	}
	var ownerIDs []string
	if m.cfg != nil {
		ownerIDs = m.cfg.Gateway.OwnerIDs
	}
	if canSeeAll(client.Role(), ownerIDs, client.UserID()) {
		return true
	}
	userID := client.UserID()
	if userID == "" {
		return false
	}
	if agent.OwnerID == userID || agent.IsDefault {
		return true
	}
	if m.agentStore == nil {
		return false
	}
	accessible, err := m.agentStore.ListAccessible(ctx, userID)
	if err != nil {
		slog.Warn("subagents access check failed", "agent_id", agent.ID, "error", err)
		return false
	}
	for _, a := range accessible {
		if a.ID == agent.ID {
			return true
		}
	}
	return false
}
