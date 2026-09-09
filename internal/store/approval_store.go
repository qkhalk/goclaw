package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrApprovalAlreadyResolved is returned by Resolve when the target approval
// request is already in a terminal state (approved/denied/expired) and cannot
// be resolved a second time.
var ErrApprovalAlreadyResolved = errors.New("approval already resolved")

// Approval status vocabulary for persisted approval_requests rows.
const (
	// ApprovalStatusPending is the initial state written when an approval
	// request is created. A pending row is resolvable until expired_at.
	ApprovalStatusPending = "pending"
	// ApprovalStatusApproved marks a request resolved by allow-once or
	// allow-always. decision distinguishes the two allow flavors.
	ApprovalStatusApproved = "approved"
	// ApprovalStatusDenied marks a request explicitly denied by an operator, or
	// timed out by the in-memory manager (decision = "deny", status = "denied").
	ApprovalStatusDenied = "denied"
	// ApprovalStatusExpired marks a row whose in-memory timeout fired and whose
	// MarkExpired was called (status = "expired", no decision).
	ApprovalStatusExpired = "expired"
)

// ApprovalDecision is the stored decision string on a resolved row. Mirrors the
// in-memory decisions in internal/tools/exec_approval.go.
const (
	ApprovalDecisionAllowOnce      = "allow-once"
	ApprovalDecisionAllowAlways    = "allow-always"
	ApprovalDecisionAllowForSession = "allow-for-session"
	ApprovalDecisionDeny           = "deny"
)

// ApprovableEntity identity semantics: requester_id/decided_by are UUIDs when
// the actor is a tenant user or agent; requester_type records the actor kind
// (e.g. "agent", "user", "system") for later filtering.

// ApprovalRequest is a persisted command-execution approval request row.
type ApprovalRequest struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	TenantID       uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	AgentID        *uuid.UUID      `json:"agent_id,omitempty" db:"agent_id"`
	RequesterID    *uuid.UUID      `json:"requester_id,omitempty" db:"requester_id"`
	RequesterType  string          `json:"requester_type,omitempty" db:"requester_type"`
	ActionType     string          `json:"action_type" db:"action_type"`
	Payload        json.RawMessage `json:"payload,omitempty" db:"payload"`
	Command        string          `json:"command,omitempty" db:"command"`
	Status         string          `json:"status" db:"status"`
	Decision       string          `json:"decision,omitempty" db:"decision"`
	DecidedBy      *uuid.UUID      `json:"decided_by,omitempty" db:"decided_by"`
	AllowOnce      bool            `json:"allow_once,omitempty" db:"allow_once"`
	AllowAlways    bool            `json:"allow_always,omitempty" db:"allow_always"`
	// SessionKey is the session the request originated from. Populated for
	// allow-for-session grants and for display in the approval queue.
	SessionKey string `json:"session_key,omitempty" db:"session_key"`
	// ArgsDigest is the canonical sha256 of (tool name, tool arguments). A
	// grant keyed by this digest lets a retried/resumed identical call pass
	// without a second prompt (allow-once semantics for tool-class approvals).
	ArgsDigest string `json:"args_digest,omitempty" db:"args_digest"`
	// GrantExpiresAt is the expiry timestamp of the granted scope (written on
	// resolve). Distinct from ExpiredAt, which bounds how long the REQUEST
	// itself stays resolvable.
	GrantExpiresAt *time.Time `json:"grant_expires_at,omitempty" db:"grant_expires_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	DecidedAt      *time.Time `json:"decided_at,omitempty" db:"decided_at"`
	ExpiredAt      *time.Time `json:"expired_at,omitempty" db:"expired_at"`
	TimeoutSeconds int        `json:"timeout_seconds" db:"timeout_seconds"`
}

// ApprovalGrant describes the scope granted when an approval request is
// approved. Exactly one of the Allow* flags should be set; ExpiresAt bounds
// the grant's lifetime (nil = no expiry).
type ApprovalGrant struct {
	AllowOnce       bool
	AllowAlways     bool
	AllowForSession bool
	// SessionKey scopes an allow-for-session grant to one session.
	SessionKey string
	// ExpiresAt is the grant expiry timestamp (nil = never expires).
	ExpiresAt *time.Time
}

// ApprovalListOpts scopes an approval history read. Reads are tenant-scoped via
// context; Status narrows the result set. Limit/Offset default to 50/0.
type ApprovalListOpts struct {
	Status string
	Limit  int
	Offset int
}

// ApprovalStore persists command-execution approval requests. All reads and
// writes are scoped to the context tenant and fail closed when a tenant is
// required but absent.
type ApprovalStore interface {
	// CreateRequest inserts a new approval request, assigning an ID, CreatedAt,
	// and the tenant when left empty. Status defaults to pending.
	CreateRequest(ctx context.Context, req *ApprovalRequest) error
	// ListPending returns unreasoned (pending) requests for the context tenant,
	// oldest first. Responses where status IN (pending, expired).
	ListPending(ctx context.Context, tenantID uuid.UUID) ([]ApprovalRequest, error)
	// ListPendingAll returns pending requests across ALL tenants, oldest first.
	// Cross-tenant by design: used at gateway startup to reinstate durable
	// waiters-without-waiter after a restart. Callers must treat the rows as
	// tenant-scoped state and carry each row's TenantID forward.
	ListPendingAll(ctx context.Context) ([]ApprovalRequest, error)
	// ResolveWithScope marks a pending request resolved with the granted scope.
	// Returns nil on success; ErrApprovalAlreadyResolved when the row was
	// already in a terminal state. Only the owning tenant can resolve.
	ResolveWithScope(ctx context.Context, id uuid.UUID, decision string, decidedBy *uuid.UUID, grant ApprovalGrant) error
	// GetByID returns one request by id, scoped to the context tenant. Returns
	// nil, nil when the row does not exist or belongs to another tenant.
	GetByID(ctx context.Context, id uuid.UUID) (*ApprovalRequest, error)
	// MarkExpired marks a pending request expired (status = expired). Only the
	// owning tenant can transition the row; already-resolved rows are left alone.
	MarkExpired(ctx context.Context, id uuid.UUID) error
	// ListHistory returns resolved requests for the context tenant, newest
	// first, optionally filtered by status.
	ListHistory(ctx context.Context, tenantID uuid.UUID, opts ApprovalListOpts) ([]ApprovalRequest, error)
}