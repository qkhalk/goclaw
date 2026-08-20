package permissions

import "context"

// Resource-action effective permission evaluation (Phase 4 W2 — fine-grained RBAC).
//
// These helpers sit on top of the tier-based policy and the resource:action
// catalog. They deliberately avoid importing internal/store: store depends on
// this package transitively, so importing it here would create a cycle. The
// per-tenant custom-role data is supplied by callers through a RoleResolver
// injected at construction time (see PolicyEngine.WithRoleResolver / SetRoleResolver).
// Unknown resource:action pairs and nil resolvers fail closed (deny).

// GrantEffect is the effective outcome of a resource:action check.
type GrantEffect int

const (
	// GrantEffectDeny means the request is denied.
	GrantEffectDeny GrantEffect = iota
	// GrantEffectAllow means the request is permitted.
	GrantEffectAllow
)

// RoleGrant is one resolved permission entry for a role. A value of
// RoleNone indicates the resolved role is not tied to a catalog permission.
type RoleGrant struct {
	Permission string // resource:action key e.g. "agent:create"
	Effect     bool   // allow (true) or deny (false)
}

// RoleResolver supplies per-tenant custom-role permission data for a user.
// It must be safe for concurrent use. A nil resolver means "no custom roles" —
// the tier fallback applies unchanged (back-compat behavior).
type RoleResolver interface {
	// ResolveCustomRoles returns the permission rows that apply to a user in a
	// tenant, merged across all custom roles the user holds. Rows are returned
	// with resource:action keys; caller side is responsible for validating them
	// against the catalog. Returns nil when the user holds no custom roles.
	ResolveCustomRoles(ctx context.Context, tenantID string, userID string) ([]RoleGrant, error)
}

// RoleResolverFunc adapts a plain function to RoleResolver.
type RoleResolverFunc func(ctx context.Context, tenantID string, userID string) ([]RoleGrant, error)

// ResolveCustomRoles implements RoleResolver.
func (f RoleResolverFunc) ResolveCustomRoles(ctx context.Context, tenantID string, userID string) ([]RoleGrant, error) {
	return f(ctx, tenantID, userID)
}

// SetRoleResolver installs a RoleResolver on the engine. Callers in the gateway
// wiring (cmd/gateway_setup.go) set this once at startup after the store layer
// is available; it stays nil in tests and for master-scope deployments.
func (pe *PolicyEngine) SetRoleResolver(r RoleResolver) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.roleResolver = r
}

// EffectiveAccess resolves a user's access to one catalog resource:action.
//
// Resolution order (fail-closed):
//
//  1. If the user's custom-role rows contain this permission with deny, deny.
//  2. If they contain it with allow, allow (custom grants can raise or lower the
//     tier default, but not override a deny).
//  3. Otherwise fall back to the catalog tier requirement: the user's gateway
//     role must be at least the catalog's DefaultMinRole.
//  4. Unknown resource:action → deny for every role.
func (pe *PolicyEngine) EffectiveAccess(ctx context.Context, tenantID, userID, role string, resource, action string) bool {
	key := CatalogKey(resource, action)
	required, ok := MinRoleFor(resource, action)
	if !ok {
		return false
	}
	g, ok := pe.lookupCustomGrant(ctx, tenantID, userID, key)
	if ok {
		return g
	}
	return HasMinRole(Role(role), required)
}

// EffectivePermissions returns the full permission row set for a tenant+role
// combination, merging builtin tier defaults with any per-tenant custom-role
// server (from the injected resolver). Returned map keys are resource:action.
//
// This is a readonly introspection helper consumed by the roles endpoints; the
// enforcement path uses EffectiveAccess which avoids materializing the full set.
func (pe *PolicyEngine) EffectivePermissions(ctx context.Context, tenantID, userID, role string) map[string]GrantEffect {
	base := make(map[string]GrantEffect)
	for key, perm := range PermissionCatalog {
		base[key] = effectForRole(perm.DefaultMinRole, Role(role))
	}
	if pe == nil {
		return base
	}
	pe.mu.RLock()
	resolver := pe.roleResolver
	pe.mu.RUnlock()
	if resolver == nil {
		return base
	}
	rows, err := resolver.ResolveCustomRoles(ctx, tenantID, userID)
	if err != nil {
		// Resolution failure fails closed: keep only the tier defaults.
		return base
	}
	denySet := make(map[string]bool)
	for _, g := range rows {
		// Only catalog-known permissions participate; unknown rows are ignored.
		if _, ok := PermissionCatalog[g.Permission]; !ok {
			continue
		}
		if g.Effect {
			base[g.Permission] = GrantEffectAllow
		} else {
			denySet[g.Permission] = true // deny wins over any allow
		}
	}
	for p := range denySet {
		base[p] = GrantEffectDeny
	}
	return base
}

// lookupCustomGrant returns the resolved grant for one key if the user's
// custom roles mention it. ok=false means no row (apply tier fallback).
func (pe *PolicyEngine) lookupCustomGrant(ctx context.Context, tenantID, userID, key string) (bool, bool) {
	pe.mu.RLock()
	resolver := pe.roleResolver
	pe.mu.RUnlock()
	if resolver == nil {
		return false, false
	}
	rows, err := resolver.ResolveCustomRoles(ctx, tenantID, userID)
	if err != nil {
		return false, false // fail closed to tier fallback on resolution error
	}
	for _, g := range rows {
		if g.Permission != key {
			continue
		}
		return g.Effect, true
	}
	return false, false
}

// effectForRole maps a tier default vs. the caller's gateway role to a grant.
func effectForRole(defaultMinRole, role Role) GrantEffect {
	if HasMinRole(role, defaultMinRole) {
		return GrantEffectAllow
	}
	return GrantEffectDeny
}