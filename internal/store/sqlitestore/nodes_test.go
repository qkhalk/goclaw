//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func newNodeTestKey(t *testing.T) (plain, hash string) {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("entropy: %v", err)
	}
	plain = "gnk_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(sum[:])
}

func TestSQLiteNodeStoreRoundtrip(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	st := NewSQLiteNodeStore(db)
	tenantID := uuid.Must(uuid.NewV7())
	seedSQLiteRunTimelineTenant(t, db, tenantID)
	ctx := store.WithTenantID(context.Background(), tenantID)

	_, hash := newNodeTestKey(t)
	n := &store.Node{
		Name:         "sqlite-roundtrip-1",
		NodeKeyHash:  hash,
		Platform:     "linux/amd64",
		Capabilities: []string{"exec", "fs"},
	}
	if err := st.Create(ctx, n); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.ID == "" {
		t.Fatal("Create did not assign an ID")
	}
	if n.Trust != store.NodeTrustPending {
		t.Fatalf("trust = %q, want default pending", n.Trust)
	}

	got, err := st.GetByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != n.Name || got.Platform != "linux/amd64" {
		t.Fatalf("got = %+v", got)
	}
	if len(got.Capabilities) != 2 || got.Capabilities[0] != "exec" || got.Capabilities[1] != "fs" {
		t.Fatalf("capabilities = %v", got.Capabilities)
	}
	if got.TenantID == nil || *got.TenantID != tenantID.String() {
		t.Fatalf("tenant_id = %v, want %v", got.TenantID, tenantID)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatal("timestamps must round-trip from SQLite text storage")
	}

	byHash, err := st.GetByKeyHash(ctx, hash)
	if err != nil || byHash.ID != n.ID {
		t.Fatalf("GetByKeyHash = (%+v, %v)", byHash, err)
	}

	if err := st.UpdateRegistration(ctx, n.ID, "renamed", "darwin/arm64", []string{"exec"}); err != nil {
		t.Fatalf("UpdateRegistration: %v", err)
	}
	got, _ = st.GetByID(ctx, n.ID)
	if got.Name != "renamed" || got.Platform != "darwin/arm64" || len(got.Capabilities) != 1 {
		t.Fatalf("after refresh got = %+v", got)
	}
	if got.LastSeenAt == nil {
		t.Fatal("UpdateRegistration must stamp last_seen_at")
	}
}

func TestSQLiteNodeStoreTrustLifecycle(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	st := NewSQLiteNodeStore(db)
	tenantID := uuid.Must(uuid.NewV7())
	seedSQLiteRunTimelineTenant(t, db, tenantID)
	ctx := store.WithTenantID(context.Background(), tenantID)

	_, hash := newNodeTestKey(t)
	n := &store.Node{Name: "lifecycle", NodeKeyHash: hash}
	if err := st.Create(ctx, n); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := st.SetTrust(ctx, n.ID, store.NodeTrustTrusted); err != nil {
		t.Fatalf("SetTrust trusted: %v", err)
	}
	got, _ := st.GetByID(ctx, n.ID)
	if got.Trust != store.NodeTrustTrusted {
		t.Fatalf("trust = %q", got.Trust)
	}
	if err := st.SetTrust(ctx, n.ID, store.NodeTrustPending); err != nil {
		t.Fatalf("SetTrust pending: %v", err)
	}
	if err := st.Revoke(ctx, n.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, _ = st.GetByID(ctx, n.ID)
	if got.Trust != store.NodeTrustRevoked || got.RevokedAt == nil {
		t.Fatalf("revoked node = %+v", got)
	}
	if err := st.SetTrust(ctx, n.ID, store.NodeTrustTrusted); err == nil {
		t.Fatal("leaving revoked must be rejected")
	}
	if err := st.Revoke(ctx, n.ID); err != nil {
		t.Fatalf("idempotent Revoke: %v", err)
	}
	if err := st.Revoke(ctx, uuid.NewString()); err == nil {
		t.Fatal("unknown node revoke must fail")
	}
}
