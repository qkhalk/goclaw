package store

import (
	"context"
	"fmt"
	"time"
)

// Node trust lifecycle (inheritance plan Phase 2). Nodes are UNTRUSTED by
// default (plan Rule 5): nothing executes on a node unless its trust is
// `trusted`. Revocation is terminal — a revoked node needs a freshly created
// key to rejoin the registry.
const (
	NodeTrustPending = "pending"
	NodeTrustTrusted = "trusted"
	NodeTrustRevoked = "revoked"
)

// Well-known node capabilities advertised at registration time.
const (
	NodeCapabilityExec    = "exec"
	NodeCapabilityFS      = "fs"
	NodeCapabilityBrowser = "browser"
)

// ValidNodeTrust reports whether s is a known trust state.
func ValidNodeTrust(s string) bool {
	switch s {
	case NodeTrustPending, NodeTrustTrusted, NodeTrustRevoked:
		return true
	}
	return false
}

// ValidNodeCapability reports whether s is a known capability name.
func ValidNodeCapability(s string) bool {
	switch s {
	case NodeCapabilityExec, NodeCapabilityFS, NodeCapabilityBrowser:
		return true
	}
	return false
}

// Node is one registered compute node (inheritance plan Phase 2). Distinct
// from NodeLease (UI-tab presence, Phase 1) and from pairing (channel sender
// trust): a node is a daemon-run execution target that authenticates with its
// own bearer key. NodeKeyHash is the SHA-256 hex of that bearer key; the
// plaintext is revealed exactly once at key creation and never stored.
type Node struct {
	ID           string     `json:"id"`
	TenantID     *string    `json:"tenantId,omitempty"`
	Name         string     `json:"name"`
	NodeKeyHash  string     `json:"-"`
	Platform     string     `json:"platform"` // "os/arch", filled by the daemon at registration
	Capabilities []string   `json:"capabilities"`
	Trust        string     `json:"trust"`
	LastSeenAt   *time.Time `json:"lastSeenAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	RevokedAt    *time.Time `json:"revokedAt,omitempty"`
}

// HasCapability reports whether the node advertised the given capability.
func (n *Node) HasCapability(cap string) bool {
	for _, c := range n.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// NodeStore persists the compute-node registry. Implementations must scope
// reads/writes to the tenant from context where a tenant column exists;
// GetByKeyHash is the single deliberate exception (daemon identity resolution
// via a globally unique 256-bit key hash happens before any tenant context is
// known to the daemon-side row).
type NodeStore interface {
	// Create inserts a new node row (trust defaults to pending). The key hash
	// must be set by the caller; the ID and timestamps are defaulted when
	// zero-valued.
	Create(ctx context.Context, n *Node) error
	// GetByID resolves one node by UUID. Tenant-scoped via context.
	GetByID(ctx context.Context, id string) (*Node, error)
	// GetByKeyHash resolves a node by the SHA-256 hex of its bearer key.
	// Global lookup (no tenant scope) — see interface comment.
	GetByKeyHash(ctx context.Context, hash string) (*Node, error)
	// List returns the tenant's nodes ordered by created_at DESC.
	// Tenant-scoped via context.
	List(ctx context.Context) ([]*Node, error)
	// UpdateRegistration refreshes the daemon-advertised fields (name,
	// platform, capabilities) and stamps last_seen_at. Called on every
	// (re)registration, so it doubles as the liveness heartbeat.
	UpdateRegistration(ctx context.Context, id, name, platform string, capabilities []string) error
	// TouchSeen stamps last_seen_at + updated_at without changing advertised
	// fields.
	TouchSeen(ctx context.Context, id string) error
	// SetTrust transitions the trust state. pending→trusted and
	// trusted→pending are allowed; revoked is terminal (the store rejects
	// leaving it).
	SetTrust(ctx context.Context, id, trust string) error
	// Revoke sets trust=revoked + revoked_at. Idempotent for already-revoked
	// nodes.
	Revoke(ctx context.Context, id string) error
}

// ValidateNodeTrustTransition reports whether from→to is a legal transition.
func ValidateNodeTrustTransition(from, to string) error {
	if !ValidNodeTrust(to) {
		return fmt.Errorf("invalid node trust state %q", to)
	}
	if from == NodeTrustRevoked {
		return fmt.Errorf("node trust is revoked (terminal); create a new key instead")
	}
	if from == to {
		return nil
	}
	return nil
}
