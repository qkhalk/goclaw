package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Tenant policy errors. These are the typed errors handlers translate into
// RESOURCE_EXHAUSTED / FAILED_PRECONDITION responses.
var (
	// ErrTenantPolicyNotFound is returned when no policy row exists for a tenant.
	// Reads that need to know "no policy configured" treat this as "no limits"
	// (a nil policy is equivalent to an all-defaults active policy).
	ErrTenantPolicyNotFound = errors.New("tenant policy not found")
	// ErrTenantLimitReached is a sentinel wrapped by *TenantLimitError so callers
	// can match resource-cap failures with errors.Is.
	ErrTenantLimitReached = errors.New("tenant resource limit reached")
	// ErrTenantSuspended is returned when the tenant policy status is not
	// "active". New connections and run entries are blocked.
	ErrTenantSuspended = errors.New("tenant is suspended")
)

// Policy status vocabulary. A policy with status != active blocks new
// connections and run entries. The default is active (no interruption).
const (
	TenantPolicyStatusActive    = "active"
	TenantPolicyStatusSuspended = "suspended"
)

// ValidTenantPolicyStatus reports whether s is a known policy status.
func ValidTenantPolicyStatus(s string) bool {
	switch s {
	case TenantPolicyStatusActive, TenantPolicyStatusSuspended:
		return true
	}
	return false
}

// IsBuiltinTenantRole reports whether role is one of the fixed tenant roles
// (owner/admin/operator/member/viewer) backed by the tier default in
// permissions.RoleFromTenantRole, as opposed to a per-tenant custom role name.
func IsBuiltinTenantRole(role string) bool {
	switch role {
	case TenantRoleOwner, TenantRoleAdmin, TenantRoleOperator, TenantRoleMember, TenantRoleViewer:
		return true
	}
	return false
}

// TenantPolicy is the typed per-tenant policy row. One row per tenant. A
// NULL max_* value means "no cap". An empty allowed_providers / allowed_models
// list means "no allowlist restriction" (fast path — nothing to check).
type TenantPolicy struct {
	ID               uuid.UUID       `json:"id" db:"id"`
	TenantID         uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Quota            json.RawMessage `json:"quota,omitempty" db:"quota"`
	AllowedProviders []string        `json:"allowed_providers" db:"allowed_providers"`
	AllowedModels    []string        `json:"allowed_models" db:"allowed_models"`
	MaxAgents        *int            `json:"max_agents,omitempty" db:"max_agents"`
	MaxSessions      *int            `json:"max_sessions,omitempty" db:"max_sessions"`
	MaxTeams         *int            `json:"max_teams,omitempty" db:"max_teams"`
	Status           string          `json:"status" db:"status"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// CapName is a stable identifier for a resource cap (matches the JSON field).
type CapName string

const (
	CapAgents   CapName = "max_agents"
	CapSessions CapName = "max_sessions"
	CapTeams    CapName = "max_teams"
)

// TenantLimitError describes one exceeded resource cap. It wraps
// ErrTenantLimitReached so gateway handlers can match with errors.Is.
type TenantLimitError struct {
	Cap      CapName
	Limit    int
	Existing int
}

func (e *TenantLimitError) Error() string {
	return fmt.Sprintf("tenant resource limit reached: %s = %d (existing %d)", e.Cap, e.Limit, e.Existing)
}

func (e *TenantLimitError) Unwrap() error { return ErrTenantLimitReached }

// ProviderAccess reports whether the policy restricts providers and whether the
// requested provider is allowed. When AllowedProviders is empty there is no
// restriction (allowed = true, withAllowlist = false).
func (p *TenantPolicy) ProviderAccess(provider string) (allowed, withAllowlist bool) {
	if len(p.AllowedProviders) == 0 {
		return true, false
	}
	if slices.Contains(p.AllowedProviders, provider) {
		return true, true
	}
	return false, true
}

// ModelAccess reports whether the policy restricts models and whether the
// requested model is allowed. When AllowedModels is empty there is no
// restriction.
func (p *TenantPolicy) ModelAccess(model string) (allowed, withAllowlist bool) {
	if len(p.AllowedModels) == 0 {
		return true, false
	}
	if slices.Contains(p.AllowedModels, model) {
		return true, true
	}
	return false, true
}

// Suspended reports whether the policy blocks new connections/run entries.
func (p *TenantPolicy) Suspended() bool {
	return p.Status != "" && p.Status != TenantPolicyStatusActive
}

// TenantPolicyStore persists one policies row per tenant and enforces the
// resource caps at creation time. All reads fail closed when a tenant ID is
// required but absent from the context.
type TenantPolicyStore interface {
	// GetTenantPolicy returns the policy for tenantID, or ErrTenantPolicyNotFound
	// when none is configured. Nil is never returned without an error.
	GetTenantPolicy(ctx context.Context, tenantID uuid.UUID) (*TenantPolicy, error)
	// UpsertTenantPolicy inserts or replaces the policy for its tenant.
	UpsertTenantPolicy(ctx context.Context, policy *TenantPolicy) error
	// DeleteTenantPolicy removes the policy for tenantID.
	DeleteTenantPolicy(ctx context.Context, tenantID uuid.UUID) error
	// ListTenantPolicies returns all tenant policies (system/owner scope).
	ListTenantPolicies(ctx context.Context) ([]TenantPolicy, error)

	// CheckTenantActive returns ErrTenantSuspended when a policy row exists and
	// its status is not active. No policy row is treated as active (fast path).
	CheckTenantActive(ctx context.Context, tenantID uuid.UUID) error

	// CheckCanCreateAgent/Session/Team enforce the max_* caps. They return nil
	// when no cap is configured or the existing count is below the limit, and a
	// *TenantLimitError when the cap is already reached. The existing counts are
	// scoped to tenantID regardless of the caller's context scope.
	CheckCanCreateAgent(ctx context.Context, tenantID uuid.UUID) error
	CheckCanCreateSession(ctx context.Context, tenantID uuid.UUID) error
	CheckCanCreateTeam(ctx context.Context, tenantID uuid.UUID) error
}

// AgentPolicies is the subset of TenantPolicyStore consumed by RPC method
// handlers and the HTTP chat surface to enforce per-tenant provider/model
// allowlists, resource caps, and suspension at creation and run-entry time.
// It exposes only the checks a handler needs; admin CRUD stays on
// TenantPolicyStore. Both PG and SQLite implementations satisfy it
// structurally, so handlers hold this narrow interface instead of the full
// store (avoids pulling list/upsert/delete into creation hot paths).
type AgentPolicies interface {
	GetTenantPolicy(ctx context.Context, tenantID uuid.UUID) (*TenantPolicy, error)
	CheckTenantActive(ctx context.Context, tenantID uuid.UUID) error
	CheckCanCreateAgent(ctx context.Context, tenantID uuid.UUID) error
	CheckCanCreateSession(ctx context.Context, tenantID uuid.UUID) error
	CheckCanCreateTeam(ctx context.Context, tenantID uuid.UUID) error
}
