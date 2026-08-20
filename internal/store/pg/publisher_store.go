package pg

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGPublisherKeyStore implements store.PublisherKeystore backed by PostgreSQL.
// Trust anchors live in `publisher_keys` (see migration 000106); fingerprint is
// globally unique because a publisher key is a public-key identity, not a
// per-tenant row.
type PGPublisherKeyStore struct {
	db *sql.DB
}

// NewPGPublisherKeyStore creates a PG-backed publisher keystore.
func NewPGPublisherKeyStore(db *sql.DB) *PGPublisherKeyStore {
	return &PGPublisherKeyStore{db: db}
}

const publisherKeyCols = `id, publisher_id, publisher_type, public_key, fingerprint, status, created_at`

// UpsertKey registers a publisher key. Idempotent: re-registering the same
// fingerprint refreshes publisher metadata and flips status back to active.
func (s *PGPublisherKeyStore) UpsertKey(ctx context.Context, p store.PublisherKeyParams) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO publisher_keys (publisher_id, publisher_type, public_key, fingerprint, status)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (fingerprint) DO UPDATE SET
			publisher_id   = EXCLUDED.publisher_id,
			publisher_type = EXCLUDED.publisher_type,
			public_key     = EXCLUDED.public_key,
			status         = 'active'`,
		p.PublisherID, p.PublisherType, p.PublicKey, p.Fingerprint, store.PublisherKeyStatusActive)
	return err
}

// GetActiveKey returns the active trust anchor for a fingerprint.
func (s *PGPublisherKeyStore) GetActiveKey(ctx context.Context, fingerprint string) (store.PublisherKey, bool, error) {
	var row publisherKeyRow
	err := pkgSqlxDB.GetContext(ctx, &row,
		`SELECT `+publisherKeyCols+` FROM publisher_keys
		 WHERE fingerprint = $1 AND status = $2`,
		fingerprint, store.PublisherKeyStatusActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.PublisherKey{}, false, nil
		}
		return store.PublisherKey{}, false, err
	}
	return row.toStore(), true, nil
}

// ListKeys lists all keys (any status) for a publisher, newest first.
func (s *PGPublisherKeyStore) ListKeys(ctx context.Context, publisherID uuid.UUID) ([]store.PublisherKey, error) {
	var rows []publisherKeyRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT `+publisherKeyCols+` FROM publisher_keys
		 WHERE publisher_id = $1 ORDER BY created_at DESC`,
		publisherID); err != nil {
		return nil, err
	}
	items := make([]store.PublisherKey, len(rows))
	for i, row := range rows {
		items[i] = row.toStore()
	}
	return items, nil
}

// DeactivateKey flips a fingerprint to inactive so signatures from that key
// are no longer accepted for published skills.
func (s *PGPublisherKeyStore) DeactivateKey(ctx context.Context, fingerprint string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE publisher_keys SET status = $1 WHERE fingerprint = $2`,
		store.PublisherKeyStatusInactive, fingerprint)
	return err
}

type publisherKeyRow struct {
	ID            uuid.UUID `db:"id"`
	PublisherID   uuid.UUID `db:"publisher_id"`
	PublisherType string    `db:"publisher_type"`
	PublicKey     []byte    `db:"public_key"`
	Fingerprint   string    `db:"fingerprint"`
	Status        string    `db:"status"`
	CreatedAt     sql.NullTime
}

func (r publisherKeyRow) toStore() store.PublisherKey {
	return store.PublisherKey{
		ID:            r.ID,
		PublisherID:   r.PublisherID,
		PublisherType: r.PublisherType,
		PublicKey:     r.PublicKey,
		Fingerprint:   r.Fingerprint,
		Status:        r.Status,
		CreatedAt:     r.CreatedAt.Time,
	}
}