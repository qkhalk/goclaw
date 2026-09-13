package store

import (
	"context"
	"errors"
	"time"
)

// ErrCloudSyncPairNotFound is returned by CloudSyncPairStore when no row
// matches the lookup (or the row is outside the caller's tenant scope —
// scope violations must not leak existence).
var ErrCloudSyncPairNotFound = errors.New("cloud sync pair not found")

// Cloud sync pair run statuses (cloud_sync_pairs.last_status).
const (
	SyncStatusRunning = "running"
	SyncStatusOK      = "ok"
	SyncStatusError   = "error"
)

// CloudSyncPair is one one-way (additive mirror) folder sync configuration
// between two connected accounts: everything under source_path of the source
// account is copied (never deleted) into target_path of the target account.
// IntervalMinutes = 0 means manual ("run now") only.
type CloudSyncPair struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	SourceAccountID string     `json:"source_account_id"`
	SourcePath      string     `json:"source_path"`
	TargetAccountID string     `json:"target_account_id"`
	TargetPath      string     `json:"target_path"`
	IntervalMinutes int        `json:"interval_minutes"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	// LastStatus is ok | error | running ("" = never ran).
	LastStatus string    `json:"last_status,omitempty"`
	LastError  string    `json:"last_error,omitempty"`
	CreatedBy  string    `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CloudSyncPairStore persists tenant-level sync pair configuration. All
// lookups are scoped by the ctx tenant (store.WithTenantID); the sync worker
// re-scopes the base context per pair before calling.
type CloudSyncPairStore interface {
	// List returns every sync pair of the ctx tenant, newest first.
	List(ctx context.Context) ([]CloudSyncPair, error)
	// Get returns one pair by ID (tenant-scoped), or ErrCloudSyncPairNotFound.
	Get(ctx context.Context, id string) (*CloudSyncPair, error)
	// Create inserts a new pair (ID assigned when empty).
	Create(ctx context.Context, p *CloudSyncPair) error
	// Update replaces the mutable fields of one pair (paths, accounts,
	// interval, enabled). Tenant-scoped.
	Update(ctx context.Context, p *CloudSyncPair) error
	// Delete removes one pair (tenant-scoped).
	Delete(ctx context.Context, id string) error
	// MarkRunning records the run start: last_run_at = at, last_status =
	// running, last_error = "". Tenant-scoped.
	MarkRunning(ctx context.Context, id string, at time.Time) error
	// MarkResult records the run outcome: last_status = status (ok|error),
	// last_error = runErr. last_run_at keeps the run START time set by
	// MarkRunning. Tenant-scoped.
	MarkResult(ctx context.Context, id string, status string, runErr string) error
	// DuePairs returns the enabled pairs with interval_minutes > 0 that are
	// due at `now` (never ran, or last run started at least interval minutes
	// ago). The due filter is applied in Go so both dialects share one
	// implementation and the clock stays injectable for tests.
	DuePairs(ctx context.Context, now time.Time) ([]CloudSyncPair, error)
	// FailStaleRunning marks pairs stuck in "running" with last_run_at older
	// than `cutoff` as errored (gateway crash recovery) and returns how many
	// rows were fixed. Tenant-scoped (the worker calls it per tenant batch).
	FailStaleRunning(ctx context.Context, cutoff time.Time) (int64, error)
}
