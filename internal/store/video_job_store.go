package store

import (
	"context"
	"errors"
	"time"
)

// ErrNoQueuedJobs is returned by ClaimNextQueued when no queued job is available.
var ErrNoQueuedJobs = errors.New("no queued video render jobs")

// ErrVideoJobNotFound is returned by Get/UpdateStatus when the job does not exist.
var ErrVideoJobNotFound = errors.New("video render job not found")

// VideoRenderJob is one storyboard → MP4 render request. Tenant-scoped.
type VideoRenderJob struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	UserID           string     `json:"user_id"`
	AgentID          string     `json:"agent_id"`
	SessionKey       string     `json:"session_key"`
	Status           string     `json:"status"`
	Engine           string     `json:"engine"`
	StoryboardJSON   string     `json:"storyboard_json"`
	OutputPath       string     `json:"output_path"`
	OutputSizeBytes  int64      `json:"output_size_bytes"`
	Error            string     `json:"error"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
}

// VideoJobUpdate describes fields to patch in UpdateStatus.
type VideoJobUpdate struct {
	Status           string
	Error            *string
	OutputPath       *string
	OutputSizeBytes  *int64
}

// VideoRenderJobStore persists storyboard-to-MP4 render jobs. All queries are
// scoped by the ctx tenant (store.WithTenantID).
type VideoRenderJobStore interface {
	// Create inserts a new job in queued status.
	Create(ctx context.Context, job *VideoRenderJob) error
	// Get returns a single job by ID (tenant-scoped).
	Get(ctx context.Context, id string) (*VideoRenderJob, error)
	// UpdateStatus patches mutable job fields and transitions timestamps
	// automatically (started_at when entering rendering, finished_at on
	// terminal status).
	UpdateStatus(ctx context.Context, id string, upd VideoJobUpdate) error
	// ListByTenant returns the most recent jobs for the caller's tenant,
	// newest first, capped by limit.
	ListByTenant(ctx context.Context, limit int) ([]VideoRenderJob, error)
	// ClaimNextQueued atomically transitions the oldest queued job to
	// rendering and returns it. Returns ErrNoQueuedJobs when empty.
	ClaimNextQueued(ctx context.Context) (*VideoRenderJob, error)
	// DeleteExpired removes terminal jobs whose expires_at < before.
	// Returns the number of deleted rows.
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
	// Delete removes a single terminal job row (tenant-scoped). Callers must
	// verify the job is not queued/rendering before deleting.
	Delete(ctx context.Context, id string) error
}
