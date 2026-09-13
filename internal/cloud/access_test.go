package cloud

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeBindingStore struct {
	bindings []store.CloudBinding
}

func (f *fakeBindingStore) ListBindings(ctx context.Context) ([]store.CloudBinding, error) {
	return f.bindings, nil
}

func (f *fakeBindingStore) UpsertBinding(ctx context.Context, b *store.CloudBinding) error {
	f.bindings = append(f.bindings, *b)
	return nil
}

func (f *fakeBindingStore) DeleteBinding(ctx context.Context, id string) error { return nil }

type fakeMultiAccountStore struct {
	fakeAccountStore
	own    []store.CloudAccount
	shared []store.CloudAccount
}

func (f *fakeMultiAccountStore) List(ctx context.Context) ([]store.CloudAccount, error) {
	return f.own, nil
}

func (f *fakeMultiAccountStore) ListShared(ctx context.Context) ([]store.CloudAccount, error) {
	return f.shared, nil
}

func storageUsable(a *store.CloudAccount) bool { return a.Provider == "onedrive" && a.Status != "revoked" }

// TestResolveAccountBindingPriority covers explicit pick, group → user →
// tenant-default binding order and the legacy own-account fallback.
func TestResolveAccountBindingPriority(t *testing.T) {
	own := &store.CloudAccount{ID: "own-1", Provider: "onedrive", Email: "me@x.test", Status: "active"}
	shared := &store.CloudAccount{ID: "shared-1", Provider: "onedrive", Email: "corp@x.test", Status: "active", Shared: true}
	fake := &fakeMultiAccountStore{own: []store.CloudAccount{*own}, shared: []store.CloudAccount{*shared}}
	bindings := &fakeBindingStore{}
	m := NewManager(CloudProviderConfig{}, fake, "k")
	m.SetBindingStore(bindings)

	groupCtx := store.WithChannelContextScope(
		store.WithUserID(store.WithTenantID(context.Background(), uuid.Must(uuid.NewV7())), "user-9"),
		store.ChannelContextScope{ChannelInstanceName: "telegram", ScopeType: store.ChannelScopeTypeGroup, ScopeKey: "-100"})

	// No bindings → fallback to own account.
	acct, err := m.ResolveAccount(groupCtx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "own-1" {
		t.Fatalf("fallback = %+v err=%v, want own-1", acct, err)
	}

	// Tenant default binding → shared corp account.
	bindings.bindings = []store.CloudBinding{{
		ScopeType: store.CloudBindingScopeTenant, Provider: "onedrive", AccountID: "shared-1",
		Enabled: true, Priority: 100,
	}}
	acct, err = m.ResolveAccount(groupCtx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "shared-1" {
		t.Fatalf("tenant default = %+v err=%v, want shared-1", acct, err)
	}

	// User binding beats tenant default.
	bindings.bindings = append(bindings.bindings, store.CloudBinding{
		ScopeType: store.CloudBindingScopeUser, ScopeKey: "user-9", Provider: "onedrive", AccountID: "own-1",
		Enabled: true, Priority: 100,
	})
	acct, err = m.ResolveAccount(groupCtx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "own-1" {
		t.Fatalf("user binding = %+v err=%v, want own-1", acct, err)
	}

	// Group binding beats user binding: bind the shared account to the group.
	bindings.bindings = append(bindings.bindings, store.CloudBinding{
		ScopeType: store.CloudBindingScopeGroup, ScopeKey: "-100", Provider: "onedrive", AccountID: "shared-1",
		Enabled: true, Priority: 100,
	})
	acct, err = m.ResolveAccount(groupCtx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "shared-1" {
		t.Fatalf("group binding = %+v err=%v, want shared-1", acct, err)
	}

	// Explicit email pick wins over everything (also matches shared accounts).
	acct, err = m.ResolveAccount(groupCtx, "CORP@x.test", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "shared-1" {
		t.Fatalf("explicit pick = %+v err=%v, want shared-1", acct, err)
	}

	// Revoked binding target falls through to the user binding.
	revoked := &store.CloudAccount{ID: "revoked-1", Provider: "onedrive", Email: "dead@x.test", Status: "revoked"}
	fake.own = append(fake.own, *revoked)
	bindings.bindings = append([]store.CloudBinding{{
		ScopeType: store.CloudBindingScopeGroup, ScopeKey: "-100", Provider: "onedrive", AccountID: "revoked-1",
		Enabled: true, Priority: 100,
	}}, bindings.bindings[:2]...) // group→revoked, user→own-1, tenant→shared-1
	acct, err = m.ResolveAccount(groupCtx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "own-1" {
		t.Fatalf("revoked fallthrough = %+v err=%v, want own-1", acct, err)
	}
}

// TestResolveAccountSkipsDisabledBindings covers the enabled flag: a disabled
// rule must be ignored entirely — the next scope tier (and fallback) applies.
func TestResolveAccountSkipsDisabledBindings(t *testing.T) {
	own := &store.CloudAccount{ID: "own-1", Provider: "onedrive", Email: "me@x.test", Status: "active"}
	shared := &store.CloudAccount{ID: "shared-1", Provider: "onedrive", Email: "corp@x.test", Status: "active", Shared: true}
	fake := &fakeMultiAccountStore{own: []store.CloudAccount{*own}, shared: []store.CloudAccount{*shared}}
	bindings := &fakeBindingStore{}
	m := NewManager(CloudProviderConfig{}, fake, "k")
	m.SetBindingStore(bindings)

	ctx := store.WithUserID(store.WithTenantID(context.Background(), uuid.Must(uuid.NewV7())), "user-9")

	// Disabled user binding + enabled tenant binding → tenant default wins
	// (the user tier is skipped, not resolved through the disabled rule).
	bindings.bindings = []store.CloudBinding{
		{ScopeType: store.CloudBindingScopeUser, ScopeKey: "user-9", Provider: "onedrive", AccountID: "shared-1", Enabled: false, Priority: 100},
		{ScopeType: store.CloudBindingScopeTenant, Provider: "onedrive", AccountID: "shared-1", Enabled: true, Priority: 100},
	}
	acct, err := m.ResolveAccount(ctx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "shared-1" {
		t.Fatalf("tenant default after disabled user rule = %+v err=%v, want shared-1", acct, err)
	}

	// All rules disabled → legacy own-account fallback.
	bindings.bindings[1].Enabled = false
	acct, err = m.ResolveAccount(ctx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "own-1" {
		t.Fatalf("fallback with all rules disabled = %+v err=%v, want own-1", acct, err)
	}
}

// TestResolveAccountPriorityWithinTier: among multiple enabled rules of the
// same scope tier, the lowest priority number wins (ties → oldest first).
// The real store enforces one rule per (tier, key, provider); the fake can
// exercise the selector directly.
func TestResolveAccountPriorityWithinTier(t *testing.T) {
	own := &store.CloudAccount{ID: "own-1", Provider: "onedrive", Email: "me@x.test", Status: "active"}
	shared := &store.CloudAccount{ID: "shared-1", Provider: "onedrive", Email: "corp@x.test", Status: "active", Shared: true}
	fake := &fakeMultiAccountStore{own: []store.CloudAccount{*own}, shared: []store.CloudAccount{*shared}}
	bindings := &fakeBindingStore{}
	m := NewManager(CloudProviderConfig{}, fake, "k")
	m.SetBindingStore(bindings)

	ctx := store.WithUserID(store.WithTenantID(context.Background(), uuid.Must(uuid.NewV7())), "user-9")

	// Priority 50 beats priority 200 within the user tier, regardless of
	// listing order.
	bindings.bindings = []store.CloudBinding{
		{ScopeType: store.CloudBindingScopeUser, ScopeKey: "user-9", Provider: "onedrive", AccountID: "shared-1", Enabled: true, Priority: 200},
		{ScopeType: store.CloudBindingScopeUser, ScopeKey: "user-9", Provider: "onedrive", AccountID: "own-1", Enabled: true, Priority: 50},
	}
	acct, err := m.ResolveAccount(ctx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "own-1" {
		t.Fatalf("priority 50 = %+v err=%v, want own-1", acct, err)
	}

	// Equal priority → oldest (earliest created_at) wins.
	base := time.Now().Add(-time.Hour)
	bindings.bindings = []store.CloudBinding{
		{ScopeType: store.CloudBindingScopeUser, ScopeKey: "user-9", Provider: "onedrive", AccountID: "shared-1", Enabled: true, Priority: 100, CreatedAt: base},
		{ScopeType: store.CloudBindingScopeUser, ScopeKey: "user-9", Provider: "onedrive", AccountID: "own-1", Enabled: true, Priority: 100, CreatedAt: base.Add(time.Minute)},
	}
	acct, err = m.ResolveAccount(ctx, "", []string{"onedrive"}, storageUsable)
	if err != nil || acct.ID != "shared-1" {
		t.Fatalf("tie → oldest = %+v err=%v, want shared-1", acct, err)
	}
}
