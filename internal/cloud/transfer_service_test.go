package cloud

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// The transfer registry is the tenant-isolation boundary for async transfer
// polling: a job started by one user must be invisible to every other user
// AND every other tenant (GET /v1/cloud/transfers/{id} 404s, never leaks).
func TestTransferRegistryOwnership(t *testing.T) {
	reg := newTransferRegistry()

	tenantA, tenantB := uuid.New(), uuid.New()
	ctxA := store.WithTenantID(store.WithUserID(context.Background(), "alice"), tenantA)
	ctxA2 := store.WithTenantID(store.WithUserID(context.Background(), "mallory"), tenantA)
	ctxB := store.WithTenantID(store.WithUserID(context.Background(), "alice"), tenantB)

	reg.register(42, TransferRecord{
		TenantID:        tenantA.String(),
		UserID:          "alice",
		SourceAccountID: "src",
		TargetAccountID: "dst",
		Mode:            "copy",
	})

	if _, ok := reg.ownedBy(ctxA, 42); !ok {
		t.Fatal("owner (same tenant+user) must see the job")
	}
	if _, ok := reg.ownedBy(ctxA2, 42); ok {
		t.Fatal("same-tenant different user must NOT see the job")
	}
	if _, ok := reg.ownedBy(ctxB, 42); ok {
		t.Fatal("same-user different tenant must NOT see the job")
	}
	if _, ok := reg.ownedBy(ctxA, 43); ok {
		t.Fatal("unknown job must not resolve")
	}
}

func TestTransferRegistryIgnoresInvalidAndEvicts(t *testing.T) {
	reg := newTransferRegistry()

	// Non-positive IDs (synchronous transfers) are never registered.
	reg.register(0, TransferRecord{})
	if len(reg.jobs) != 0 {
		t.Fatalf("jobs = %d, want 0 after register(0)", len(reg.jobs))
	}

	// Over the cap, the oldest records are evicted.
	tenant := uuid.New()
	ctx := store.WithTenantID(store.WithUserID(context.Background(), "u"), tenant)
	rec := TransferRecord{TenantID: tenant.String(), UserID: "u"}
	for i := int64(1); i <= transferMaxEntries+10; i++ {
		reg.register(i, rec)
	}
	if _, ok := reg.ownedBy(ctx, 1); ok {
		t.Fatal("oldest job should have been evicted")
	}
	if _, ok := reg.ownedBy(ctx, transferMaxEntries+10); !ok {
		t.Fatal("newest job should still be present")
	}

	// Expired records are pruned on the next register.
	reg2 := newTransferRegistry()
	reg2.jobs[7] = TransferRecord{StartedAt: time.Now().Add(-2 * transferTTL)}
	reg2.register(8, TransferRecord{})
	if _, ok := reg2.jobs[7]; ok {
		t.Fatal("expired job should have been pruned")
	}
}
