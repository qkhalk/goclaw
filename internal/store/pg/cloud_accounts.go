package pg

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/crypto"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGCloudAccountStore implements store.CloudAccountStore backed by Postgres.
// Token columns are AES-256-GCM encrypted at rest (llm_providers pattern);
// every query is scoped by the tenant+user carried in the context.
type PGCloudAccountStore struct {
	db     *sql.DB
	encKey string
}

// NewPGCloudAccountStore creates a PGCloudAccountStore.
func NewPGCloudAccountStore(db *sql.DB, encryptionKey string) *PGCloudAccountStore {
	return &PGCloudAccountStore{db: db, encKey: encryptionKey}
}

func (s *PGCloudAccountStore) Upsert(ctx context.Context, acct *store.CloudAccount) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("cloud_accounts: missing tenant in context")
	}
	userID := store.UserIDFromContext(ctx)
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

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO cloud_accounts
		  (id, tenant_id, user_id, provider, email, display_name, scopes,
		   access_token, refresh_token, token_expires_at, status, settings)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (tenant_id, user_id, provider, email) DO UPDATE SET
		  display_name     = EXCLUDED.display_name,
		  scopes           = EXCLUDED.scopes,
		  access_token     = EXCLUDED.access_token,
		  refresh_token    = EXCLUDED.refresh_token,
		  token_expires_at = EXCLUDED.token_expires_at,
		  status           = EXCLUDED.status,
		  status_message   = '',
		  updated_at       = NOW()`,
		acct.ID, tenantID, userID, acct.Provider, acct.Email, acct.DisplayName,
		acct.Scopes, accessEnc, refreshEnc, acct.TokenExpiresAt, acct.Status, acct.Settings)
	return err
}

func (s *PGCloudAccountStore) Get(ctx context.Context, id string) (*store.CloudAccount, error) {
	return s.queryOne(ctx,
		`WHERE id=$1 AND tenant_id=$2 AND user_id=$3`,
		id, store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
}

func (s *PGCloudAccountStore) GetByEmail(ctx context.Context, provider, email string) (*store.CloudAccount, error) {
	return s.queryOne(ctx,
		`WHERE provider=$1 AND email=$2 AND tenant_id=$3 AND user_id=$4`,
		provider, email, store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
}

func (s *PGCloudAccountStore) List(ctx context.Context) ([]store.CloudAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cloudAccountColumns+`
		FROM cloud_accounts WHERE tenant_id=$1 AND user_id=$2
		ORDER BY created_at DESC`,
		store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
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

func (s *PGCloudAccountStore) UpdateTokens(ctx context.Context, id string, upd store.CloudAccountUpdate) error {
	// Refresh tokens are not re-issued on every grant: an empty upd.RefreshToken
	// keeps the existing one. Encrypted values are never written as empty.
	sets := []string{"updated_at = NOW()"}
	var args []any
	if upd.AccessToken != "" {
		enc, err := crypto.Encrypt(upd.AccessToken, s.encKey)
		if err != nil {
			return err
		}
		sets = append(sets, "access_token = $"+strconv.Itoa(len(args)+1))
		args = append(args, enc)
	}
	if upd.RefreshToken != "" {
		enc, err := crypto.Encrypt(upd.RefreshToken, s.encKey)
		if err != nil {
			return err
		}
		sets = append(sets, "refresh_token = $"+strconv.Itoa(len(args)+1))
		args = append(args, enc)
	}
	if upd.TokenExpiresAt != nil {
		sets = append(sets, "token_expires_at = $"+strconv.Itoa(len(args)+1))
		args = append(args, *upd.TokenExpiresAt)
	}
	if upd.Status != "" {
		sets = append(sets, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, upd.Status)
	}
	if upd.StatusMessage != "" {
		sets = append(sets, "status_message = $"+strconv.Itoa(len(args)+1))
		args = append(args, upd.StatusMessage)
	}

	// WHERE placeholders come after the SET placeholders.
	idN := len(args) + 1
	args = append(args, id, store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
	res, err := s.db.ExecContext(ctx,
		`UPDATE cloud_accounts SET `+strings.Join(sets, ", ")+
			` WHERE id=$`+strconv.Itoa(idN)+
			` AND tenant_id=$`+strconv.Itoa(idN+1)+
			` AND user_id=$`+strconv.Itoa(idN+2),
		args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudAccountNotFound
	}
	return nil
}

func (s *PGCloudAccountStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM cloud_accounts WHERE id=$1 AND tenant_id=$2 AND user_id=$3`,
		id, store.TenantIDFromContext(ctx), store.UserIDFromContext(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrCloudAccountNotFound
	}
	return nil
}

const cloudAccountColumns = `id, tenant_id, user_id, provider, email, display_name,
	scopes, access_token, refresh_token, token_expires_at, status,
	COALESCE(status_message,''), COALESCE(settings,'{}'), created_at, updated_at`

// queryOne runs a scoped SELECT and decrypts token columns. A row outside the
// caller's tenant/user scope returns ErrCloudAccountNotFound (no existence leak).
func (s *PGCloudAccountStore) queryOne(ctx context.Context, where string, args ...any) (*store.CloudAccount, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+cloudAccountColumns+` FROM cloud_accounts `+where, args...)
	acct, err := s.scan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrCloudAccountNotFound
		}
		return nil, err
	}
	return acct, nil
}

func (s *PGCloudAccountStore) scan(rs interface{ Scan(dest ...any) error }) (*store.CloudAccount, error) {
	var acct store.CloudAccount
	var accessEnc, refreshEnc string
	var statusMessage, settings sql.NullString
	if err := rs.Scan(&acct.ID, &acct.TenantID, &acct.UserID, &acct.Provider, &acct.Email,
		&acct.DisplayName, &acct.Scopes, &accessEnc, &refreshEnc, &acct.TokenExpiresAt,
		&acct.Status, &statusMessage, &settings, &acct.CreatedAt, &acct.UpdatedAt); err != nil {
		return nil, err
	}
	if statusMessage.Valid {
		acct.StatusMessage = statusMessage.String
	}
	if settings.Valid {
		acct.Settings = settings.String
	}
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
