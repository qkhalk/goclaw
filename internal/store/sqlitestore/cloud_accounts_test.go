//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func newCloudTestStore(t *testing.T) (*SQLiteCloudAccountStore, context.Context) {
	t.Helper()
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	s := NewSQLiteCloudAccountStore(db, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	tenant := uuid.New()
	seedCloudTenant(t, db, tenant)
	ctx := store.WithUserID(store.WithTenantID(context.Background(), tenant), "user-1")
	return s, ctx
}

// cloud_accounts has an FK to tenants — seed a row so the insert holds.
func seedCloudTenant(t *testing.T, db *sql.DB, tenantID uuid.UUID) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO tenants (id, name, slug, status) VALUES (?, 'T', ?, 'active')`,
		tenantID.String(), "t-"+tenantID.String()[:8])
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
}

func TestCloudAccountsUpsertGetList(t *testing.T) {
	s, ctx := newCloudTestStore(t)

	acct := &store.CloudAccount{
		Provider:     "google",
		Email:        "me@gmail.com",
		DisplayName:  "Me",
		AccessToken:  "secret-access",
		RefreshToken: "secret-refresh",
		Status:       "active",
	}
	if err := s.Upsert(ctx, acct); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if acct.ID == "" {
		t.Fatal("Upsert did not assign an ID")
	}

	// Encrypted at rest: the raw row must not contain plaintext tokens.
	var accessEnc string
	if err := s.db.QueryRow(`SELECT access_token FROM cloud_accounts WHERE id=?`, acct.ID).Scan(&accessEnc); err != nil {
		t.Fatalf("raw query: %v", err)
	}
	if !strings.HasPrefix(accessEnc, "aes-gcm:") || strings.Contains(accessEnc, "secret-access") {
		t.Fatalf("access token not encrypted at rest: %q", accessEnc)
	}

	got, err := s.GetByEmail(ctx, "google", "me@gmail.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.AccessToken != "secret-access" || got.RefreshToken != "secret-refresh" {
		t.Fatalf("decrypt roundtrip failed: %+v", got)
	}

	// Reconnect (same identity) upserts instead of duplicating.
	acct2 := &store.CloudAccount{
		Provider:     "google",
		Email:        "me@gmail.com",
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		Status:       "active",
	}
	if err := s.Upsert(ctx, acct2); err != nil {
		t.Fatalf("Upsert #2: %v", err)
	}
	list := mustCloudList(t, s, ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1 account after reconnect, got %d", len(list))
	}
	if list[0].AccessToken != "new-access" {
		t.Fatalf("reconnect did not replace tokens")
	}
}

func TestCloudAccountsUserScopeIsolation(t *testing.T) {
	s, ctx := newCloudTestStore(t)
	if err := s.Upsert(ctx, &store.CloudAccount{
		Provider: "google", Email: "a@gmail.com", AccessToken: "ta", RefreshToken: "tr",
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	list := mustCloudList(t, s, ctx)

	// A foreign tenant+user context sees nothing.
	db := openTestDB(t)
	_ = db
	otherTenant := uuid.New()
	other := store.WithUserID(store.WithTenantID(context.Background(), otherTenant), "user-2")
	if got := mustCloudList(t, s, other); len(got) != 0 {
		t.Fatalf("other user sees %d foreign accounts", len(got))
	}
	// Foreign delete is a scoped no-op returning ErrCloudAccountNotFound.
	if err := s.Delete(other, list[0].ID); err == nil {
		t.Fatal("foreign delete succeeded")
	}
	// Foreign token update likewise.
	if err := s.UpdateTokens(other, list[0].ID, store.CloudAccountUpdate{AccessToken: "x", Status: "active"}); err == nil {
		t.Fatal("foreign token update succeeded")
	}
	// Owner can delete.
	if err := s.Delete(ctx, list[0].ID); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
}

func mustCloudList(t *testing.T, s *SQLiteCloudAccountStore, ctx context.Context) []store.CloudAccount {
	t.Helper()
	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return list
}
