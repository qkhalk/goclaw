package pg

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGVideoJobStore implements store.VideoRenderJobStore backed by Postgres.
type PGVideoJobStore struct {
	db *sql.DB
}

// NewPGVideoJobStore creates a PGVideoJobStore.
func NewPGVideoJobStore(db *sql.DB) *PGVideoJobStore {
	return &PGVideoJobStore{db: db}
}

const videoJobColumns = `id, tenant_id, user_id, agent_id, session_key,
	status, engine, storyboard_json, output_path, output_size_bytes, error,
	created_at, updated_at, started_at, finished_at, expires_at`

// videoJobColumnsAliased qualifies columns with the j. alias for use in
// CTE RETURNING clauses where the bare column name is ambiguous.
const videoJobColumnsAliased = `j.id, j.tenant_id, j.user_id, j.agent_id, j.session_key,
	j.status, j.engine, j.storyboard_json, j.output_path, j.output_size_bytes, j.error,
	j.created_at, j.updated_at, j.started_at, j.finished_at, j.expires_at`

func tenantIDArg(ctx context.Context) uuid.UUID {
	return store.TenantIDFromContext(ctx)
}

// hasTenant returns true when the context carries a real (non-Nil) tenant ID.
// The dispatcher uses context.Background() which has uuid.Nil, meaning "all tenants".
func hasTenant(ctx context.Context) bool {
	return store.TenantIDFromContext(ctx) != uuid.Nil
}

func scanVideoJob(rs interface{ Scan(dest ...any) error }) (*store.VideoRenderJob, error) {
	var j store.VideoRenderJob
	if err := rs.Scan(
		&j.ID, &j.TenantID, &j.UserID, &j.AgentID, &j.SessionKey,
		&j.Status, &j.Engine, &j.StoryboardJSON, &j.OutputPath,
		&j.OutputSizeBytes, &j.Error,
		&j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.FinishedAt, &j.ExpiresAt,
	); err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *PGVideoJobStore) Create(ctx context.Context, job *store.VideoRenderJob) error {
	tenantID := tenantIDArg(ctx)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO video_render_jobs (id, tenant_id, user_id, agent_id, session_key,
			status, engine, storyboard_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		job.ID, tenantID, job.UserID, job.AgentID, job.SessionKey,
		"queued", job.Engine, job.StoryboardJSON)
	return err
}

func (s *PGVideoJobStore) Get(ctx context.Context, id string) (*store.VideoRenderJob, error) {
	tenantID := tenantIDArg(ctx)
	var row *sql.Row
	if hasTenant(ctx) {
		row = s.db.QueryRowContext(ctx,
			"SELECT "+videoJobColumns+" FROM video_render_jobs WHERE id=$1 AND tenant_id=$2",
			id, tenantID)
	} else {
		row = s.db.QueryRowContext(ctx,
			"SELECT "+videoJobColumns+" FROM video_render_jobs WHERE id=$1",
			id)
	}
	j, err := scanVideoJob(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrVideoJobNotFound
	}
	return j, err
}

func (s *PGVideoJobStore) UpdateStatus(ctx context.Context, id string, upd store.VideoJobUpdate) error {
	tenantID := tenantIDArg(ctx)
	var res sql.Result
	var err error
	if hasTenant(ctx) {
		res, err = s.db.ExecContext(ctx, `
			UPDATE video_render_jobs SET
				status = $2,
				error = COALESCE($3, error),
				output_path = COALESCE($4, output_path),
				output_size_bytes = COALESCE($5, output_size_bytes),
				started_at = CASE WHEN $2 = 'rendering' AND started_at IS NULL THEN now() ELSE started_at END,
				finished_at = CASE WHEN $2 IN ('done','failed','cancelled') AND finished_at IS NULL THEN now() ELSE finished_at END,
				updated_at = now()
			WHERE id = $1 AND tenant_id = $6`,
			id, upd.Status, upd.Error, upd.OutputPath, upd.OutputSizeBytes, tenantID)
	} else {
		res, err = s.db.ExecContext(ctx, `
			UPDATE video_render_jobs SET
				status = $2,
				error = COALESCE($3, error),
				output_path = COALESCE($4, output_path),
				output_size_bytes = COALESCE($5, output_size_bytes),
				started_at = CASE WHEN $2 = 'rendering' AND started_at IS NULL THEN now() ELSE started_at END,
				finished_at = CASE WHEN $2 IN ('done','failed','cancelled') AND finished_at IS NULL THEN now() ELSE finished_at END,
				updated_at = now()
			WHERE id = $1`,
			id, upd.Status, upd.Error, upd.OutputPath, upd.OutputSizeBytes)
	}
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrVideoJobNotFound
	}
	return nil
}

