package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PublisherKeyStatus values for publisher_keys.status.
const (
	PublisherKeyStatusActive   = "active"
	PublisherKeyStatusInactive = "inactive"
)

// PublisherKey is a trust-anchor entry: the ed25519 public key a publisher
// uses to sign skill manifests. fingerprint = hex(sha256(public_key))
// computed in Go, globally unique (public-key identity, not per-tenant).
type PublisherKey struct {
	ID            uuid.UUID `json:"id" db:"id"`
	PublisherID   uuid.UUID `json:"publisher_id" db:"publisher_id"`
	PublisherType string    `json:"publisher_type" db:"publisher_type"`
	PublicKey     []byte    `json:"-" db:"public_key"`
	Fingerprint   string    `json:"fingerprint" db:"fingerprint"`
	Status        string    `json:"status" db:"status"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// PublisherKeyParams describes a key to upsert.
type PublisherKeyParams struct {
	PublisherID   uuid.UUID
	PublisherType string // "tenant", "user", or "system"
	PublicKey     []byte
	Fingerprint   string
}

// PublisherKeystore manages publisher trust anchors (Phase 3 W2).
// Implemented by PGPublisherKeyStore (PostgreSQL) and SQLitePublisherKeyStore.
type PublisherKeystore interface {
	// UpsertKey registers a publisher key (insert or reactivate an existing
	// fingerprint row). Idempotent: re-registering the same fingerprint
	// refreshes publisher metadata and flips status back to active.
	UpsertKey(ctx context.Context, p PublisherKeyParams) error
	// GetActiveKey returns the active trust anchor for a fingerprint.
	// ok=false when no active key exists.
	GetActiveKey(ctx context.Context, fingerprint string) (PublisherKey, bool, error)
	// ListKeys lists all keys (any status) for a publisher, newest first.
	ListKeys(ctx context.Context, publisherID uuid.UUID) ([]PublisherKey, error)
	// DeactivateKey flips a fingerprint to inactive so signatures from that
	// key are no longer accepted for published skills.
	DeactivateKey(ctx context.Context, fingerprint string) error
}