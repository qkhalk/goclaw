//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/crypto"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteCloudAccountStore implements store.CloudAccountStore backed by SQLite.
// Token columns are AES-256-GCM encrypted at rest; every query is scoped by
// the tenant+user carried in the context.
type SQLiteCloudAccountStore struct {
	db     *sql.DB
	encKey string
}

func NewSQLiteCloudAccountStore(db *sql.DB, encryptionKey string) *SQLiteCloudAccountStore {
	return &SQLiteCloudAccountStore{db: db, encKey: encryptionKey}
}

const sqliteCloudAccountColumns = `id, tenant_id, user_id, provider, email, display_name,
	scopes, access_token, refresh_token, token_expires_at, status,
	COALESCE(status_message,''), COALESCE(settings,'{}'), COALESCE(shared,0), created_at, updated_at`

func (s *SQLiteCloudAccountStore) Upsert(ctx context.Context, acct *store.CloudAccount) error {
	tenantID := store.TenantIDFromContext(ctx).String()
	userID := store.UserIDFromContext(ctx)
	if tenantID == "" {
		return errors.New("cloud_accounts: missing tenant in context")
	}
	if userID == "" {
		return errors.New("cloud_accounts: missing user in context")
	}

	accessEnc, err := crypto.Encrypt(acct.AccessToken, s.encKey)
	if err != nil {
		return err
	}
	refreshEnc, err := crypto.Encrypt(acct.RefreshToken, s.encKey)
	if err != nil {
		return err
	}

	if acct.ID == "" {
		acct.ID = uuid.NewString()
	}
	if acct.Scopes == "" {
		acct.Scopes = "[]"
	}
	if acct.Status == "" {
		acct.Status = "active"
	}
	if acct.Settings == "" {
		acct.Settings = "{}"
	}

	var expiresAt any
	if acct.TokenExpiresAt != nil {
		expiresAt = acct.TokenExpiresAt.Format(time.RFC3339Nano)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO cloud_accounts
		  (id, tenant_id, user_id, provider, email, display_name, scopes,
		   access_token, refresh_token, token_expires_at, status, settings)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (tenant_id, user_id, provider, email) DO UPDATE SET
		  display_name     = excluded.display_name,
		  scopes           = excluded.scopes,
		  access_token     = excluded.access_token,
		  refresh_token    = excluded.refresh_token,
		  token_expires_at = excluded.token_expires_at,
		  status           = excluded.status,
		  status_message   = '',
		  updated_at       = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		acct.ID, tenantID, userID, acct.Provider, acct.Email, acct.DisplayName,
		acct.Scopes, accessEnc, refreshEnc, expiresAt, acct.Status, acct.Settings)
	return err
}

func (s *SQLiteCloudAccountStore) Get(ctx context.Context, id string) (*store.CloudAccount, error) {
	return s.queryOne(ctx,
		`WHERE id=? AND tenant_id=? AND user_id=?`,
		id, store.TenantIDFromContext(ctx).String(), store.UserIDFromContext(ctx))
}

func (s *SQLiteCloudAccountStore) GetByEmail(ctx context.Context, provider, email string) (*store.CloudAccount, error) {
	return s.queryOne(ctx,
		`WHERE provider=? AND email=? AND tenant_id=? AND user_id=?`,
		provider, email, store.TenantIDFromContext(ctx).String(), store.UserIDFromContext(ctx))
}

func (s *SQLiteCloudAccountStore) List(ctx context.Context) ([]store.CloudAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sqliteCloudAccountColumns+`
		FROM cloud_accounts WHERE tenant_id=? AND user_id=?
		ORDER BY created_at DESC`,
		store.TenantIDFromContext(ctx).String(), store.UserIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.CloudAccount
	for rows.Next() {
		acct, scanErr := s.scan(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *acct)
	}
	return out, rows.Err()
}

func (s *SQLiteCloudAccountStore) UpdateTokens(ctx context.Context, id string, upd store.CloudAccountUpdate) error {
	// Refresh tokens are not re-issued on every grant: an empty upd.RefreshToken
	// keeps the existing one. Encrypted values are never written as empty.
	sets := []string{"updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')"}
	var args []any
	if upd.AccessToken != "" {
		enc, err := crypto.Encrypt(upd.AccessToken, s.encKey)
		if err != nil {
			return err
		}
		sets = append(sets, "access_token = ?")
		args = append(args, enc)
	}
	if upd.RefreshToken != "" {
		enc, err := crypto.Encrypt(upd.RefreshToken, s.encKey)
		if err != nil {
			return err
		}
		sets = append(sets, "refresh_token = ?")
		args = append(args, enc)
	}
	if upd.TokenExpiresAt != nil {
		sets = append(sets, "token_expires_at = ?")
		args = append(args, upd.TokenExpiresAt.Format(time.RFC3339Nano))
	}
	if upd.Status != "" {
		sets = append(sets, "status = ?")
		args = append(args, upd.Status)
	}
	if upd.StatusMessage != "" {
		sets = append(sets, "status_message = ?")
		args = append(args, upd.StatusMessage)
	}

	args = append(args, id, store.TenantIDFromContext(ctx).String(), store.UserIDFromContext(ctx))
	res, err := s.db.ExecContext(ctx,
		`UPDATE cloud_accounts SET `+joinSQLiteSets(sets)+` WHERE id=? AND tenant_id=? AND user_id=?`,
		args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudAccountNotFound
	}
	return nil
}

