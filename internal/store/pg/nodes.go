package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGNodeStore implements store.NodeStore backed by PostgreSQL.
type PGNodeStore struct {
	db *sql.DB
}

func NewPGNodeStore(db *sql.DB) *PGNodeStore {
	return &PGNodeStore{db: db}
}

// nodeColumns selects capabilities as text so the JSONB value scans into a
// plain string (pgx returns JSONB as string when selected via ::text cast).
const nodeColumns = `id, tenant_id, name, node_key_hash, platform,
 capabilities::text AS capabilities, trust, last_seen_at, created_at,
 updated_at, revoked_at`

func scanNode(row interface{ Scan(...any) error }) (*store.Node, error) {
	var n store.Node
	var (
		tenantID   sql.NullString
		capsJSON   sql.NullString
		lastSeenAt sql.NullTime
		revokedAt  sql.NullTime
	)
	if err := row.Scan(&n.ID, &tenantID, &n.Name, &n.NodeKeyHash, &n.Platform,
		&capsJSON, &n.Trust, &lastSeenAt, &n.CreatedAt, &n.UpdatedAt, &revokedAt); err != nil {
		return nil, err
	}
	if tenantID.Valid {
		tid := tenantID.String
		n.TenantID = &tid
	}
	if capsJSON.Valid && capsJSON.String != "" {
		var caps []string
		if err := json.Unmarshal([]byte(capsJSON.String), &caps); err == nil {
			n.Capabilities = caps
		}
	}
	if lastSeenAt.Valid {
		t := lastSeenAt.Time
		n.LastSeenAt = &t
	}
	if revokedAt.Valid {
		t := revokedAt.Time
		n.RevokedAt = &t
	}
	return &n, nil
}

// Create inserts a new node row (trust defaults to pending when unset).
func (s *PGNodeStore) Create(ctx context.Context, n *store.Node) error {
	if strings.TrimSpace(n.ID) == "" {
		n.ID = store.GenNewID().String()
	}
	now := time.Now()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt = now
	}
	if !store.ValidNodeTrust(n.Trust) {
		n.Trust = store.NodeTrustPending
	}
	if n.Capabilities == nil {
		n.Capabilities = []string{}
	}
	capsJSON, err := json.Marshal(n.Capabilities)
	if err != nil {
		return fmt.Errorf("node capabilities: %w", err)
	}
	if strings.TrimSpace(n.NodeKeyHash) == "" {
		return fmt.Errorf("node key hash required")
	}
	tenantID := tenantIDForInsert(ctx)
	nid, err := uuid.Parse(n.ID)
	if err != nil {
		return fmt.Errorf("node id %q: %w", n.ID, err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO nodes
		 (id, tenant_id, name, node_key_hash, platform, capabilities, trust,
		  created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9)`,
		nid, nullableUUID(tenantID), n.Name, n.NodeKeyHash,
		n.Platform, string(capsJSON), n.Trust, n.CreatedAt, n.UpdatedAt)
	return err
}

func (s *PGNodeStore) GetByID(ctx context.Context, id string) (*store.Node, error) {
	nid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("node id %q: %w", id, err)
	}
	clause, args, _, err := scopeClause(ctx, 2)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE id = $1`+clause,
		append([]any{nid}, args...)...)
	return scanNode(row)
}

func (s *PGNodeStore) GetByKeyHash(ctx context.Context, hash string) (*store.Node, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE node_key_hash = $1`, hash)
	return scanNode(row)
}

func (s *PGNodeStore) List(ctx context.Context) ([]*store.Node, error) {
	clause, args, _, err := scopeClause(ctx, 1)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE 1=1`+clause+
			` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpdateRegistration refreshes daemon-advertised fields + last_seen_at.
// Only pending/trusted nodes may refresh: a revoked node presenting its key
// must not look alive.
func (s *PGNodeStore) UpdateRegistration(ctx context.Context, id, name, platform string, capabilities []string) error {
	nid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("node id %q: %w", id, err)
	}
	if capabilities == nil {
		capabilities = []string{}
	}
	capsJSON, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("node capabilities: %w", err)
	}
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET name = $2, platform = $3, capabilities = $4::jsonb,
		 last_seen_at = $5, updated_at = $5
		 WHERE id = $1 AND trust <> $6`,
		nid, name, platform, string(capsJSON), now, store.NodeTrustRevoked)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node not found or revoked: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *PGNodeStore) TouchSeen(ctx context.Context, id string) error {
	nid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("node id %q: %w", id, err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET last_seen_at = $2, updated_at = $2
		 WHERE id = $1 AND trust <> $3`,
		nid, time.Now(), store.NodeTrustRevoked)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node not found or revoked: %w", sql.ErrNoRows)
	}
	return nil
}

// SetTrust transitions trust state. Revoked is terminal (guarded here and in
// ValidateNodeTrustTransition); updated_at is stamped.
func (s *PGNodeStore) SetTrust(ctx context.Context, id, trust string) error {
	if err := validateTrustTransition(ctx, s.db, id, trust); err != nil {
		return err
	}
	nid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("node id %q: %w", id, err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET trust = $2, updated_at = $3 WHERE id = $1`,
		nid, trust, time.Now())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *PGNodeStore) Revoke(ctx context.Context, id string) error {
	nid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("node id %q: %w", id, err)
	}
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET trust = $2, revoked_at = $3, updated_at = $3
		 WHERE id = $1 AND trust <> $2`,
		nid, store.NodeTrustRevoked, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Idempotent: already revoked is fine, unknown id is not.
		var exists bool
		if err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM nodes WHERE id = $1)`, nid).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("node not found: %w", sql.ErrNoRows)
		}
	}
	return nil
}

// validateTrustTransition reads the current trust and applies the shared
// transition rules before the UPDATE.
func validateTrustTransition(ctx context.Context, db *sql.DB, id, trust string) error {
	if !store.ValidNodeTrust(trust) {
		return fmt.Errorf("invalid node trust state %q", trust)
	}
	nid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("node id %q: %w", id, err)
	}
	var current string
	if err := db.QueryRowContext(ctx,
		`SELECT trust FROM nodes WHERE id = $1`, nid).Scan(&current); err != nil {
		return err
	}
	return store.ValidateNodeTrustTransition(current, trust)
}
