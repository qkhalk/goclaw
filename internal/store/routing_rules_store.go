package store

import (
	"context"
	"errors"
	"time"
)

// routing_rules (inheritance plan Phase 4): tenant-scoped inbound routing
// rules evaluated between config-binding peer matches and channel matches.
//
// Match semantics: every set field must equal the inbound message's value
// (nil/absent = wildcard). `priority` is ascending — the LOWEST number has
// the highest precedence; the first matching enabled rule wins.
//
// JSON shape (WS wire + JSONB blob) is camelCase, consistent with all other
// WS method params (agents, teams, sessions use camelCase params).

// RoutingRuleMatch constrains which inbound messages a rule applies to.
// A nil field is a wildcard; every set field must match.
type RoutingRuleMatch struct {
	Channel   *string `json:"channel,omitempty"`   // channel instance name (same values as config bindings)
	AccountID *string `json:"accountId,omitempty"` // reserved: inbound messages carry no account context yet; rules setting it never match
	PeerKind  *string `json:"peerKind,omitempty"`  // "direct" | "group"
	PeerID    *string `json:"peerId,omitempty"`    // chat/channel ID
	GuildID   *string `json:"guildId,omitempty"`   // Discord guild (msg metadata)
}

// RoutingRule is one stored routing rule.
type RoutingRule struct {
	ID            string           `json:"id"`
	TenantID      string           `json:"tenantId"`
	Priority      int              `json:"priority"`
	Match         RoutingRuleMatch `json:"match"`
	TargetAgentID string           `json:"targetAgentId"`
	Enabled       bool             `json:"enabled"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`

	// TargetAgentKey is resolved via JOIN with agents for the inbound
	// resolution path (which routes by agent key). Populated by ListRules;
	// not part of the wire shape.
	TargetAgentKey string `json:"-"`
}

// ErrRoutingRuleNotFound is returned by DeleteRule when the rule does not
// exist in the tenant scope.
var ErrRoutingRuleNotFound = errors.New("routing rule not found")

// RoutingRulesStore persists per-tenant inbound routing rules.
// Implementations must always scope reads and writes to the tenant from
// context/parameters — rules never cross tenants.
type RoutingRulesStore interface {
	// ListRules returns the tenant's rules ordered by priority ASC,
	// created_at ASC (evaluation order: first match wins).
	ListRules(ctx context.Context, tenantID string) ([]*RoutingRule, error)
	// UpsertRule inserts a rule (ID defaulted when empty) or updates the
	// existing row when ID belongs to the same tenant. TenantID must be set.
	UpsertRule(ctx context.Context, rule *RoutingRule) error
	// DeleteRule removes one rule scoped to the tenant. Returns
	// ErrRoutingRuleNotFound when no row matched.
	DeleteRule(ctx context.Context, tenantID, id string) error
}
