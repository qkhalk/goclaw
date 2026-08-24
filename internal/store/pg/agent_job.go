package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGAgentJobStore implements store.AgentJobStore backed by PostgreSQL.
type PGAgentJobStore struct {
	db *sql.DB
}

func NewPGAgentJobStore(db *sql.DB) *PGAgentJobStore {
	return &PGAgentJobStore{db: db}
}

const agentJobColumns = `id, tenant_id, workspace_id, session_key, agent_id,
 kind, status, priority, title, result_ref, error, started_at, completed_at,
 created_at, updated_at`

func scanAgentJob(row interface{ Scan(...any) error }) (*store.AgentJob, error) {
	var j store.AgentJob
	var workspaceID, agentID, resultRef, errMsg sql.NullString
	var startedAt, completedAt sql.NullTime
	if err := row.Scan(&j.ID, &j.TenantID, &workspaceID, &j.SessionKey,
		&agentID, &j.Kind, &j.Status, &j.Priority, &j.Title,
		&resultRef, &errMsg, &startedAt, &completedAt,
		&j.CreatedAt, &j.UpdatedAt); err != nil {
		return nil, err
	}
	j.WorkspaceID = nilStrPtr(workspaceID)
	j.AgentID = nilStrPtr(agentID)
	j.ResultRef = nilStrPtr(resultRef)
	j.Error = nilStrPtr(errMsg)
	if startedAt.Valid {
		t := startedAt.Time
		j.StartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		j.CompletedAt = &t
	}
	return &j, nil
}

// CreateJob inserts a new job. ID defaults to a fresh UUIDv7; timestamps,
// kind, priority, title and status are defaulted when zero-valued. tenant_id
// is taken from the AgentJob row itself (nil = master/global).
func (s *PGAgentJobStore) CreateJob(ctx context.Context, job *store.AgentJob) error {
	if job.ID == "" {
		job.ID = store.GenNewID().String()
	}
	now := time.Now()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = now
	}
	if !store.ValidAgentJobKind(job.Kind) {
		job.Kind = store.AgentJobKindRun
	}
	if !store.ValidAgentJobStatus(job.Status) {
		job.Status = store.AgentJobStatusQueued
	}
	var tenantID, workspaceID, agentID any
	if job.TenantID != nil && *job.TenantID != "" {
		tid, err := uuid.Parse(*job.TenantID)
		if err != nil {
			return fmt.Errorf("agent job tenant_id: %w", err)
		}
		tenantID = tid
	}
	if job.WorkspaceID != nil && *job.WorkspaceID != "" {
		wid, err := uuid.Parse(*job.WorkspaceID)
		if err != nil {
			return fmt.Errorf("agent job workspace_id: %w", err)
		}
		workspaceID = wid
	}
	if job.AgentID != nil && *job.AgentID != "" {
		agentID = *job.AgentID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_jobs
		 (id, tenant_id, workspace_id, session_key, agent_id, kind, status,
		  priority, title, result_ref, error, started_at, completed_at,
		  created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		job.ID, tenantID, workspaceID, job.SessionKey, agentID,
		job.Kind, job.Status, job.Priority, job.Title,
		nilStr(derefStr(job.ResultRef)), nilStr(derefStr(job.Error)),
		nilTime(job.StartedAt), nilTime(job.CompletedAt),
	)
	return err
}

// GetJob returns one job by id, scoped to the context tenant (master scope
// sees only nil-tenant rows). sql.ErrNoRows when not found / not visible.
func (s *PGAgentJobStore) GetJob(ctx context.Context, id string) (*store.AgentJob, error) {
	jid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("agent job id %q: %w", id, err)
	}
	where := "id = $1"
	args := []any{jid}
	where, args = pgTenantFilter(ctx, where, args)
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentJobColumns+` FROM agent_jobs WHERE `+where, args...)
	return scanAgentJob(row)
}

