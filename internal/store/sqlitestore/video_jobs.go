//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteVideoJobStore implements store.VideoRenderJobStore backed by SQLite.
type SQLiteVideoJobStore struct {
	db *sql.DB
}

// NewSQLiteVideoJobStore creates a SQLiteVideoJobStore.
func NewSQLiteVideoJobStore(db *sql.DB) *SQLiteVideoJobStore {
	return &SQLiteVideoJobStore{db: db}
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

func tenantIDArg(ctx context.Context) uuid.UUID {
	return store.TenantIDFromContext(ctx)
}

func hasTenant(ctx context.Context) bool {
	return store.TenantIDFromContext(ctx) != uuid.Nil
}

func (s *SQLiteVideoJobStore) Create(ctx context.Context, job *store.VideoRenderJob) error {
	tenantID := tenantIDArg(ctx)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO video_render_jobs (id, tenant_id, user_id, agent_id, session_key,
			status, engine, storyboard_json)
		VALUES (?,?,?,?,?,?,?,?)`,
		job.ID, tenantID, job.UserID, job.AgentID, job.SessionKey,
		"queued", job.Engine, job.StoryboardJSON)
	return err
}

func (s *SQLiteVideoJobStore) Get(ctx context.Context, id string) (*store.VideoRenderJob, error) {
	tenantID := tenantIDArg(ctx)
	var row *sql.Row
	if hasTenant(ctx) {
		row = s.db.QueryRowContext(ctx,
			`SELECT id, tenant_id, user_id, agent_id, session_key,
				status, engine, storyboard_json, output_path, output_size_bytes, error,
				created_at, updated_at, started_at, finished_at, expires_at
			FROM video_render_jobs WHERE id=? AND tenant_id=?`,
			id, tenantID)
	} else {
		row = s.db.QueryRowContext(ctx,
			`SELECT id, tenant_id, user_id, agent_id, session_key,
				status, engine, storyboard_json, output_path, output_size_bytes, error,
				created_at, updated_at, started_at, finished_at, expires_at
			FROM video_render_jobs WHERE id=?`,
			id)
	}
	j, err := scanVideoJob(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrVideoJobNotFound
	}
	return j, err
}

func (s *SQLiteVideoJobStore) UpdateStatus(ctx context.Context, id string, upd store.VideoJobUpdate) error {
	tenantID := tenantIDArg(ctx)
	var res sql.Result
	var err error
	if hasTenant(ctx) {
		res, err = s.db.ExecContext(ctx, `
			UPDATE video_render_jobs SET
				status = ?,
				error = COALESCE(?, error),
				output_path = COALESCE(?, output_path),
				output_size_bytes = COALESCE(?, output_size_bytes),
				started_at = CASE WHEN ? = 'rendering' AND started_at IS NULL
					THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') ELSE started_at END,
				finished_at = CASE WHEN ? IN ('done','failed','cancelled') AND finished_at IS NULL
					THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') ELSE finished_at END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id = ? AND tenant_id = ?`,
			id, upd.Status, upd.Error, upd.OutputPath, upd.OutputSizeBytes,
			upd.Status, upd.Status,
			id, tenantID)
	} else {
		res, err = s.db.ExecContext(ctx, `
			UPDATE video_render_jobs SET
				status = ?,
				error = COALESCE(?, error),
				output_path = COALESCE(?, output_path),
				output_size_bytes = COALESCE(?, output_size_bytes),
				started_at = CASE WHEN ? = 'rendering' AND started_at IS NULL
					THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') ELSE started_at END,
				finished_at = CASE WHEN ? IN ('done','failed','cancelled') AND finished_at IS NULL
					THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') ELSE finished_at END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id = ?`,
			id, upd.Status, upd.Error, upd.OutputPath, upd.OutputSizeBytes,
			upd.Status, upd.Status, id)
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

func (s *SQLiteVideoJobStore) ListByTenant(ctx context.Context, limit int) ([]store.VideoRenderJob, error) {
	if limit <= 0 {
		limit = 50
	}
	tenantID := tenantIDArg(ctx)
	var rows *sql.Rows
	var err error
	if hasTenant(ctx) {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, tenant_id, user_id, agent_id, session_key,
				status, engine, storyboard_json, output_path, output_size_bytes, error,
				created_at, updated_at, started_at, finished_at, expires_at
			FROM video_render_jobs WHERE tenant_id=? ORDER BY created_at DESC LIMIT ?`,
			tenantID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, tenant_id, user_id, agent_id, session_key,
				status, engine, storyboard_json, output_path, output_size_bytes, error,
				created_at, updated_at, started_at, finished_at, expires_at
			FROM video_render_jobs ORDER BY created_at DESC LIMIT ?`,
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

func (s *SQLiteVideoJobStore) ClaimNextQueued(ctx context.Context) (*store.VideoRenderJob, error) {
	tenantID := tenantIDArg(ctx)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var jobID string
	if hasTenant(ctx) {
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM video_render_jobs
			WHERE status = 'queued' AND tenant_id = ?
			ORDER BY created_at LIMIT 1`, tenantID).Scan(&jobID)
	} else {
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM video_render_jobs
			WHERE status = 'queued'
			ORDER BY created_at LIMIT 1`).Scan(&jobID)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, store.ErrNoQueuedJobs
		}
		return nil, err
	}

	if hasTenant(ctx) {
		_, err = tx.ExecContext(ctx, `
			UPDATE video_render_jobs SET
				status = 'rendering',
				started_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'),
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id = ? AND tenant_id = ?`, jobID, tenantID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE video_render_jobs SET
				status = 'rendering',
				started_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'),
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id = ?`, jobID)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.Get(ctx, jobID)
}

func (s *SQLiteVideoJobStore) DeleteExpired(ctx context.Context, _ time.Time) (int64, error) {
	tenantID := tenantIDArg(ctx)
	var res sql.Result
	var err error
	if hasTenant(ctx) {
		res, err = s.db.ExecContext(ctx, `
			DELETE FROM video_render_jobs
			WHERE status IN ('done','failed','cancelled')
			AND expires_at IS NOT NULL AND expires_at < strftime('%Y-%m-%dT%H:%M:%fZ','now')
			AND tenant_id = ?`,
			tenantID)
	} else {
		res, err = s.db.ExecContext(ctx, `
			DELETE FROM video_render_jobs
			WHERE status IN ('done','failed','cancelled')
			AND expires_at IS NOT NULL AND expires_at < strftime('%Y-%m-%dT%H:%M:%fZ','now')`)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
