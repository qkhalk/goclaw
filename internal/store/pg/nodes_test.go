package pg

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// newNodeTestKey returns a random key material pair (plaintext + sha256 hex)
// without importing the internal/nodes package (it imports store/; keeping
// the pg store tests dependency-light avoids accidental cycles in tests).
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

func TestPGNodeStoreRoundtrip(t *testing.T) {
	db := hooksTestDB(t)
	st := NewPGNodeStore(db)
	tenantID, _ := seedTenantAndAgent(t, db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	_, hash := newNodeTestKey(t)
	n := &store.Node{
		Name:         "pg-roundtrip-1",
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

	byHash, err := st.GetByKeyHash(ctx, hash)
	if err != nil || byHash.ID != n.ID {
		t.Fatalf("GetByKeyHash = (%+v, %v)", byHash, err)
	}

	// Registration refresh updates advertised fields + last_seen.
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

func TestPGNodeStoreTenantScoping(t *testing.T) {
	db := hooksTestDB(t)
	st := NewPGNodeStore(db)
	tenantA, _ := seedTenantAndAgent(t, db)
	tenantB, _ := seedTenantAndAgent(t, db)

	_, hashA := newNodeTestKey(t)
	_, hashB := newNodeTestKey(t)
	if err := st.Create(store.WithTenantID(context.Background(), tenantA), &store.Node{Name: "a", NodeKeyHash: hashA}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if err := st.Create(store.WithTenantID(context.Background(), tenantB), &store.Node{Name: "b", NodeKeyHash: hashB}); err != nil {
		t.Fatalf("create B: %v", err)
	}

	listA, err := st.List(store.WithTenantID(context.Background(), tenantA))
	if err != nil {
		t.Fatalf("List A: %v", err)
	}
	for _, n := range listA {
		if n.Name == "b" {
			t.Fatal("tenant A list leaked tenant B's node")
		}
	}

	listB, _ := st.List(store.WithTenantID(context.Background(), tenantB))
	if len(listB) != 1 {
		t.Fatalf("list B = %d nodes, want 1", len(listB))
	}
	// GetByID from another tenant misses (scope clause).
	if _, err := st.GetByID(store.WithTenantID(context.Background(), tenantA), listB[0].ID); err == nil {
		t.Fatal("cross-tenant GetByID must miss")
	}
}

func TestPGNodeStoreTrustLifecycle(t *testing.T) {
	db := hooksTestDB(t)
	st := NewPGNodeStore(db)
	tenantID, _ := seedTenantAndAgent(t, db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	_, hash := newNodeTestKey(t)
	n := &store.Node{Name: "lifecycle", NodeKeyHash: hash}
	if err := st.Create(ctx, n); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// pending → trusted
	if err := st.SetTrust(ctx, n.ID, store.NodeTrustTrusted); err != nil {
		t.Fatalf("SetTrust trusted: %v", err)
	}
	got, _ := st.GetByID(ctx, n.ID)
	if got.Trust != store.NodeTrustTrusted {
		t.Fatalf("trust = %q", got.Trust)
	}

	// trusted → pending (un-approve)
	if err := st.SetTrust(ctx, n.ID, store.NodeTrustPending); err != nil {
		t.Fatalf("SetTrust pending: %v", err)
	}

	// revoke stamps revoked_at
	if err := st.Revoke(ctx, n.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, _ = st.GetByID(ctx, n.ID)
	if got.Trust != store.NodeTrustRevoked || got.RevokedAt == nil {
		t.Fatalf("revoked node = %+v", got)
	}

	// revoked is terminal
	if err := st.SetTrust(ctx, n.ID, store.NodeTrustTrusted); err == nil {
		t.Fatal("leaving revoked must be rejected")
	}

	// A revoked node cannot refresh its registration (stays dead).
	_, hash2 := newNodeTestKey(t)
	n2 := &store.Node{Name: "lifecycle-2", NodeKeyHash: hash2}
	if err := st.Create(ctx, n2); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if err := st.Revoke(ctx, n2.ID); err != nil {
		t.Fatalf("Revoke 2: %v", err)
	}
	if err := st.UpdateRegistration(ctx, n2.ID, "x", "y", nil); err == nil {
		t.Fatal("revoked node registration refresh must fail")
	}
	// Revoke is idempotent.
	if err := st.Revoke(ctx, n2.ID); err != nil {
		t.Fatalf("idempotent Revoke: %v", err)
	}
	// Unknown id → error.
	if err := st.Revoke(ctx, uuid.NewString()); err == nil {
		t.Fatal("unknown node revoke must fail")
	}
}
