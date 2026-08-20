package permissions

import (
	"context"
	"testing"
)

// stubResolver returns a fixed grant set for testing.
type stubResolver struct {
	grants []RoleGrant
	err    error
}

func (s *stubResolver) ResolveCustomRoles(ctx context.Context, tenantID, userID string) ([]RoleGrant, error) {
	return s.grants, s.err
}

func TestMinRoleFor(t *testing.T) {
	cases := []struct {
		resource string
		action   string
		want     Role
		ok       bool
	}{
		{"agent", "create", RoleOperator, true},
		{"agent", "delete", RoleAdmin, true},
		{"channel", "create", RoleAdmin, true},
		{"skill", "publish", RoleOperator, true},
		{"config", "read", RoleViewer, true},
		{"config", "write", RoleAdmin, true},
		{"user", "invite", RoleAdmin, true},
		{"unknown", "create", RoleNone, false},
		{"agent", "wat", RoleNone, false},
	}
	for _, c := range cases {
		got, ok := MinRoleFor(c.resource, c.action)
		if got != c.want || ok != c.ok {
			t.Errorf("MinRoleFor(%q,%q) = (%v,%v), want (%v,%v)", c.resource, c.action, got, ok, c.want, c.ok)
		}
	}
}

func TestEffectiveAccessTierFallback(t *testing.T) {
	pe := NewPolicyEngine([]string{"owner"})
	// No resolver → tier fallback.
	cases := []struct {
		role     string
		resource string
		action   string
		want     bool
	}{
		{"operator", "agent", "create", true},  // agent:create requires operator
		{"viewer", "agent", "create", false},   // viewer too low
		{"admin", "agent", "create", true},     // admin ≥ operator
		{"viewer", "config", "read", true},     // config:read requires viewer
		{"admin", "provider", "create", true},  // provider:create requires admin
		{"operator", "provider", "create", false},
		{"viewer", "unknown", "x", false}, // unknown pair → fail closed
		{"owner", "unknown", "x", false},  // even owner denied on unknown pair
	}
	for _, c := range cases {
		got := pe.EffectiveAccess(context.Background(), "", "", c.role, c.resource, c.action)
		if got != c.want {
			t.Errorf("EffectiveAccess(%q,%q,%q) = %v, want %v", c.role, c.resource, c.action, got, c.want)
		}
	}
}

func TestEffectiveAccessCustomGrantOverridesTier(t *testing.T) {
	// Custom grant lets a viewer manage agents, and denies an operator the
	// channel:create tier default (admin) is lowered to allow via custom grant.
	pe := NewPolicyEngine([]string{"owner"})
	pe.SetRoleResolver(&stubResolver{grants: []RoleGrant{
		{Permission: "agent:create", Effect: true},
		{Permission: "channel:create", Effect: true},
		{Permission: "skill:publish", Effect: false},
	}})

	cases := []struct {
		role     string
		resource string
		action   string
		want     bool
	}{
		{"viewer", "agent", "create", true},    // custom allow raises viewer above tier
		{"viewer", "channel", "create", true},  // custom allow on admin-tier default
		{"operator", "skill", "publish", false}, // deny beats tier allow
		{"admin", "skill", "publish", false},    // deny beats tier allow even for admin
		{"admin", "provider", "create", true},   // untouched pair keeps tier default
	}
	for _, c := range cases {
		got := pe.EffectiveAccess(context.Background(), "t1", "u1", c.role, c.resource, c.action)
		if got != c.want {
			t.Errorf("EffectiveAccess(%q,%q,%q) = %v, want %v", c.role, c.resource, c.action, got, c.want)
		}
	}
}

func TestEffectiveAccessResolverErrorFailsClosed(t *testing.T) {
	pe := NewPolicyEngine([]string{"owner"})
	pe.SetRoleResolver(&stubResolver{err: context.DeadlineExceeded})
	// Resolver failure must not raise access: falls back to tier (fail closed).
	if pe.EffectiveAccess(context.Background(), "t1", "u1", "viewer", "agent", "create") {
		t.Error("resolver error must fail closed to tier default (viewer denied agent:create)")
	}
	if !pe.EffectiveAccess(context.Background(), "t1", "u1", "admin", "agent", "create") {
		t.Error("admin still gets agent:create via tier fallback despite resolver error")
	}
}

func TestEffectivePermissionsMerge(t *testing.T) {
	pe := NewPolicyEngine([]string{"owner"})
	pe.SetRoleResolver(&stubResolver{grants: []RoleGrant{
		{Permission: "agent:create", Effect: true},
		{Permission: "provider:create", Effect: false},
	}})
	perms := pe.EffectivePermissions(context.Background(), "t1", "u1", "operator")

	if perms["agent:create"] != GrantEffectAllow {
		t.Errorf("agent:create = %v, want allow (custom grant)", perms["agent:create"])
	}
	if perms["provider:create"] != GrantEffectDeny {
		t.Errorf("provider:create = %v, want deny (custom deny beats tier)", perms["provider:create"])
	}
	if perms["channel:create"] != GrantEffectDeny {
		t.Errorf("channel:create = %v, want deny (admin-tier default, operator caller)", perms["channel:create"])
	}
	if perms["config:read"] != GrantEffectAllow {
		t.Errorf("config:read = %v, want allow (viewer-tier default)", perms["config:read"])
	}
	// Unknown catalog rows from the resolver are ignored.
	pe.SetRoleResolver(&stubResolver{grants: []RoleGrant{
		{Permission: "totally:madeup", Effect: true},
	}})
	perms = pe.EffectivePermissions(context.Background(), "t1", "u1", "viewer")
	if perms["totally:madeup"] != GrantEffectDeny {
		t.Errorf("unknown catalog row must be ignored (present=%v)", perms["totally:madeup"])
	}
}

func TestEffectivePermissionsNilEngine(t *testing.T) {
	// Nil engine (or nil resolver) still yields tier defaults via effectForRole.
	var pe *PolicyEngine
	perms := pe.EffectivePermissions(context.Background(), "", "", "admin")
	if perms["agent:create"] != GrantEffectAllow {
		t.Errorf("nil engine admin agent:create = %v, want allow", perms["agent:create"])
	}
}