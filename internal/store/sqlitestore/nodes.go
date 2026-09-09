//go:build sqlite || sqliteonly

package sqlitestore

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

// SQLiteNodeStore implements store.NodeStore backed by SQLite
// (desktop/lite edition).
type SQLiteNodeStore struct {
	db *sql.DB
}

func NewSQLiteNodeStore(db *sql.DB) *SQLiteNodeStore {
	return &SQLiteNodeStore{db: db}
}

const nodeColumns = `id, tenant_id, name, node_key_hash, platform,
 capabilities, trust, last_seen_at, created_at, updated_at, revoked_at`

func scanNode(row interface{ Scan(...any) error }) (*store.Node, error) {
	var n store.Node
	var (
		tenantID   sql.NullString
		capsJSON   sql.NullString
		lastSeenAt nullSqliteTime
		revokedAt  nullSqliteTime
		createdAt  sqliteTime
		updatedAt  sqliteTime
	)
	if err := row.Scan(&n.ID, &tenantID, &n.Name, &n.NodeKeyHash, &n.Platform,
		&capsJSON, &n.Trust, &lastSeenAt, &createdAt, &updatedAt, &revokedAt); err != nil {
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
	n.CreatedAt = createdAt.Time
	n.UpdatedAt = updatedAt.Time
	return &n, nil
}

// Create inserts a new node row (trust defaults to pending when unset).
func (s *SQLiteNodeStore) Create(ctx context.Context, n *store.Node) error {
	if strings.TrimSpace(n.ID) == "" || n.ID == uuid.Nil.String() {
		n.ID = uuid.NewString()
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
	var tenantArg any
	if n.TenantID != nil && *n.TenantID != "" {
		tenantArg = *n.TenantID
	} else if tid := store.TenantIDFromContext(ctx); tid != store.MasterTenantID {
		tenantArg = tid.String()
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO nodes
		 (id, tenant_id, name, node_key_hash, platform, capabilities, trust,
		  created_at, updated_at)
		 VALUES (?1,?2,?3,?4,?5,?6,?7,?8,?9)`,
		n.ID, tenantArg, n.Name, n.NodeKeyHash, n.Platform,
		string(capsJSON), n.Trust, n.CreatedAt.UTC(), n.UpdatedAt.UTC())
	return err
}

func (s *SQLiteNodeStore) GetByID(ctx context.Context, id string) (*store.Node, error) {
	clause, args, err := scopeClause(ctx)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE id = ?`+clause,
		append([]any{id}, args...)...)
	return scanNode(row)
}

func (s *SQLiteNodeStore) GetByKeyHash(ctx context.Context, hash string) (*store.Node, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE node_key_hash = ?1`, hash)
	n, err := scanNode(row)
	if err != nil {
		return nil, fmt.Errorf("node by key hash: %w", err)
	}
	return n, nil
}

func (s *SQLiteNodeStore) List(ctx context.Context) ([]*store.Node, error) {
	clause, args, err := scopeClause(ctx)
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
func (s *SQLiteNodeStore) UpdateRegistration(ctx context.Context, id, name, platform string, capabilities []string) error {
	if capabilities == nil {
		capabilities = []string{}
	}
	capsJSON, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("node capabilities: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET name = ?2, platform = ?3, capabilities = ?4,
		 last_seen_at = ?5, updated_at = ?5
		 WHERE id = ?1 AND trust <> ?6`,
		id, name, platform, string(capsJSON), now, store.NodeTrustRevoked)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node not found or revoked: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *SQLiteNodeStore) TouchSeen(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET last_seen_at = ?2, updated_at = ?2
		 WHERE id = ?1 AND trust <> ?3`,
		id, time.Now().UTC(), store.NodeTrustRevoked)
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
func (s *SQLiteNodeStore) SetTrust(ctx context.Context, id, trust string) error {
	if !store.ValidNodeTrust(trust) {
		return fmt.Errorf("invalid node trust state %q", trust)
	}
	var current string
	if err := s.db.QueryRowContext(ctx,
		`SELECT trust FROM nodes WHERE id = ?1`, id).Scan(&current); err != nil {
		return err
	}
	if err := store.ValidateNodeTrustTransition(current, trust); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET trust = ?2, updated_at = ?3 WHERE id = ?1`,
		id, trust, time.Now().UTC())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *SQLiteNodeStore) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET trust = ?2, revoked_at = ?3, updated_at = ?3
		 WHERE id = ?1 AND trust <> ?2`,
		id, store.NodeTrustRevoked, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Idempotent: already revoked is fine, unknown id is not.
		var exists bool
		if err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM nodes WHERE id = ?1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("node not found: %w", sql.ErrNoRows)
		}
	}
	return nil
}
