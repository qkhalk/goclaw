package cloud

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// newTestCopyJob builds a finished copyJob with the given ownership record
// (registry tests do not run walkers).
func newTestCopyJob(rec TransferRecord, startedAt time.Time, finished bool) *copyJob {
	return &copyJob{rec: rec, startedAt: startedAt, finished: finished, done: make(chan struct{})}
}

// The transfer registry is the tenant-isolation boundary for async transfer
// polling: a job started by one user must be invisible to every other user
// AND every other tenant (GET /v1/cloud/transfers/{id} 404s, never leaks).
func TestTransferRegistryOwnership(t *testing.T) {
	reg := newTransferRegistry()

	tenantA, tenantB := uuid.New(), uuid.New()
	ctxA := store.WithTenantID(store.WithUserID(context.Background(), "alice"), tenantA)
	ctxA2 := store.WithTenantID(store.WithUserID(context.Background(), "mallory"), tenantA)
	ctxB := store.WithTenantID(store.WithUserID(context.Background(), "alice"), tenantB)

	id := reg.register(newTestCopyJob(TransferRecord{
		TenantID:        tenantA.String(),
		UserID:          "alice",
		SourceAccountID: "src",
		TargetAccountID: "dst",
		Mode:            "copy",
	}, time.Now(), true))
	if id <= 0 {
		t.Fatalf("register returned id %d, want positive", id)
	}

	if _, ok := reg.ownedBy(ctxA, id); !ok {
		t.Fatal("owner (same tenant+user) must see the job")
	}
	if _, ok := reg.ownedBy(ctxA2, id); ok {
		t.Fatal("same-tenant different user must NOT see the job")
	}
	if _, ok := reg.ownedBy(ctxB, id); ok {
		t.Fatal("same-user different tenant must NOT see the job")
	}
	if _, ok := reg.ownedBy(ctxA, id+999); ok {
		t.Fatal("unknown job must not resolve")
	}
}

func TestTransferRegistryEvictsAndPrunes(t *testing.T) {
	reg := newTransferRegistry()

	// Over the cap, the oldest records are evicted.
	tenant := uuid.New()
	ctx := store.WithTenantID(store.WithUserID(context.Background(), "u"), tenant)
	rec := TransferRecord{TenantID: tenant.String(), UserID: "u"}
	var firstID int64
	for i := 0; i <= transferMaxEntries+10; i++ {
		id := reg.register(newTestCopyJob(rec, time.Now(), true))
		if i == 0 {
			firstID = id
		}
	}
	if _, ok := reg.ownedBy(ctx, firstID); ok {
		t.Fatal("oldest job should have been evicted")
	}

	// Expired FINISHED records are pruned on the next register.
	reg2 := newTransferRegistry()
	reg2.jobs[7] = newTestCopyJob(TransferRecord{}, time.Now().Add(-2*transferTTL), true)
	reg2.register(newTestCopyJob(TransferRecord{}, time.Now(), true))
	if _, ok := reg2.jobs[7]; ok {
		t.Fatal("expired job should have been pruned")
	}

	// Unfinished (running) jobs are never pruned by TTL — they are cancelled
	// by shutdownAll instead.
	reg3 := newTransferRegistry()
	reg3.jobs[7] = newTestCopyJob(TransferRecord{}, time.Now().Add(-2*transferTTL), false)
	reg3.register(newTestCopyJob(TransferRecord{}, time.Now(), true))
	if _, ok := reg3.jobs[7]; !ok {
		t.Fatal("running job must not be pruned by TTL")
	}
}

func TestCopyJobSnapshotProgress(t *testing.T) {
	job := &copyJob{rec: TransferRecord{}, done: make(chan struct{})}
	snap := job.snapshot(1)
	if snap.Finished || snap.Success || snap.FilesDone != 0 {
		t.Fatalf("snapshot = %+v, want unfinished/zeroed", snap)
	}
	job.countFile()
	job.setTotal(3)
	job.mu.Lock()
	job.finished, job.success = true, false
	job.errMsg = "boom"
	job.mu.Unlock()
	snap = job.snapshot(1)
	if !snap.Finished || snap.Success || snap.Error != "boom" || snap.FilesDone != 1 || snap.FilesTotal != 3 {
		t.Fatalf("snapshot = %+v, want finished+failed with progress", snap)
	}
}

func TestModTimeCloseAndJoin(t *testing.T) {
	if !modTimeClose("2026-01-01T00:00:01Z", "2026-01-01T00:00:02Z") {
		t.Fatal("1s apart must be close")
	}
	if modTimeClose("2026-01-01T00:00:01Z", "2026-01-01T00:01:01Z") {
		t.Fatal("60s apart must not be close")
	}
	if modTimeClose("garbage", "2026-01-01T00:00:01Z") {
		t.Fatal("unparseable must never be close")
	}
	if got := joinRemotePath("", "a.txt"); got != "a.txt" {
		t.Fatalf("join root = %q", got)
	}
	if got := joinRemotePath("docs/sub", "a.txt"); got != "docs/sub/a.txt" {
		t.Fatalf("join = %q", got)
	}
}
