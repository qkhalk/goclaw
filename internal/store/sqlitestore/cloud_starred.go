//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteCloudStarredStore implements store.CloudStarredStore backed by SQLite.
// Every query is scoped by the tenant+user carried in the context.
type SQLiteCloudStarredStore struct {
	db *sql.DB
}

// NewSQLiteCloudStarredStore creates a SQLiteCloudStarredStore.
func NewSQLiteCloudStarredStore(db *sql.DB) *SQLiteCloudStarredStore {
	return &SQLiteCloudStarredStore{db: db}
}

const sqliteCloudStarredColumns = `id, tenant_id, user_id, account_id, path, name,
	COALESCE(is_dir,0), starred_at`

func (s *SQLiteCloudStarredStore) List(ctx context.Context) ([]store.CloudStarred, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sqliteCloudStarredColumns+`
		FROM cloud_starred WHERE tenant_id=? AND user_id=?
		ORDER BY starred_at DESC`,
		store.TenantIDFromContext(ctx).String(), store.UserIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.CloudStarred
	for rows.Next() {
		st, scanErr := scanCloudStarred(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *st)
	}
	return out, rows.Err()
}

func (s *SQLiteCloudStarredStore) Add(ctx context.Context, st *store.CloudStarred) error {
	tenantID := store.TenantIDFromContext(ctx).String()
	userID := store.UserIDFromContext(ctx)
	if tenantID == "" {
		return errors.New("cloud_starred: missing tenant in context")
	}
	if userID == "" {
		return errors.New("cloud_starred: missing user in context")
	}
	if st.ID == "" {
		st.ID = store.GenNewID().String()
	}
	// ON CONFLICT DO NOTHING: re-starring a starred path is a no-op, never a
	// duplicate-key 500 (double-click guard).
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cloud_starred (id, tenant_id, user_id, account_id, path, name, is_dir)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT (tenant_id, user_id, account_id, path) DO NOTHING`,
		st.ID, tenantID, userID, st.AccountID, st.Path, st.Name, boolToInt(st.IsDir))
	return err
}

func (s *SQLiteCloudStarredStore) Remove(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_starred WHERE id=? AND tenant_id=? AND user_id=?`,
		id, store.TenantIDFromContext(ctx).String(), store.UserIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudStarredNotFound
	}
	return nil
}

func (s *SQLiteCloudStarredStore) RemoveByPath(ctx context.Context, accountID, path string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_starred WHERE account_id=? AND path=? AND tenant_id=? AND user_id=?`,
		accountID, path, store.TenantIDFromContext(ctx).String(), store.UserIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudStarredNotFound
	}
	return nil
}

func scanCloudStarred(rs interface{ Scan(dest ...any) error }) (*store.CloudStarred, error) {
	var st store.CloudStarred
	var isDirInt int
	var starredStr string
	if err := rs.Scan(&st.ID, &st.TenantID, &st.UserID, &st.AccountID, &st.Path,
		&st.Name, &isDirInt, &starredStr); err != nil {
		return nil, err
	}
	st.IsDir = isDirInt != 0
	st.StarredAt, _ = time.Parse(time.RFC3339Nano, starredStr)
	return &st, nil
}