func (s *PGVideoJobStore) ListByTenant(ctx context.Context, limit int) ([]store.VideoRenderJob, error) {
	if limit <= 0 {
		limit = 50
	}
	tenantID := tenantIDArg(ctx)
	var rows *sql.Rows
	var err error
	if hasTenant(ctx) {
		rows, err = s.db.QueryContext(ctx,
			"SELECT "+videoJobColumns+" FROM video_render_jobs WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2",
			tenantID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			"SELECT "+videoJobColumns+" FROM video_render_jobs ORDER BY created_at DESC LIMIT $1",
			limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.VideoRenderJob
	for rows.Next() {
		j, scanErr := scanVideoJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

func (s *PGVideoJobStore) ClaimNextQueued(ctx context.Context) (*store.VideoRenderJob, error) {
	tenantID := tenantIDArg(ctx)
	var row *sql.Row
	if hasTenant(ctx) {
		row = s.db.QueryRowContext(ctx, `
			WITH next AS (
				SELECT id FROM video_render_jobs
				WHERE status = 'queued' AND tenant_id = $1
				ORDER BY created_at
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			UPDATE video_render_jobs j SET
				status = 'rendering',
				started_at = now(),
				updated_at = now()
			FROM next WHERE j.id = next.id
			RETURNING `+videoJobColumnsAliased, tenantID)
	} else {
		row = s.db.QueryRowContext(ctx, `
			WITH next AS (
				SELECT id FROM video_render_jobs
				WHERE status = 'queued'
				ORDER BY created_at
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			UPDATE video_render_jobs j SET
				status = 'rendering',
				started_at = now(),
				updated_at = now()
			FROM next WHERE j.id = next.id
			RETURNING `+videoJobColumnsAliased)
	}
	j, err := scanVideoJob(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNoQueuedJobs
	}
	return j, err
}

func (s *PGVideoJobStore) Delete(ctx context.Context, id string) error {
	tenantID := tenantIDArg(ctx)
	var res sql.Result
	var err error
	if hasTenant(ctx) {
		res, err = s.db.ExecContext(ctx,
			"DELETE FROM video_render_jobs WHERE id=$1 AND tenant_id=$2",
			id, tenantID)
	} else {
		res, err = s.db.ExecContext(ctx,
			"DELETE FROM video_render_jobs WHERE id=$1",
			id)
	}
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrVideoJobNotFound
	}
	return nil
}

func (s *PGVideoJobStore) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	tenantID := tenantIDArg(ctx)
	var res sql.Result
	var err error
	if hasTenant(ctx) {
		res, err = s.db.ExecContext(ctx, `
			DELETE FROM video_render_jobs
			WHERE status IN ('done','failed','cancelled')
			AND expires_at IS NOT NULL AND expires_at < $1
			AND tenant_id = $2`,
			before, tenantID)
	} else {
		res, err = s.db.ExecContext(ctx, `
			DELETE FROM video_render_jobs
			WHERE status IN ('done','failed','cancelled')
			AND expires_at IS NOT NULL AND expires_at < $1`,
			before)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
