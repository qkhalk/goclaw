package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGNodeLeaseStore implements store.NodeLeaseStore backed by PostgreSQL.
type PGNodeLeaseStore struct {
	db *sql.DB
}

func NewPGNodeLeaseStore(db *sql.DB) *PGNodeLeaseStore {
	return &PGNodeLeaseStore{db: db}
}

const nodeLeaseColumns = `id, node_id, client_id, user_id, tenant_id, resume_token,
 session_epoch, status, issued_at, last_seen_at, expires_at`

func scanNodeLease(row interface{ Scan(...any) error }) (*store.NodeLease, error) {
	var l store.NodeLease
	var tenantID sql.NullString
	var epoch int
	if err := row.Scan(&l.ID, &l.NodeID, &l.ClientID, &l.UserID, &tenantID,
		&l.ResumeToken, &epoch, &l.Status, &l.IssuedAt, &l.LastSeenAt, &l.ExpiresAt); err != nil {
		return nil, err
	}
	l.TenantID = tenantID.String
	l.SessionEpoch = epoch
	return &l, nil
}

// UpsertLease inserts or refreshes the lease keyed by node_id. An empty
// ResumeToken keeps the stored token (heartbeat-style refresh); a non-empty
// token rotates it.
func (s *PGNodeLeaseStore) UpsertLease(ctx context.Context, lease *store.NodeLease) error {
	if strings.TrimSpace(lease.ID) == "" {
		lease.ID = store.GenNewID().String()
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
	tenantID := tenantIDForInsert(ctx)
	tokenVal := lease.ResumeToken
	if strings.TrimSpace(tokenVal) == "" {
		tokenVal = uuid.NewString()
	}
	lease.ResumeToken = tokenVal
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO node_leases
		 (id, node_id, client_id, user_id, tenant_id, resume_token, session_epoch,
		  status, issued_at, last_seen_at, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (node_id) DO UPDATE SET
		   client_id    = EXCLUDED.client_id,
		   user_id      = EXCLUDED.user_id,
		   tenant_id    = EXCLUDED.tenant_id,
		   resume_token = CASE WHEN $6 <> '' THEN $6 ELSE node_leases.resume_token END,
		   status       = EXCLUDED.status,
		   last_seen_at = EXCLUDED.last_seen_at,
		   expires_at   = EXCLUDED.expires_at`,
		lease.ID, lease.NodeID, lease.ClientID, lease.UserID, nullableUUID(tenantID),
		tokenVal, lease.SessionEpoch, lease.Status, lease.IssuedAt, lease.LastSeenAt, lease.ExpiresAt,
	)
	return err
}

func (s *PGNodeLeaseStore) GetByResumeToken(ctx context.Context, token string) (*store.NodeLease, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeLeaseColumns+` FROM node_leases WHERE resume_token = $1`, token)
	return scanNodeLease(row)
}

func (s *PGNodeLeaseStore) GetByNodeID(ctx context.Context, nodeID string) (*store.NodeLease, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeLeaseColumns+` FROM node_leases WHERE node_id = $1`, nodeID)
	return scanNodeLease(row)
}

func (s *PGNodeLeaseStore) TouchHeartbeat(ctx context.Context, nodeID string, ttl time.Duration) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE node_leases SET last_seen_at = $2, expires_at = $3,
		 status = CASE WHEN status = 'expired' THEN status ELSE 'online' END
		 WHERE node_id = $1`,
		nodeID, time.Now(), time.Now().Add(ttl))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node lease not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *PGNodeLeaseStore) MarkStatus(ctx context.Context, nodeID, status string) error {
	if !store.ValidNodeLeaseStatus(status) {
		return fmt.Errorf("invalid node lease status %q", status)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE node_leases SET status = $2 WHERE node_id = $1`, nodeID, status)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node lease not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (s *PGNodeLeaseStore) ExpireStale(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE node_leases SET status = 'expired'
		 WHERE status <> 'expired' AND expires_at < $1`, time.Now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func nullableUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
