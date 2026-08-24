//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteNodeLeaseStore implements store.NodeLeaseStore backed by SQLite
// (desktop/lite edition).
type SQLiteNodeLeaseStore struct {
	db *sql.DB
}

func NewSQLiteNodeLeaseStore(db *sql.DB) *SQLiteNodeLeaseStore {
	return &SQLiteNodeLeaseStore{db: db}
}

const nodeLeaseColumns = `id, node_id, client_id, user_id, tenant_id, resume_token,
 session_epoch, status, issued_at, last_seen_at, expires_at`

func scanNodeLease(row interface{ Scan(...any) error }) (*store.NodeLease, error) {
	var l store.NodeLease
	var tenantID sql.NullString
	var epoch int
	// Timestamps are stored as TEXT; scan through sqliteTime because
	// modernc.org/sqlite hands strings back, which cannot Scan into time.Time.
	var issuedAt, lastSeenAt, expiresAt sqliteTime
	if err := row.Scan(&l.ID, &l.NodeID, &l.ClientID, &l.UserID, &tenantID,
		&l.ResumeToken, &epoch, &l.Status, &issuedAt, &lastSeenAt, &expiresAt); err != nil {
		return nil, err
	}
	l.TenantID = tenantID.String
	l.SessionEpoch = epoch
	l.IssuedAt = issuedAt.Time
	l.LastSeenAt = lastSeenAt.Time
	l.ExpiresAt = expiresAt.Time
	return &l, nil
}

// UpsertLease inserts or refreshes the lease keyed by node_id. An empty
// ResumeToken keeps the stored token; a non-empty token rotates it.
func (s *SQLiteNodeLeaseStore) UpsertLease(ctx context.Context, lease *store.NodeLease) error {
	if lease.ID == "" || lease.ID == uuid.Nil.String() {
		lease.ID = uuid.NewString()
	}
	now := time.Now()
	if lease.IssuedAt.IsZero() {
		lease.IssuedAt = now
	}
	if lease.LastSeenAt.IsZero() {
		lease.LastSeenAt = now
	}
	if lease.ExpiresAt.IsZero() {
		lease.ExpiresAt = now.Add(60 * time.Second)
	}
	if !store.ValidNodeLeaseStatus(lease.Status) {
		lease.Status = store.NodeLeaseStatusOnline
	}
	if lease.SessionEpoch <= 0 {
		lease.SessionEpoch = 1
	}
	tokenVal := strings.TrimSpace(lease.ResumeToken)
	if tokenVal == "" {
		tokenVal = uuid.NewString()
	}
	lease.ResumeToken = tokenVal
	tenantArg := any(nil)
	if lease.TenantID != "" {
		tenantArg = lease.TenantID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO node_leases
		 (id, node_id, client_id, user_id, tenant_id, resume_token, session_epoch,
		  status, issued_at, last_seen_at, expires_at)
		 VALUES (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11)
		 ON CONFLICT(node_id) DO UPDATE SET
		   client_id    = excluded.client_id,
		   user_id      = excluded.user_id,
		   tenant_id    = excluded.tenant_id,
		   resume_token = excluded.resume_token,
		   status       = excluded.status,
		   last_seen_at = excluded.last_seen_at,
		   expires_at   = excluded.expires_at`,
		lease.ID, lease.NodeID, lease.ClientID, lease.UserID, tenantArg,
		tokenVal, lease.SessionEpoch, lease.Status,
		lease.IssuedAt.UTC(), lease.LastSeenAt.UTC(), lease.ExpiresAt.UTC(),
	)
	return err
}

func (s *SQLiteNodeLeaseStore) GetByResumeToken(ctx context.Context, token string) (*store.NodeLease, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeLeaseColumns+` FROM node_leases WHERE resume_token = ?1`, token)
	l, err := scanNodeLease(row)
	if err != nil {
		return nil, fmt.Errorf("node lease by token: %w", err)
	}
	return l, nil
}

func (s *SQLiteNodeLeaseStore) GetByNodeID(ctx context.Context, nodeID string) (*store.NodeLease, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeLeaseColumns+` FROM node_leases WHERE node_id = ?1`, nodeID)
	l, err := scanNodeLease(row)
	if err != nil {
		return nil, fmt.Errorf("node lease by id: %w", err)
	}
	return l, nil
}

func (s *SQLiteNodeLeaseStore) TouchHeartbeat(ctx context.Context, nodeID string, ttl time.Duration) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE node_leases SET last_seen_at = ?2, expires_at = ?3,
		 status = CASE WHEN status = 'expired' THEN status ELSE 'online' END
		 WHERE node_id = ?1`,
		nodeID, time.Now().UTC(), time.Now().Add(ttl).UTC())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node lease not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *SQLiteNodeLeaseStore) MarkStatus(ctx context.Context, nodeID, status string) error {
	if !store.ValidNodeLeaseStatus(status) {
		return fmt.Errorf("invalid node lease status %q", status)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE node_leases SET status = ?2 WHERE node_id = ?1`, nodeID, status)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node lease not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *SQLiteNodeLeaseStore) ExpireStale(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE node_leases SET status = 'expired'
		 WHERE status <> 'expired' AND expires_at < ?1`, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
