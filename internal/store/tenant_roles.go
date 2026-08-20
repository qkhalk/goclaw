package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// RBAC error sentinels.
var (
	// ErrRoleNotFound is returned when a role does not exist or does not belong
	// to the tenant requested.
	ErrRoleNotFound = errors.New("role not found")
	// ErrRolePermissionDenied covers invalid permission names supplied at writes.
	ErrRolePermissionDenied = errors.New("invalid role permission")
	// ErrBuiltinRoleProtected is returned when mutating a builtin role
	// (builtin roles are seeded per tenant and are not editable/deletable).
	ErrBuiltinRoleProtected = errors.New("builtin role is protected")
)

// Permission effects for role_permissions rows. deny always wins over allow.
const (
	PermissionEffectAllow = "allow"
	PermissionEffectDeny  = "deny"
)

// ValidPermissionEffect reports whether e is a known role_permissions effect.
func ValidPermissionEffect(e string) bool {
	return e == PermissionEffectAllow || e == PermissionEffectDeny
}

// TenantRole is one row in the per-tenant `roles` table. Builtin roles are
// seeded once per tenant at first policy access and resolve to the existing
// tier defaults via permissions.RoleFromTenantRole; custom roles carry their
// own resource:action permission rows.
type TenantRole struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Description *string   `json:"description,omitempty" db:"description"`
	Builtin     bool      `json:"builtin" db:"builtin"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// RolePermission is one grant/deny override in `role_permissions`.
type RolePermission struct {
	ID         uuid.UUID `json:"id" db:"id"`
	RoleID     uuid.UUID `json:"role_id" db:"role_id"`
	Permission string    `json:"permission" db:"permission"`
	Effect     string    `json:"effect" db:"effect"`
}

// MemberRoleAssignment links a tenant member to an assigned custom role.
// A user may hold multiple custom roles; the effective permission set is the
// union, with deny winning over allow regardless of source role.
type MemberRoleAssignment struct {
	UserID string    `json:"user_id" db:"user_id"`
	RoleID uuid.UUID `json:"role_id" db:"role_id"`
}

// TenantRoleStore persists per-tenant custom roles and their permissions, and
// tracks which tenant members hold which custom roles. All reads and writes
// are tenant-scoped and fail closed when a tenant is required but absent.
type TenantRoleStore interface {
	// ListRoles returns all roles for the context tenant.
	ListRoles(ctx context.Context) ([]TenantRole, error)
	// GetRole returns one role by id, scoped to the context tenant.
	GetRole(ctx context.Context, id uuid.UUID) (*TenantRole, error)
	// CreateRole inserts a custom role for the context tenant and returns the
	// created row. Name must be non-empty and a builtin role name is rejected.
	CreateRole(ctx context.Context, role *TenantRole) error
	// UpdateRole patches a custom role (name/description), scoped to the context
	// tenant. Builtin roles cannot be updated.
	UpdateRole(ctx context.Context, id uuid.UUID, updates map[string]any) error
	// DeleteRole removes a custom role (cascades to its role_permissions and
	// member assignments). Builtin roles cannot be deleted.
	DeleteRole(ctx context.Context, id uuid.UUID) error

	// SetRolePermissions replaces the permission rows for one role, scoped to
	// the context tenant. Runs in a transaction.
	SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissions []RolePermission) error
	// ListRolePermissions returns the permission rows for one role, scoped to
	// the context tenant.
	ListRolePermissions(ctx context.Context, roleID uuid.UUID) ([]RolePermission, error)

	// AssignRoleToMember grants a custom role to a tenant member.
	AssignRoleToMember(ctx context.Context, tenantID uuid.UUID, userID string, roleID uuid.UUID) error
	// RevokeRoleFromMember removes a custom role from a tenant member.
	RevokeRoleFromMember(ctx context.Context, tenantID uuid.UUID, userID string, roleID uuid.UUID) error
	// ListMemberRoles returns all custom role IDs assigned to a tenant member.
	ListMemberRoles(ctx context.Context, tenantID uuid.UUID, userID string) ([]uuid.UUID, error)
	// ListTenantMemberRoles returns all custom-role assignments for the tenant
	// (used to resolve effective permissions for a call).
	ListTenantMemberRoles(ctx context.Context, tenantID uuid.UUID) ([]MemberRoleAssignment, error)
}