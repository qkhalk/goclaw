package pg

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGCloudSyncPairStore implements store.CloudSyncPairStore backed by Postgres.
// Every query is scoped by the tenant carried in the context.
type PGCloudSyncPairStore struct {
	db *sql.DB
}

// NewPGCloudSyncPairStore creates a PGCloudSyncPairStore.
func NewPGCloudSyncPairStore(db *sql.DB) *PGCloudSyncPairStore {
	return &PGCloudSyncPairStore{db: db}
}

const cloudSyncPairColumns = `id, tenant_id, source_account_id, source_path,
	target_account_id, target_path, interval_minutes, COALESCE(enabled,true),
	last_run_at, COALESCE(last_status,''), COALESCE(last_error,''),
	COALESCE(created_by,''), created_at, updated_at`

func (s *PGCloudSyncPairStore) List(ctx context.Context) ([]store.CloudSyncPair, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cloudSyncPairColumns+`
		FROM cloud_sync_pairs WHERE tenant_id=$1
		ORDER BY created_at DESC`,
		store.TenantIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.CloudSyncPair
	for rows.Next() {
		p, scanErr := scanCloudSyncPair(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *PGCloudSyncPairStore) Get(ctx context.Context, id string) (*store.CloudSyncPair, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+cloudSyncPairColumns+`
		FROM cloud_sync_pairs WHERE id=$1 AND tenant_id=$2`,
		id, store.TenantIDFromContext(ctx))
	p, err := scanCloudSyncPair(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrCloudSyncPairNotFound
		}
		return nil, err
	}
	return p, nil
}

func (s *PGCloudSyncPairStore) Create(ctx context.Context, p *store.CloudSyncPair) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("cloud_sync_pairs: missing tenant in context")
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.SourcePath == "" {
		p.SourcePath = "/"
	}
	if p.TargetPath == "" {
		p.TargetPath = "/"
	}
	// created_by is optional (worker/internal callers have no user in ctx).
	// Bind NULL, never "": an empty string passed for a uuid-typed column
	// fails with `invalid input syntax for type uuid: ""` — and even as TEXT,
	// NULL (not '') is the "no creator" value. Scan COALESCEs back to "".
	var createdBy any // nil → SQL NULL
	if p.CreatedBy != "" {
		createdBy = p.CreatedBy
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cloud_sync_pairs
		  (id, tenant_id, source_account_id, source_path, target_account_id,
		   target_path, interval_minutes, enabled, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		p.ID, tenantID, p.SourceAccountID, p.SourcePath, p.TargetAccountID,
		p.TargetPath, p.IntervalMinutes, p.Enabled, createdBy)
	return err
}

func (s *PGCloudSyncPairStore) Update(ctx context.Context, p *store.CloudSyncPair) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cloud_sync_pairs SET
		  source_account_id = $1, source_path = $2,
		  target_account_id = $3, target_path = $4,
		  interval_minutes  = $5, enabled = $6,
		  updated_at        = NOW()
		WHERE id = $7 AND tenant_id = $8`,
		p.SourceAccountID, p.SourcePath, p.TargetAccountID, p.TargetPath,
		p.IntervalMinutes, p.Enabled, p.ID, store.TenantIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudSyncPairNotFound
	}
	return nil
}

func (s *PGCloudSyncPairStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_sync_pairs WHERE id=$1 AND tenant_id=$2`,
		id, store.TenantIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudSyncPairNotFound
	}
	return nil
}

func (s *PGCloudSyncPairStore) MarkRunning(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cloud_sync_pairs
		SET last_run_at = $1, last_status = 'running', last_error = '', updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3`,
		at, id, store.TenantIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudSyncPairNotFound
	}
	return nil
}

func (s *PGCloudSyncPairStore) MarkResult(ctx context.Context, id string, status, runErr string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cloud_sync_pairs
		SET last_status = $1, last_error = $2, updated_at = NOW()
		WHERE id = $3 AND tenant_id = $4`,
		status, runErr, id, store.TenantIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudSyncPairNotFound
	}
	return nil
}

func (s *PGCloudSyncPairStore) DuePairs(ctx context.Context, now time.Time) ([]store.CloudSyncPair, error) {
	// Enabled pairs with an interval only; the "due" comparison happens in Go
	// (see store.CloudSyncPairStore.DuePairs) so both dialects share one rule
	// and the clock stays injectable.
	rows, err := s.db.QueryContext(ctx, `SELECT `+cloudSyncPairColumns+`
		FROM cloud_sync_pairs
		WHERE tenant_id=$1 AND enabled = true AND interval_minutes > 0`,
		store.TenantIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.CloudSyncPair
	for rows.Next() {
		p, scanErr := scanCloudSyncPair(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if syncPairDue(p, now) {
			out = append(out, *p)
		}
	}
	return out, rows.Err()
}

func (s *PGCloudSyncPairStore) FailStaleRunning(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cloud_sync_pairs
		SET last_status = 'error',
		    last_error  = 'interrupted — gateway restarted mid-run',
		    updated_at  = NOW()
		WHERE tenant_id = $1 AND last_status = 'running' AND last_run_at < $2`,
		store.TenantIDFromContext(ctx), cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanCloudSyncPair(rs interface{ Scan(dest ...any) error }) (*store.CloudSyncPair, error) {
	var p store.CloudSyncPair
	var lastRunAt sql.NullTime
	if err := rs.Scan(&p.ID, &p.TenantID, &p.SourceAccountID, &p.SourcePath,
		&p.TargetAccountID, &p.TargetPath, &p.IntervalMinutes, &p.Enabled,
		&lastRunAt, &p.LastStatus, &p.LastError, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	if lastRunAt.Valid {
		t := lastRunAt.Time
		p.LastRunAt = &t
	}
	return &p, nil
}

// syncPairDue reports whether p is due at now (shared by both dialects).
func syncPairDue(p *store.CloudSyncPair, now time.Time) bool {
	if p.IntervalMinutes <= 0 || !p.Enabled {
		return false
	}
	if p.LastRunAt == nil {
		return true
	}
	return now.Sub(*p.LastRunAt) >= time.Duration(p.IntervalMinutes)*time.Minute
}
