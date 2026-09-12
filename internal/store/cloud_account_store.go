package store

import (
	"context"
	"errors"
	"time"
)

// ErrCloudAccountNotFound is returned by CloudAccountStore when no row matches
// the lookup (or the row is outside the caller's tenant/user scope — callers
// cannot distinguish: scope violations must not leak existence).
var ErrCloudAccountNotFound = errors.New("cloud account not found")

// CloudAccount is one OAuth-connected cloud provider account owned by a
// (tenant, user) pair. AccessToken/RefreshToken are stored AES-256-GCM
// encrypted by the store implementation ("aes-gcm:" prefix, same scheme as
// llm_providers.api_key); the in-memory struct carries the plaintext.
type CloudAccount struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	UserID         string     `json:"user_id"`
	Provider       string     `json:"provider"` // "google"
	Email          string     `json:"email"`
	DisplayName    string     `json:"display_name"`
	Scopes         string     `json:"scopes"` // JSON array string
	AccessToken    string     `json:"-"`      // plaintext in memory, encrypted at rest
	RefreshToken   string     `json:"-"`      // plaintext in memory, encrypted at rest
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
	Status         string     `json:"status"` // active | expired | revoked | error
	StatusMessage  string     `json:"status_message"`
	Settings       string     `json:"settings"` // JSON object string
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// CloudAccountUpdate carries the mutable fields for UpdateTokens/UpdateStatus.
// Empty string fields are left unchanged.
type CloudAccountUpdate struct {
	AccessToken    string
	RefreshToken   string // empty = keep existing (Google does not always re-issue)
	TokenExpiresAt *time.Time
	Status         string
	StatusMessage  string
}

// CloudAccountStore persists OAuth cloud connections. All lookups are scoped
// by tenant_id + user_id taken from the context (store.WithTenantID /
// store.WithUserID) — a caller can never read or mutate another user's
// connection.
type CloudAccountStore interface {
	// Upsert inserts or updates by (tenant, user, provider, email). Used by the
	// OAuth callback after a successful token exchange.
	Upsert(ctx context.Context, acct *CloudAccount) error
	// Get returns the account by ID, or ErrCloudAccountNotFound. Scoped to the
	// ctx tenant+user.
	Get(ctx context.Context, id string) (*CloudAccount, error)
	// GetByEmail returns the account by provider+email, or
	// ErrCloudAccountNotFound. Scoped to the ctx tenant+user.
	GetByEmail(ctx context.Context, provider, email string) (*CloudAccount, error)
	// List returns all accounts of the ctx tenant+user, newest first.
	List(ctx context.Context) ([]CloudAccount, error)
	// UpdateTokens applies CloudAccountUpdate to the account with the given ID
	// (scoped to ctx tenant+user). Returns ErrCloudAccountNotFound if missing.
	UpdateTokens(ctx context.Context, id string, upd CloudAccountUpdate) error
	// Delete removes the account (scoped to ctx tenant+user).
	Delete(ctx context.Context, id string) error
}
