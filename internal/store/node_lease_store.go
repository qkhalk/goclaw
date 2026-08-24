package store

import (
	"context"
	"time"
)

// NodeLeaseStatus values for the lease lifecycle. A WebSocket close moves the
// lease to RECONNECTING (never a logout); only expiry after the grace window
// terminates it.
const (
	NodeLeaseStatusOnline       = "online"
	NodeLeaseStatusReconnecting = "reconnecting"
	NodeLeaseStatusOfflineGrace = "offline_grace"
	NodeLeaseStatusExpired      = "expired"
)

// NodeLease is a device-level connectivity record, deliberately separate from
// authentication (gateway token / API key) and from agent sessions (Paseo
// plan Phase 1: identity != connection != lease != session).
type NodeLease struct {
	ID           string    `json:"id"`
	NodeID       string    `json:"nodeId"`
	ClientID     string    `json:"clientId"`
	UserID       string    `json:"userId"`
	TenantID     string    `json:"tenantId,omitempty"`
	ResumeToken  string    `json:"resumeToken"`
	SessionEpoch int       `json:"sessionEpoch"`
	Status       string    `json:"status"`
	IssuedAt     time.Time `json:"issuedAt"`
	LastSeenAt   time.Time `json:"lastSeenAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// ValidNodeLeaseStatus reports whether s is a known lease status.
func ValidNodeLeaseStatus(s string) bool {
	switch s {
	case NodeLeaseStatusOnline, NodeLeaseStatusReconnecting,
		NodeLeaseStatusOfflineGrace, NodeLeaseStatusExpired:
		return true
	}
	return false
}

// NodeLeaseStore persists device leases. Implementations must scope writes to
// the tenant from context where a tenant column exists.
type NodeLeaseStore interface {
	// UpsertLease inserts or refreshes the lease keyed by node_id. A fresh
	// resume_token rotates the token; an empty token keeps the existing one.
	UpsertLease(ctx context.Context, lease *NodeLease) error
	// GetByResumeToken resolves a lease by its opaque resume token.
	GetByResumeToken(ctx context.Context, token string) (*NodeLease, error)
	// GetByNodeID resolves the current lease for a node.
	GetByNodeID(ctx context.Context, nodeID string) (*NodeLease, error)
	// TouchHeartbeat extends the lease expiry and marks it online.
	TouchHeartbeat(ctx context.Context, nodeID string, ttl time.Duration) error
	// MarkStatus transitions the lease lifecycle status.
	MarkStatus(ctx context.Context, nodeID, status string) error
	// ExpireStale flips online/reconnecting/offline_grace rows whose
	// expires_at has passed to expired. Returns affected row count.
	ExpireStale(ctx context.Context) (int64, error)
}
