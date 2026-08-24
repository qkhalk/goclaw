package methods

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

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
	MethodJobsList   = "jobs.list"
	MethodJobsGet    = "jobs.get"
	MethodJobsCancel = "jobs.cancel"
)

// JobsMethods implements the jobs.* WS surface over the agent-job lifecycle
// store (plan §21): listing/getting/cancelling persisted execution jobs.
type JobsMethods struct {
	jobs store.AgentJobStore
}

func NewJobsMethods(jobs store.AgentJobStore) *JobsMethods {
	return &JobsMethods{jobs: jobs}
}

// Register wires the jobs.* methods into the method router.
func (m *JobsMethods) Register(router *gateway.MethodRouter) {
	router.Register(MethodJobsList, m.handleList)
	router.Register(MethodJobsGet, m.handleGet)
	router.Register(MethodJobsCancel, m.handleCancel)
}

// jobJSON is the camelCase wire form of store.AgentJob.
type jobJSON struct {
	ID          string     `json:"id"`
	TenantID    *string    `json:"tenantId"`
	WorkspaceID *string    `json:"workspaceId"`
	SessionKey  string     `json:"sessionKey"`
	AgentID     *string    `json:"agentId"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	Title       string     `json:"title"`
	ResultRef   *string    `json:"resultRef"`
	Error       *string    `json:"error"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func toJobJSON(job *store.AgentJob) jobJSON {
	return jobJSON{
		ID:          job.ID,
		TenantID:    job.TenantID,
		WorkspaceID: job.WorkspaceID,
		SessionKey:  job.SessionKey,
		AgentID:     job.AgentID,
		Kind:        job.Kind,
		Status:      job.Status,
		Priority:    job.Priority,
		Title:       job.Title,
		ResultRef:   job.ResultRef,
		Error:       job.Error,
		StartedAt:   job.StartedAt,
		CompletedAt: job.CompletedAt,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
	}
}

// handleList lists agent jobs (jobs.list), tenant-scoped via ctx.
// Params: { workspaceId?, sessionKey?, status?, activeOnly? }. Ordering
// (priority DESC, created_at DESC) and the default limit are owned by the
// store's JobListOpts handling.
func (m *JobsMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	if m.jobs == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "agent job store not wired"))
		return
	}
	var params struct {
		WorkspaceID string `json:"workspaceId"`
		SessionKey  string `json:"sessionKey"`
		Status      string `json:"status"`
		ActiveOnly  bool   `json:"activeOnly"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			locale := store.LocaleFromContext(ctx)
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	opts := store.JobListOpts{
		WorkspaceID: params.WorkspaceID,
		SessionKey:  params.SessionKey,
		Status:      params.Status,
		ActiveOnly:  params.ActiveOnly,
		Limit:       0, // store default (100)
	}
	items, err := m.jobs.ListJobs(ctx, opts)
	if err != nil {
		slog.Warn("jobs.list_failed", "error", err)
		locale := store.LocaleFromContext(ctx)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "list jobs")))
		return
	}
	out := make([]jobJSON, 0, len(items))
	for _, j := range items {
		out = append(out, toJobJSON(j))
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"jobs": out}))
}

// handleGet reads one job (jobs.get). Params: { jobId }. The fetch is scoped
// to the caller's tenant by the store; a miss or error answers not-found
// (matching fetchOwnedWorkspace's convention of not distinguishing errors
// from absence).
func (m *JobsMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if m.jobs == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "agent job store not wired"))
		return
	}
	var params struct {
		JobID string `json:"jobId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.JobID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "jobId")))
		return
	}
	job, ok := m.fetchJob(ctx, client, req, params.JobID)
	if !ok {
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"job": toJobJSON(job)}))
}

// handleCancel cancels one job (jobs.cancel). Requires operator role; viewers
// are forbidden. Params: { jobId }. Terminal rows are left untouched
// idempotently; the response is identical either way. A not-found answer
// matches the workspace MsgNotFound convention.
func (m *JobsMethods) handleCancel(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "jobs.cancel requires operator role")))
		return
	}
	if m.jobs == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "agent job store not wired"))
		return
	}
	var params struct {
		JobID string `json:"jobId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.JobID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "jobId")))
		return
	}
	err := m.jobs.CancelJob(ctx, params.JobID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("jobs.cancel_failed", "job_id", params.JobID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "cancel job")))
		return
	}
	if err != nil {
		slog.Warn("jobs.cancel_failed", "job_id", params.JobID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "cancel job")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"cancelled": true}))
}

// fetchJob loads one job through the tenant-scoped store; a miss or error
// answers not-found; ok=false means the caller must stop.
func (m *JobsMethods) fetchJob(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, id string) (*store.AgentJob, bool) {
	locale := store.LocaleFromContext(ctx)
	job, err := m.jobs.GetJob(ctx, id)
	if err != nil || job == nil {
		slog.Debug("jobs.get_failed", "job_id", id, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "job", id)))
		return nil, false
	}
	return job, true
}
