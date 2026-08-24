package store

import (
	"context"
	"time"
)

// Agent-job lifecycle status values (Paseo plan Phase 2, plan §21). A job
// moves forward queued -> starting -> running, optionally parking in
// waiting_input / waiting_approval / paused, and ends in exactly one
// terminal status: completed | failed | cancelled.
const (
	AgentJobStatusQueued          = "queued"
	AgentJobStatusStarting        = "starting"
	AgentJobStatusRunning         = "running"
	AgentJobStatusWaitingInput    = "waiting_input"
	AgentJobStatusWaitingApproval = "waiting_approval"
	AgentJobStatusPaused          = "paused"
	AgentJobStatusCompleted       = "completed"
	AgentJobStatusFailed          = "failed"
	AgentJobStatusCancelled       = "cancelled"
)

// Agent-job kinds (plan §21): a run executes a unit of work in its own
// session, a delegation fans sub-work out to subagents, a consolidation
// merges delegated results back into the parent session.
const (
	AgentJobKindRun           = "run"
	AgentJobKindDelegation    = "delegation"
	AgentJobKindConsolidation = "consolidation"
)

// AgentJobTerminalStatuses are the lifecycle end states: jobs in one of them
// no longer accept status transitions, are excluded by JobListOpts.ActiveOnly
// listings, and are skipped by CancelJob.
var AgentJobTerminalStatuses = []string{
	AgentJobStatusCompleted,
	AgentJobStatusFailed,
	AgentJobStatusCancelled,
}

// ValidAgentJobKind reports whether k is a known agent-job kind.
func ValidAgentJobKind(k string) bool {
	switch k {
	case AgentJobKindRun, AgentJobKindDelegation, AgentJobKindConsolidation:
		return true
	}
	return false
}

// AgentJob is the persisted subset of the agent execution lifecycle, kept
// deliberately separate from sessions (plan §21): hot runtime state lives in
// memory / agent_runs, only the restart-surviving subset lands here.
//
// TenantID == nil means master/global scope — the same convention as
// workspaces, node_leases and api_keys. WorkspaceID optionally links the job
// to the sandboxed workspace it runs in.
type AgentJob struct {
	ID          string // canonical job id, uuid string
	TenantID    *string
	WorkspaceID *string // nullable FK -> workspaces(id)
	SessionKey  string  // conversation the job belongs to; '' for headless jobs
	AgentID     *string // nullable owning-agent identifier
	Kind        string  // "run" | "delegation" | "consolidation"
	Status      string  // queued|starting|running|waiting_input|waiting_approval|paused|completed|failed|cancelled
	Priority    int     // higher runs first; default 0
	Title       string
	ResultRef   *string
	Error       *string
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ValidAgentJobStatus reports whether s is a known agent-job status.
func ValidAgentJobStatus(s string) bool {
	switch s {
	case AgentJobStatusQueued,
		AgentJobStatusStarting,
		AgentJobStatusRunning,
		AgentJobStatusWaitingInput,
		AgentJobStatusWaitingApproval,
		AgentJobStatusPaused,
		AgentJobStatusCompleted,
		AgentJobStatusFailed,
		AgentJobStatusCancelled:
		return true
	}
	return false
}

// JobListOpts filters ListJobs. Empty fields are ignored; ActiveOnly excludes
// terminal rows (completed | failed | cancelled). Limit <= 0 defaults to 100.
type JobListOpts struct {
	WorkspaceID string
	SessionKey  string
	Status      string
	ActiveOnly  bool
	Limit       int
}

// AgentJobStore persists agent execution jobs. Implementations must scope
// reads and writes to the tenant from context where the row carries one;
// nil-TenantID rows are master/global and only reachable from master scope.
type AgentJobStore interface {
	// CreateJob inserts a new job. The ID, timestamps and status are
	// defaulted by the implementation when zero-valued; tenant_id is taken
	// from the AgentJob row itself (nil = master/global).
	CreateJob(ctx context.Context, job *AgentJob) error
	// GetJob resolves a job by canonical id. Scoped to the context tenant
	// when one is present; master scope (no tenant) reaches only nil-tenant
	// rows. Returns sql.ErrNoRows when the job does not exist or is not
	// visible from the caller's scope.
	GetJob(ctx context.Context, id string) (*AgentJob, error)
	// ListJobs returns jobs ordered by priority DESC, created_at DESC,
	// filtered by the non-empty JobListOpts fields and scoped like GetJob.
	ListJobs(ctx context.Context, opts JobListOpts) ([]*AgentJob, error)
	// UpdateJobStatus transitions a job's status. result_ref keeps its
	// existing value when resultRef is nil; error is overwritten with errMsg
	// (NULL clears it). started_at is stamped on the first transition out of
	// 'queued'; completed_at is stamped when the target status is terminal
	// (completed | failed | cancelled). updated_at always refreshes. Returns
	// sql.ErrNoRows when nothing matched the caller's scope.
	UpdateJobStatus(ctx context.Context, id string, status string, resultRef, errMsg *string) error
	// CancelJob marks a job cancelled unless it is already terminal
	// (completed | failed | cancelled). Zero affected rows are treated as
	// success (idempotent cancel).
	CancelJob(ctx context.Context, id string) error
}
