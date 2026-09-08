//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestSQLiteMemoryFabricSearchMultiValueWhitelist pins the union semantics of
// multi-value Scopes/Kinds whitelists. The original implementation AND-chained
// the values ("scope = ? AND scope = ?"), a contradiction that matched
// nothing; the eval case memory scope-whitelist-multi-value pins the same
// behavior against PostgreSQL.
func TestSQLiteMemoryFabricSearchMultiValueWhitelist(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	tenantID := uuid.New()
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES (?, 'T', ?, 'active')`,
		tenantID.String(), "t"+tenantID.String()[:8]); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	s := NewSQLiteMemoryFabricStore(db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	user := "whitelist-user"
	write := func(scope, content string) *store.Memory {
		m := &store.Memory{
			UserID:     &user,
			Scope:      scope,
			Kind:       store.MemoryKindFact,
			Content:    content,
			SourceType: "manual",
		}
		if err := s.WriteMemory(ctx, m); err != nil {
			t.Fatalf("WriteMemory(%s): %v", scope, err)
		}
		return m
	}
	write(store.MemoryScopeUser, "ring-user note")
	global := &store.Memory{
		Scope:      store.MemoryScopeGlobal,
		Kind:       store.MemoryKindFact,
		Content:    "ring-global note",
		SourceType: "manual",
	}
	if err := s.WriteMemory(ctx, global); err != nil {
		t.Fatalf("WriteMemory(global): %v", err)
	}
	write(store.MemoryScopeSession, "ring-session note") // session rows carry no session_key here

	results, err := s.SearchMemories(ctx, store.MemoryQuery{
		UserID: user,
		Scopes: []string{store.MemoryScopeUser, store.MemoryScopeGlobal},
	})
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	got := map[string]bool{}
	for _, r := range results {
		got[r.Content] = true
	}
	if !got["ring-user note"] || !got["ring-global note"] {
		t.Fatalf("multi-scope whitelist must be a union, got %v", got)
	}
	if len(results) != 2 {
		t.Fatalf("want exactly user+global rows, got %d: %v", len(results), got)
	}

	// Multi-kind whitelist is a union too: seed a preference in the same
	// user ring and require both kinds to come back.
	if err := s.WriteMemory(ctx, &store.Memory{
		UserID:     &user,
		Scope:      store.MemoryScopeUser,
		Kind:       store.MemoryKindPreference,
		Content:    "ring-preference note",
		SourceType: "manual",
	}); err != nil {
		t.Fatalf("WriteMemory(preference): %v", err)
	}
	results, err = s.SearchMemories(ctx, store.MemoryQuery{
		UserID: user,
		Kinds:  []string{store.MemoryKindFact, store.MemoryKindPreference},
	})
	if err != nil {
		t.Fatalf("SearchMemories kinds: %v", err)
	}
	kinds := map[string]bool{}
	for _, r := range results {
		kinds[r.Kind] = true
	}
	if !kinds[store.MemoryKindFact] || !kinds[store.MemoryKindPreference] {
		t.Fatalf("multi-kind whitelist must be a union, got kinds %v", kinds)
	}

	// All-invalid whitelist values apply no filter (single-value behavior).
	if _, err := s.SearchMemories(ctx, store.MemoryQuery{UserID: user, Scopes: []string{"bogus"}}); err != nil {
		t.Fatalf("SearchMemories all-invalid scopes: %v", err)
	}
}

// TestSQLiteMemoryFabricUserIsolation pins the user identity gate: bob never
// sees alice's user-scoped rows.
func TestSQLiteMemoryFabricUserIsolation(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	tenantID := uuid.New()
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES (?, 'T', ?, 'active')`,
		tenantID.String(), "t"+tenantID.String()[:8]); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	s := NewSQLiteMemoryFabricStore(db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	alice := "alice-u"
	if err := s.WriteMemory(ctx, &store.Memory{
		UserID:     &alice,
		Scope:      store.MemoryScopeUser,
		Kind:       store.MemoryKindPreference,
		Content:    "alice-secret",
		SourceType: "manual",
	}); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	bob := "bob-u"
	results, err := s.SearchMemories(ctx, store.MemoryQuery{UserID: bob})
	if err != nil {
		t.Fatalf("SearchMemories as bob: %v", err)
	}
	for _, r := range results {
		if r.Content == "alice-secret" {
			t.Fatal("bob must not see alice's user-scoped memory")
		}
	}
	results, err = s.SearchMemories(ctx, store.MemoryQuery{UserID: alice})
	if err != nil {
		t.Fatalf("SearchMemories as alice: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Content == "alice-secret" {
			found = true
		}
	}
	if !found {
		t.Fatal("alice must see her own memory (positive control)")
	}
}
