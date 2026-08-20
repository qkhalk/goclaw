//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLitePublisherKeyStore implements store.PublisherKeystore backed by SQLite.
// Trust anchors live in `publisher_keys` (see schema.go patch 69).
type SQLitePublisherKeyStore struct {
	db *sql.DB
}

// NewSQLitePublisherKeyStore creates a SQLite-backed publisher keystore.
func NewSQLitePublisherKeyStore(db *sql.DB) *SQLitePublisherKeyStore {
	return &SQLitePublisherKeyStore{db: db}
}

const sqlitePublisherKeyCols = `id, publisher_id, publisher_type, public_key, fingerprint, status, created_at`

// UpsertKey registers a publisher key. Idempotent: re-registering the same
// fingerprint refreshes publisher metadata and flips status back to active.
func (s *SQLitePublisherKeyStore) UpsertKey(ctx context.Context, p store.PublisherKeyParams) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO publisher_keys (publisher_id, publisher_type, public_key, fingerprint, status)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (fingerprint) DO UPDATE SET
			publisher_id   = excluded.publisher_id,
			publisher_type = excluded.publisher_type,
			public_key     = excluded.public_key,
			status         = 'active'`,
		p.PublisherID.String(), p.PublisherType, p.PublicKey, p.Fingerprint, store.PublisherKeyStatusActive)
	return err
}

// GetActiveKey returns the active trust anchor for a fingerprint.
func (s *SQLitePublisherKeyStore) GetActiveKey(ctx context.Context, fingerprint string) (store.PublisherKey, bool, error) {
	var row sqlitePublisherKeyRow
	err := pkgSqlxDB.GetContext(ctx, &row,
		`SELECT `+sqlitePublisherKeyCols+` FROM publisher_keys
		 WHERE fingerprint = ? AND status = ?`,
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
func (s *SQLitePublisherKeyStore) ListKeys(ctx context.Context, publisherID uuid.UUID) ([]store.PublisherKey, error) {
	var rows []sqlitePublisherKeyRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT `+sqlitePublisherKeyCols+` FROM publisher_keys
		 WHERE publisher_id = ? ORDER BY created_at DESC`,
		publisherID.String()); err != nil {
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
func (s *SQLitePublisherKeyStore) DeactivateKey(ctx context.Context, fingerprint string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE publisher_keys SET status = ? WHERE fingerprint = ?`,
		store.PublisherKeyStatusInactive, fingerprint)
	return err
}

type sqlitePublisherKeyRow struct {
	ID            uuid.UUID
	PublisherID   uuid.UUID
	PublisherType string
	PublicKey     []byte
	Fingerprint   string
	Status        string
	CreatedAt     sqliteTime
}

func (r sqlitePublisherKeyRow) toStore() store.PublisherKey {
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