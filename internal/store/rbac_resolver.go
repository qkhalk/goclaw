package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
)

// TenantRoleResolver implements permissions.RoleResolver against the
// TenantRoleStore. It resolves a tenant member's custom-role permission rows by
// merging every custom role assigned to the user (deny wins over allow across
// roles). It lives in the store package — not permissions — because store
// imports permissions transitively and importing store from permissions would
// cycle.
//
// Wiring (gateway_setup): construct with the TenantRoleStore and install via
// permissions.PolicyEngine.SetRoleResolver.
type TenantRoleResolver struct {
	roles TenantRoleStore
}

// NewTenantRoleResolver creates a resolver backed by the tenant roles store.
func NewTenantRoleResolver(roles TenantRoleStore) *TenantRoleResolver {
	return &TenantRoleResolver{roles: roles}
}

// ResolveCustomRoles implements permissions.RoleResolver.
//
// tenantID arriving in non-UUID form (master-scope callers, empty tenant)
// yields no custom roles, so the tier fallback applies unchanged.
func (r *TenantRoleResolver) ResolveCustomRoles(ctx context.Context, tenantID string, userID string) ([]permissions.RoleGrant, error) {
	if tenantID == "" || userID == "" {
		return nil, nil
	}
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		// Not a tenant-scoped context — no per-tenant overrides apply.
		return nil, nil
	}

	roleIDs, err := r.roles.ListMemberRoles(ctx, tid, userID)
	if err != nil {
		return nil, err
	}
	if len(roleIDs) == 0 {
		return nil, nil
	}

	// Merge permission rows across all assigned roles. deny wins over allow
	// regardless of which role supplied it.
	merged := make(map[string]permissions.RoleGrant)
	deny := make(map[string]bool)
	for _, roleID := range roleIDs {
		rows, err := r.roles.ListRolePermissions(ctx, roleID)
		if err != nil {
			return nil, err
		}
		for _, rp := range rows {
			if !ValidPermissionEffect(rp.Effect) {
				continue
			}
			key := rp.Permission
			if key == "" {
				continue
			}
			grant := rp.Effect == PermissionEffectAllow
			if grant {
				merged[key] = permissions.RoleGrant{Permission: key, Effect: true}
			} else {
				deny[key] = true
			}
		}
	}
	for key := range deny {
		merged[key] = permissions.RoleGrant{Permission: key, Effect: false}
	}
	if len(merged) == 0 {
		return nil, nil
	}
	out := make([]permissions.RoleGrant, 0, len(merged))
	for _, g := range merged {
		out = append(out, g)
	}
	return out, nil
}