func (s *SQLiteCloudAccountStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_accounts WHERE id=? AND tenant_id=? AND user_id=?`,
		id, store.TenantIDFromContext(ctx).String(), store.UserIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudAccountNotFound
	}
	return nil
}

func (s *SQLiteCloudAccountStore) queryOne(ctx context.Context, where string, args ...any) (*store.CloudAccount, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+sqliteCloudAccountColumns+` FROM cloud_accounts `+where, args...)
	acct, err := s.scan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrCloudAccountNotFound
		}
		return nil, err
	}
	return acct, nil
}

func (s *SQLiteCloudAccountStore) scan(rs interface{ Scan(dest ...any) error }) (*store.CloudAccount, error) {
	var acct store.CloudAccount
	var accessEnc, refreshEnc string
	var expiresAtStr sql.NullString
	var statusMessage, settings sql.NullString
	var createdStr, updatedStr string
	var sharedInt int
	if err := rs.Scan(&acct.ID, &acct.TenantID, &acct.UserID, &acct.Provider, &acct.Email,
		&acct.DisplayName, &acct.Scopes, &accessEnc, &refreshEnc, &expiresAtStr,
		&acct.Status, &statusMessage, &settings, &sharedInt, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	acct.Shared = sharedInt != 0
	if expiresAtStr.Valid && expiresAtStr.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, expiresAtStr.String); err == nil {
			acct.TokenExpiresAt = &t
		}
	}
	if statusMessage.Valid {
		acct.StatusMessage = statusMessage.String
	}
	if settings.Valid {
		acct.Settings = settings.String
	}
	acct.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	acct.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)

	if dec, err := crypto.Decrypt(accessEnc, s.encKey); err == nil {
		acct.AccessToken = dec
	} else {
		slog.Warn("cloud_accounts: decrypt access_token failed", "account_id", acct.ID)
	}
	if dec, err := crypto.Decrypt(refreshEnc, s.encKey); err == nil {
		acct.RefreshToken = dec
	}
	return &acct, nil
}

func joinSQLiteSets(sets []string) string {
	out := ""
	for i, s := range sets {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// ListShared returns the tenant-wide shared accounts (any owner), newest first.
func (s *SQLiteCloudAccountStore) ListShared(ctx context.Context) ([]store.CloudAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sqliteCloudAccountColumns+`
		FROM cloud_accounts WHERE tenant_id=? AND shared = 1
		ORDER BY created_at DESC`,
		store.TenantIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.CloudAccount
	for rows.Next() {
		acct, scanErr := s.scan(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *acct)
	}
	return out, rows.Err()
}

// SetShared toggles the tenant-wide shared flag on one account (owner-scoped).
func (s *SQLiteCloudAccountStore) SetShared(ctx context.Context, id string, shared bool) error {
	sharedInt := 0
	if shared {
		sharedInt = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE cloud_accounts SET shared=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id=? AND tenant_id=? AND user_id=?`,
		sharedInt, id, store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudAccountNotFound
	}
	return nil
}

// --- CloudBindingStore (same DB handle) ---

// ListBindings returns every binding of the ctx tenant.
func (s *SQLiteCloudAccountStore) ListBindings(ctx context.Context) ([]store.CloudBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sqliteCloudBindingColumns+`
		FROM cloud_account_bindings WHERE tenant_id=? ORDER BY created_at DESC`,
		store.TenantIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.CloudBinding
	for rows.Next() {
		var b store.CloudBinding
		var createdStr, updatedStr string
		if err := rows.Scan(&b.ID, &b.TenantID, &b.ScopeType, &b.ScopeKey, &b.Provider,
			&b.AccountID, &b.CreatedBy, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpsertBinding inserts or updates by (tenant, scope_type, scope_key, provider).
func (s *SQLiteCloudAccountStore) UpsertBinding(ctx context.Context, b *store.CloudBinding) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("cloud_account_bindings: missing tenant in context")
	}
	if b.ID == "" {
		b.ID = store.GenNewID().String()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cloud_account_bindings
		  (id, tenant_id, scope_type, scope_key, provider, account_id, created_by)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT (tenant_id, scope_type, scope_key, provider) DO UPDATE SET
		  account_id = excluded.account_id,
		  created_by = excluded.created_by,
		  updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		b.ID, tenantID, b.ScopeType, b.ScopeKey, b.Provider, b.AccountID, b.CreatedBy)
	return err
}

// DeleteBinding removes one binding by ID (tenant-scoped).
func (s *SQLiteCloudAccountStore) DeleteBinding(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_account_bindings WHERE id=? AND tenant_id=?`,
		id, store.TenantIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudAccountNotFound
	}
	return nil
}

const sqliteCloudBindingColumns = `id, tenant_id, scope_type, scope_key, provider,
	account_id, COALESCE(created_by,''), created_at, updated_at`
