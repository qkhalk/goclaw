package store

import (
	"context"
	"errors"
	"time"
)

// ErrCloudStarredNotFound is returned by CloudStarredStore when no row matches
// the lookup (or the row is outside the caller's tenant+user scope — scope
// violations must not leak existence).
var ErrCloudStarredNotFound = errors.New("cloud starred item not found")

// CloudStarred is one per-user bookmark of a remote file/folder (Drive-style
// "starred"). Providers do not expose star metadata through the rclone rc API,
// so the star lives in GoClaw's DB; path is the encoded remote path.
type CloudStarred struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	AccountID string    `json:"account_id"`
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	IsDir     bool      `json:"is_dir"`
	StarredAt time.Time `json:"starred_at"`
}

// CloudStarredStore persists per-user starred items. All lookups are scoped by
// the ctx tenant + user (store.WithTenantID / store.WithUserID) — a caller can
// never see or mutate another user's stars.
type CloudStarredStore interface {
	// List returns the caller's stars, newest first.
	List(ctx context.Context) ([]CloudStarred, error)
	// Add stars one path. Re-starring an already-starred path is a no-op
	// (ON CONFLICT DO NOTHING) so double-clicks never 500.
	Add(ctx context.Context, s *CloudStarred) error
	// Remove unstares one row by ID (tenant+user scoped). Returns
	// ErrCloudStarredNotFound when missing.
	Remove(ctx context.Context, id string) error
	// RemoveByPath unstares one path of one account (tenant+user scoped).
	// Returns ErrCloudStarredNotFound when not starred.
	RemoveByPath(ctx context.Context, accountID, path string) error
}
