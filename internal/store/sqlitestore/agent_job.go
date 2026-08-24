//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteAgentJobStore implements store.AgentJobStore backed by SQLite
// (desktop/lite edition).
type SQLiteAgentJobStore struct {
	db *sql.DB
}

func NewSQLiteAgentJobStore(db *sql.DB) *SQLiteAgentJobStore {
	return &SQLiteAgentJobStore{db: db}
}

const agentJobColumns = `id, tenant_id, workspace_id, session_key, agent_id,
 kind, status, priority, title, result_ref, error, started_at, completed_at,
 created_at, updated_at`

func scanAgentJob(row interface{ Scan(...any) error }) (*store.AgentJob, error) {
	var j store.AgentJob
	var workspaceID, agentID, resultRef, errMsg sql.NullString
	// Timestamps are TEXT in SQLite (modernc driver returns strings); scan
	// through sqliteTime/nullSqliteTime so RFC3339 text round-trips into
	// time.Time.
	var startedAt, completedAt nullSqliteTime
	var createdAt, updatedAt sqliteTime
	if err := row.Scan(&j.ID, &j.TenantID, &workspaceID, &j.SessionKey,
		&agentID, &j.Kind, &j.Status, &j.Priority, &j.Title,
		&resultRef, &errMsg, &startedAt, &completedAt,
		&createdAt, &updatedAt); err != nil {
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
	j.CreatedAt = createdAt.Time
	j.UpdatedAt = updatedAt.Time
	return &j, nil
}

// CreateJob inserts a new job. ID defaults to a fresh UUIDv7; timestamps,
// kind, priority, title and status are defaulted when zero-valued. tenant_id
// is taken from the AgentJob row itself (nil = master/global).
func (s *SQLiteAgentJobStore) CreateJob(ctx context.Context, job *store.AgentJob) error {
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
	var tenantArg, workspaceArg, agentArg any
	if job.TenantID != nil && *job.TenantID != "" {
		tenantArg = *job.TenantID
	}
	if job.WorkspaceID != nil && *job.WorkspaceID != "" {
		workspaceArg = *job.WorkspaceID
	}
	if job.AgentID != nil && *job.AgentID != "" {
		agentArg = *job.AgentID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_jobs
		 (id, tenant_id, workspace_id, session_key, agent_id, kind, status,
		  priority, title, result_ref, error, started_at, completed_at,
		  created_at, updated_at)
		 VALUES (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15)`,
		job.ID, tenantArg, workspaceArg, job.SessionKey, agentArg,
		job.Kind, job.Status, job.Priority, job.Title,
		nilStr(derefStr(job.ResultRef)), nilStr(derefStr(job.Error)),
		sqliteTimePtrArg(job.StartedAt), sqliteTimePtrArg(job.CompletedAt),
		job.CreatedAt.UTC(), job.UpdatedAt.UTC(),
	)
	return err
}

// GetJob returns one job by id, scoped to the context tenant (master scope
// sees only nil-tenant rows). sql.ErrNoRows when not found / not visible.
func (s *SQLiteAgentJobStore) GetJob(ctx context.Context, id string) (*store.AgentJob, error) {
	where := "id = ?1"
	args := []any{id}
	where, args = agentJobTenantFilter(ctx, where, args)
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentJobColumns+` FROM agent_jobs WHERE `+where, args...)
	return scanAgentJob(row)
}

// ListJobs returns jobs ordered priority DESC then created_at DESC, filtered
// by the non-empty JobListOpts fields and scoped like GetJob. Terminal rows
// are excluded when opts.ActiveOnly is set; LIMIT defaults to 100.
func (s *SQLiteAgentJobStore) ListJobs(ctx context.Context, opts store.JobListOpts) ([]*store.AgentJob, error) {
	where := "1=1"
	var args []any
	where, args = agentJobTenantFilter(ctx, where, args)
	if opts.WorkspaceID != "" {
		args = append(args, opts.WorkspaceID)
		where += fmt.Sprintf(" AND workspace_id = ?%d", len(args))
	}
	if opts.SessionKey != "" {
		args = append(args, opts.SessionKey)
		where += fmt.Sprintf(" AND session_key = ?%d", len(args))
	}
	if opts.Status != "" {
		args = append(args, opts.Status)
		where += fmt.Sprintf(" AND status = ?%d", len(args))
	}
	if opts.ActiveOnly {
		args = append(args, strings.Join(store.AgentJobTerminalStatuses, ","))
		where += fmt.Sprintf(" AND status NOT IN (?%d)", len(args))
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
func (s *SQLiteAgentJobStore) UpdateJobStatus(ctx context.Context, id string, status string, resultRef, errMsg *string) error {
	if !store.ValidAgentJobStatus(status) {
		return fmt.Errorf("update agent job status: unknown status %q", status)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx,
		`UPDATE agent_jobs SET
		 status = ?2,
		 result_ref = COALESCE(?3, result_ref),
		 error = ?4,
		 started_at = CASE WHEN status = 'queued' AND ?2 <> 'queued' THEN ?5 ELSE started_at END,
		 completed_at = CASE WHEN ?2 IN ('completed','failed','cancelled') THEN ?5 ELSE completed_at END,
		 updated_at = ?5
		 WHERE id = ?1 AND status <> ?2`,
		id, status, nilStr(derefStr(resultRef)), nilStr(derefStr(errMsg)), now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CancelJob marks a job cancelled unless it is already terminal; zero
// affected rows are success (idempotent cancel).
func (s *SQLiteAgentJobStore) CancelJob(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_jobs SET status = 'cancelled',
		 completed_at = COALESCE(completed_at, ?2), updated_at = ?2
		 WHERE id = ?1 AND status NOT IN ('completed','failed','cancelled')`,
		id, now)
	// RowsAffected == 0 means already terminal or absent: still success.
	return err
}

// agentJobTenantFilter appends the context-tenant condition to a WHERE clause
// being built with ?N placeholders. Master scope (no tenant in ctx) reaches
// only rows whose tenant_id is NULL; with a tenant present the filter is
// exact.
func agentJobTenantFilter(ctx context.Context, where string, args []any) (string, []any) {
	if !store.IsCrossTenant(ctx) {
		tenantID := store.TenantIDFromContext(ctx)
		if tenantID == uuid.Nil {
			where += " AND tenant_id IS NULL"
			return where, args
		}
		args = append(args, tenantID.String())
		where += fmt.Sprintf(" AND tenant_id = ?%d", len(args))
	}
	return where, args
}

// sqliteTimePtrArg maps a *time.Time to an SQLite TEXT timestamp argument
// (nil stays SQL NULL).
func sqliteTimePtrArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}