// ListJobs returns jobs ordered priority DESC then created_at DESC, filtered
// by the non-empty JobListOpts fields and scoped like GetJob. Terminal rows
// are excluded when opts.ActiveOnly is set; LIMIT defaults to 100.
func (s *PGAgentJobStore) ListJobs(ctx context.Context, opts store.JobListOpts) ([]*store.AgentJob, error) {
	where := "1=1"
	var args []any
	where, args = pgTenantFilter(ctx, where, args)
	if opts.WorkspaceID != "" {
		wid, err := uuid.Parse(opts.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("agent job workspace_id %q: %w", opts.WorkspaceID, err)
		}
		args = append(args, wid)
		where += fmt.Sprintf(" AND workspace_id = $%d", len(args))
	}
	if opts.SessionKey != "" {
		args = append(args, opts.SessionKey)
		where += fmt.Sprintf(" AND session_key = $%d", len(args))
	}
	if opts.Status != "" {
		args = append(args, opts.Status)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if opts.ActiveOnly {
		args = append(args, pqStringArray(store.AgentJobTerminalStatuses))
		where += fmt.Sprintf(" AND status <> ALL($%d::text[])", len(args))
	}
	limit := 100
	if opts.Limit > 0 {
		limit = opts.Limit
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentJobColumns+` FROM agent_jobs WHERE `+where+
			fmt.Sprintf(` ORDER BY priority DESC, created_at DESC LIMIT %d`, limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.AgentJob{}
	for rows.Next() {
		j, err := scanAgentJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// UpdateJobStatus transitions one job's status. result_ref keeps its existing
// value when resultRef is nil; error is overwritten with errMsg (NULL clears
// it). started_at is stamped only on the first transition out of 'queued'
// (CASE guards on the current status); completed_at is stamped only when the
// target status is terminal. updated_at always refreshes. Returns
// sql.ErrNoRows when nothing matched the caller's scope.
func (s *PGAgentJobStore) UpdateJobStatus(ctx context.Context, id string, status string, resultRef, errMsg *string) error {
	if !store.ValidAgentJobStatus(status) {
		return fmt.Errorf("update agent job status: unknown status %q", status)
	}
	jid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("agent job id %q: %w", id, err)
	}
	where := "id = $1 AND status <> $2"
	args := []any{jid, status} // no-op transition guard (also keeps RowsAffected meaningful)
	where, args = pgTenantFilter(ctx, where, args)
	args = append(args,
		status,                      // new status
		nilStr(derefStr(resultRef)), // keep existing result_ref when NULL
		nilStr(derefStr(errMsg)),    // overwrite error (NULL clears it)
		store.AgentJobStatusQueued,  // started_at stamp guard
		pqStringArray(store.AgentJobTerminalStatuses), // completed_at stamp guard
	)
	n := len(args)
	q := `UPDATE agent_jobs SET
	      status = $` + strconv.Itoa(n-4) + `,
	      result_ref = COALESCE($` + strconv.Itoa(n-3) + `, result_ref),
	      error = $` + strconv.Itoa(n-2) + `,
	      started_at = CASE WHEN status = $` + strconv.Itoa(n-1) + `
	                        AND status <> $` + strconv.Itoa(n-4) + ` THEN now() ELSE started_at END,
	      completed_at = CASE WHEN $` + strconv.Itoa(n-4) + `::text = ANY($` + strconv.Itoa(n) + `)
	                          THEN now() ELSE completed_at END,
	      updated_at = now()
	      WHERE ` + where
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CancelJob marks a job cancelled unless it is already terminal; zero
// affected rows are success (idempotent cancel).
func (s *PGAgentJobStore) CancelJob(ctx context.Context, id string) error {
	jid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("agent job id %q: %w", id, err)
	}
	where := "id = $1 AND status NOT IN ('completed','failed','cancelled')"
	args := []any{jid}
	where, args = pgTenantFilter(ctx, where, args)
	res, err := s.db.ExecContext(ctx,
		`UPDATE agent_jobs SET status = 'cancelled',
		 completed_at = COALESCE(completed_at, now()), updated_at = now()
		 WHERE `+where, args...)
	if err != nil {
		return err
	}
	_ = res // RowsAffected == 0 means already terminal or absent: still success.
	return nil
}

// pgTenantFilter appends the context-tenant condition to a WHERE clause being
// built with $N placeholders. Master scope (no tenant in ctx) reaches only
// rows whose tenant_id is NULL; with a tenant present the filter is exact.
func pgTenantFilter(ctx context.Context, where string, args []any) (string, []any) {
	if !store.IsCrossTenant(ctx) {
		tenantID := store.TenantIDFromContext(ctx)
		if tenantID == uuid.Nil {
			where += " AND tenant_id IS NULL"
			return where, args
		}
		args = append(args, tenantID)
		where += fmt.Sprintf(" AND tenant_id = $%d", len(args))
	}
	return where, args
}
