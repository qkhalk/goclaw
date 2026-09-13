//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteCloudSyncPairStore implements store.CloudSyncPairStore backed by
// SQLite. Every query is scoped by the tenant carried in the context.
type SQLiteCloudSyncPairStore struct {
	db *sql.DB
}

// NewSQLiteCloudSyncPairStore creates a SQLiteCloudSyncPairStore.
func NewSQLiteCloudSyncPairStore(db *sql.DB) *SQLiteCloudSyncPairStore {
	return &SQLiteCloudSyncPairStore{db: db}
}

const sqliteCloudSyncPairColumns = `id, tenant_id, source_account_id, source_path,
	target_account_id, target_path, interval_minutes, COALESCE(enabled,1),
	last_run_at, COALESCE(last_status,''), COALESCE(last_error,''),
	COALESCE(created_by,''), created_at, updated_at`

func (s *SQLiteCloudSyncPairStore) List(ctx context.Context) ([]store.CloudSyncPair, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sqliteCloudSyncPairColumns+`
		FROM cloud_sync_pairs WHERE tenant_id=?
		ORDER BY created_at DESC`,
		store.TenantIDFromContext(ctx).String())
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

func (s *SQLiteCloudSyncPairStore) Get(ctx context.Context, id string) (*store.CloudSyncPair, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+sqliteCloudSyncPairColumns+`
		FROM cloud_sync_pairs WHERE id=? AND tenant_id=?`,
		id, store.TenantIDFromContext(ctx).String())
	p, err := scanCloudSyncPair(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrCloudSyncPairNotFound
		}
		return nil, err
	}
	return p, nil
}

func (s *SQLiteCloudSyncPairStore) Create(ctx context.Context, p *store.CloudSyncPair) error {
	tenantID := store.TenantIDFromContext(ctx).String()
	if tenantID == "" {
		return errors.New("cloud_sync_pairs: missing tenant in context")
	}
	if p.ID == "" {
		p.ID = store.GenNewID().String()
	}
	if p.SourcePath == "" {
		p.SourcePath = "/"
	}
	if p.TargetPath == "" {
		p.TargetPath = "/"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cloud_sync_pairs
		  (id, tenant_id, source_account_id, source_path, target_account_id,
		   target_path, interval_minutes, enabled, created_by)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		p.ID, tenantID, p.SourceAccountID, p.SourcePath, p.TargetAccountID,
		p.TargetPath, p.IntervalMinutes, boolToInt(p.Enabled), p.CreatedBy)
	return err
}

func (s *SQLiteCloudSyncPairStore) Update(ctx context.Context, p *store.CloudSyncPair) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cloud_sync_pairs SET
		  source_account_id = ?, source_path = ?,
		  target_account_id = ?, target_path = ?,
		  interval_minutes  = ?, enabled = ?,
		  updated_at        = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ? AND tenant_id = ?`,
		p.SourceAccountID, p.SourcePath, p.TargetAccountID, p.TargetPath,
		p.IntervalMinutes, boolToInt(p.Enabled), p.ID, store.TenantIDFromContext(ctx).String())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudSyncPairNotFound
	}
	return nil
}

func (s *SQLiteCloudSyncPairStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_sync_pairs WHERE id=? AND tenant_id=?`,
		id, store.TenantIDFromContext(ctx).String())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudSyncPairNotFound
	}
	return nil
}

func (s *SQLiteCloudSyncPairStore) MarkRunning(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cloud_sync_pairs
		SET last_run_at = ?, last_status = 'running', last_error = '',
		    updated_at  = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ? AND tenant_id = ?`,
		at.Format(time.RFC3339Nano), id, store.TenantIDFromContext(ctx).String())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudSyncPairNotFound
	}
	return nil
}

func (s *SQLiteCloudSyncPairStore) MarkResult(ctx context.Context, id string, status, runErr string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cloud_sync_pairs
		SET last_status = ?, last_error = ?,
		    updated_at  = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ? AND tenant_id = ?`,
		status, runErr, id, store.TenantIDFromContext(ctx).String())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudSyncPairNotFound
	}
	return nil
}

func (s *SQLiteCloudSyncPairStore) DuePairs(ctx context.Context, now time.Time) ([]store.CloudSyncPair, error) {
	// Enabled pairs with an interval only; the "due" comparison happens in Go
	// (see store.CloudSyncPairStore.DuePairs) so both dialects share one rule
	// and the clock stays injectable.
	rows, err := s.db.QueryContext(ctx, `SELECT `+sqliteCloudSyncPairColumns+`
		FROM cloud_sync_pairs
		WHERE tenant_id=? AND enabled = 1 AND interval_minutes > 0`,
		store.TenantIDFromContext(ctx).String())
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

func (s *SQLiteCloudSyncPairStore) FailStaleRunning(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE cloud_sync_pairs
		SET last_status = 'error',
		    last_error  = 'interrupted — gateway restarted mid-run',
		    updated_at  = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE tenant_id = ? AND last_status = 'running' AND last_run_at < ?`,
		store.TenantIDFromContext(ctx).String(), cutoff.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanCloudSyncPair(rs interface{ Scan(dest ...any) error }) (*store.CloudSyncPair, error) {
	var p store.CloudSyncPair
	var lastRunAt sql.NullString
	var enabledInt int
	var createdStr, updatedStr string
	if err := rs.Scan(&p.ID, &p.TenantID, &p.SourceAccountID, &p.SourcePath,
		&p.TargetAccountID, &p.TargetPath, &p.IntervalMinutes, &enabledInt,
		&lastRunAt, &p.LastStatus, &p.LastError, &p.CreatedBy, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	p.Enabled = enabledInt != 0
	if lastRunAt.Valid && lastRunAt.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, lastRunAt.String); err == nil {
			p.LastRunAt = &t
		}
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	return &p, nil
}

// syncPairDue reports whether p is due at now (same rule as the PG impl —
// keep the two in sync; the SQL layers only pre-filter enabled+interval>0).
func syncPairDue(p *store.CloudSyncPair, now time.Time) bool {
	if p.IntervalMinutes <= 0 || !p.Enabled {
		return false
	}
	if p.LastRunAt == nil {
		return true
	}
	return now.Sub(*p.LastRunAt) >= time.Duration(p.IntervalMinutes)*time.Minute
}
