//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestSQLiteCloudSharedAndBindings covers the tenant-wide share flag and the
// per-scope binding CRUD on the SQLite store.
func TestSQLiteCloudSharedAndBindings(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	accounts := NewSQLiteCloudAccountStore(db, "")
	tenantA := uuid.Must(uuid.NewV7())
	tenantB := uuid.Must(uuid.NewV7())
	for _, tc := range []struct {
		id   uuid.UUID
		slug string
	}{{tenantA, "tenant-a"}, {tenantB, "tenant-b"}} {
		if _, err := db.Exec(
			`INSERT INTO tenants (id, name, slug, status) VALUES (?, ?, ?, 'active')`,
			tc.id.String(), tc.slug, tc.slug,
		); err != nil {
			t.Fatalf("insert tenant: %v", err)
		}
	}

	ownerCtx := store.WithUserID(store.WithTenantID(context.Background(), tenantA), "owner")
	otherCtx := store.WithUserID(store.WithTenantID(context.Background(), tenantA), "member")
	foreignCtx := store.WithUserID(store.WithTenantID(context.Background(), tenantB), "stranger")

	acct := &store.CloudAccount{Provider: "onedrive", Email: "drive@corp.test", DisplayName: "Corp Drive"}
	if err := accounts.Upsert(ownerCtx, acct); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Not shared yet: ListShared empty; other member cannot see it.
	if got, _ := accounts.ListShared(ownerCtx); len(got) != 0 {
		t.Fatalf("ListShared before share = %d, want 0", len(got))
	}
	if _, err := accounts.Get(otherCtx, acct.ID); err == nil {
		t.Fatal("member sees a private account — scope leak")
	}

	// Share it: visible to the whole tenant (any member), not other tenants.
	if err := accounts.SetShared(ownerCtx, acct.ID, true); err != nil {
		t.Fatalf("SetShared: %v", err)
	}
	shared, err := accounts.ListShared(otherCtx)
	if err != nil || len(shared) != 1 || !shared[0].Shared || shared[0].Email != acct.Email {
		t.Fatalf("member ListShared = %+v err=%v, want the corp drive", shared, err)
	}
	if got, _ := accounts.ListShared(foreignCtx); len(got) != 0 {
		t.Fatalf("foreign tenant sees shared account — tenant leak")
	}

	// Toggle back off.
	if err := accounts.SetShared(ownerCtx, acct.ID, false); err != nil {
		t.Fatalf("SetShared(false): %v", err)
	}
	if got, _ := accounts.ListShared(otherCtx); len(got) != 0 {
		t.Fatalf("ListShared after unshare = %d, want 0", len(got))
	}

	// Bindings: upsert by (tenant, scope, key, provider), list, delete.
	binding := &store.CloudBinding{
		ScopeType: store.CloudBindingScopeGroup,
		ScopeKey:  "-100123",
		Provider:  "onedrive",
		AccountID: acct.ID,
		CreatedBy: "owner",
	}
	if err := accounts.UpsertBinding(ownerCtx, binding); err != nil {
		t.Fatalf("UpsertBinding: %v", err)
	}
	if got, _ := accounts.ListBindings(foreignCtx); len(got) != 0 {
		t.Fatalf("foreign tenant sees bindings — tenant leak")
	}
	bindings, err := accounts.ListBindings(ownerCtx)
	if err != nil || len(bindings) != 1 || bindings[0].AccountID != acct.ID {
		t.Fatalf("ListBindings = %+v err=%v", bindings, err)
	}

	// Re-assign the same scope: upsert replaces, no duplicate row.
	binding.AccountID = ""
	binding2 := &store.CloudBinding{
		ScopeType: store.CloudBindingScopeGroup,
		ScopeKey:  "-100123",
		Provider:  "onedrive",
		AccountID: acct.ID,
		CreatedBy: "owner2",
	}
	_ = binding
	if err := accounts.UpsertBinding(ownerCtx, binding2); err != nil {
		t.Fatalf("UpsertBinding #2: %v", err)
	}
	if bindings, _ = accounts.ListBindings(ownerCtx); len(bindings) != 1 || bindings[0].CreatedBy != "owner2" {
		t.Fatalf("re-binding did not replace: %+v", bindings)
	}

	if err := accounts.DeleteBinding(ownerCtx, bindings[0].ID); err != nil {
		t.Fatalf("DeleteBinding: %v", err)
	}
	if got, _ := accounts.ListBindings(ownerCtx); len(got) != 0 {
		t.Fatalf("bindings after delete = %d, want 0", len(got))
	}
}
