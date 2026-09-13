package pg

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGCloudStarredStore implements store.CloudStarredStore backed by Postgres.
// Every query is scoped by the tenant+user carried in the context.
type PGCloudStarredStore struct {
	db *sql.DB
}

// NewPGCloudStarredStore creates a PGCloudStarredStore.
func NewPGCloudStarredStore(db *sql.DB) *PGCloudStarredStore {
	return &PGCloudStarredStore{db: db}
}

const cloudStarredColumns = `id, tenant_id, user_id, account_id, path, name,
	COALESCE(is_dir,false), starred_at`

func (s *PGCloudStarredStore) List(ctx context.Context) ([]store.CloudStarred, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cloudStarredColumns+`
		FROM cloud_starred WHERE tenant_id=$1 AND user_id=$2
		ORDER BY starred_at DESC`,
		store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
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

func (s *PGCloudStarredStore) Add(ctx context.Context, st *store.CloudStarred) error {
	tenantID := store.TenantIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("cloud_starred: missing tenant in context")
	}
	if userID == "" {
		return errors.New("cloud_starred: missing user in context")
	}
	if st.ID == "" {
		st.ID = uuid.NewString()
	}
	// ON CONFLICT DO NOTHING: re-starring a starred path is a no-op, never a
	// duplicate-key 500 (double-click guard).
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cloud_starred (id, tenant_id, user_id, account_id, path, name, is_dir)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (tenant_id, user_id, account_id, path) DO NOTHING`,
		st.ID, tenantID, userID, st.AccountID, st.Path, st.Name, st.IsDir)
	return err
}

func (s *PGCloudStarredStore) Remove(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_starred WHERE id=$1 AND tenant_id=$2 AND user_id=$3`,
		id, store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudStarredNotFound
	}
	return nil
}

func (s *PGCloudStarredStore) RemoveByPath(ctx context.Context, accountID, path string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_starred WHERE account_id=$1 AND path=$2 AND tenant_id=$3 AND user_id=$4`,
		accountID, path, store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
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
	if err := rs.Scan(&st.ID, &st.TenantID, &st.UserID, &st.AccountID, &st.Path,
		&st.Name, &st.IsDir, &st.StarredAt); err != nil {
		return nil, err
	}
	return &st, nil
}
