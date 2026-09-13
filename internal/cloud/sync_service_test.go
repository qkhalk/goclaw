package cloud

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Worker state-machine tests with fakes only — no rclone, no DB.

type fakePairStore struct {
	pairs    map[string]*store.CloudSyncPair
	byTenant map[string][]string // tenant → pair IDs (due-pairs pre-filter basis)
	marked   []string            // "pair:status" transitions
}

func newFakePairStore() *fakePairStore {
	return &fakePairStore{pairs: map[string]*store.CloudSyncPair{}, byTenant: map[string][]string{}}
}

func (f *fakePairStore) add(tenantID string, p *store.CloudSyncPair) {
	f.pairs[p.ID] = p
	f.byTenant[tenantID] = append(f.byTenant[tenantID], p.ID)
}

func (f *fakePairStore) tenantOf(pairID string) string {
	for t, ids := range f.byTenant {
		for _, id := range ids {
			if id == pairID {
				return t
			}
		}
	}
	return ""
}

func (f *fakePairStore) List(ctx context.Context) ([]store.CloudSyncPair, error) { return nil, nil }

func (f *fakePairStore) Get(ctx context.Context, id string) (*store.CloudSyncPair, error) {
	if p, ok := f.pairs[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, store.ErrCloudSyncPairNotFound
}

func (f *fakePairStore) Create(context.Context, *store.CloudSyncPair) error { return nil }

func (f *fakePairStore) Update(context.Context, *store.CloudSyncPair) error { return nil }

func (f *fakePairStore) Delete(context.Context, string) error { return nil }

func (f *fakePairStore) MarkRunning(_ context.Context, id string, at time.Time) error {
	p := f.pairs[id]
	if p == nil {
		return store.ErrCloudSyncPairNotFound
	}
	t := at
	p.LastRunAt = &t
	p.LastStatus = store.SyncStatusRunning
	p.LastError = ""
	f.marked = append(f.marked, id+":running")
	return nil
}

func (f *fakePairStore) MarkResult(_ context.Context, id, status, runErr string) error {
	p := f.pairs[id]
	if p == nil {
		return store.ErrCloudSyncPairNotFound
	}
	p.LastStatus = status
	p.LastError = runErr
	f.marked = append(f.marked, id+":"+status)
	return nil
}

func (f *fakePairStore) DuePairs(_ context.Context, now time.Time) ([]store.CloudSyncPair, error) {
	var out []store.CloudSyncPair
	for _, ids := range f.byTenant {
		for _, id := range ids {
			p := f.pairs[id]
			if p.IntervalMinutes > 0 && p.Enabled && (p.LastRunAt == nil || now.Sub(*p.LastRunAt) >= time.Duration(p.IntervalMinutes)*time.Minute) {
				out = append(out, *p)
			}
		}
	}
	return out, nil
}

func (f *fakePairStore) FailStaleRunning(_ context.Context, cutoff time.Time) (int64, error) {
	var fixed int64
	for _, p := range f.pairs {
		if p.LastStatus == store.SyncStatusRunning && p.LastRunAt != nil && p.LastRunAt.Before(cutoff) {
			p.LastStatus = store.SyncStatusError
			p.LastError = "stale"
			fixed++
		}
	}
	return fixed, nil
}

type fakeAccounts struct{ accounts []store.CloudAccount }

func (f *fakeAccounts) ListTenant(context.Context) ([]store.CloudAccount, error) {
	return f.accounts, nil
}

type fakeTenants struct{ ids []uuid.UUID }

func (f *fakeTenants) ListTenants(context.Context) ([]store.TenantData, error) {
	out := make([]store.TenantData, 0, len(f.ids))
	for _, id := range f.ids {
		out = append(out, store.TenantData{ID: id})
	}
	return out, nil
}

type fakeRunner struct {
	// jobIDs returned per SyncCopyAccounts call; a negative value simulates a
	// synchronous failure (error below).
	jobIDs   []int64
	jobErr   error
	statuses []storage.JobInfo // polled in order; last one repeats
	polls    int
	copied   [][2]string // "srcAccount:path" → "dstAccount:path"
}

func (f *fakeRunner) SyncCopyAccounts(_ context.Context, src, dst *store.CloudAccount, srcPath, dstPath string) (int64, error) {
	if f.jobErr != nil {
		return 0, f.jobErr
	}
	f.copied = append(f.copied, [2]string{src.ID + ":" + srcPath, dst.ID + ":" + dstPath})
	id := f.jobIDs[0]
	f.jobIDs = f.jobIDs[1:]
	return id, nil
}

func (f *fakeRunner) SyncJobStatus(context.Context, int64) (*storage.JobInfo, error) {
	if len(f.statuses) == 0 {
		return nil, fmt.Errorf("no scripted job status")
	}
	idx := f.polls
	if idx >= len(f.statuses) {
		idx = len(f.statuses) - 1
	}
	f.polls++
	info := f.statuses[idx]
	return &info, nil
}

func fixture(t *testing.T) (*SyncService, *fakePairStore, *fakeRunner, *fakeAccounts, string) {
	t.Helper()
	tenID := uuid.New()
	pairs := newFakePairStore()
	accounts := &fakeAccounts{accounts: []store.CloudAccount{
		{ID: "src-1", TenantID: tenID.String(), Provider: "google", Status: "active"},
		{ID: "dst-1", TenantID: tenID.String(), Provider: "google", Status: "active"},
	}}
	runner := &fakeRunner{}
	svc := NewSyncService(pairs, accounts, &fakeTenants{ids: []uuid.UUID{tenID}}, runner)
	return svc, pairs, runner, accounts, tenID.String()
}

func syncTestPair(id string, interval int, enabled bool) *store.CloudSyncPair {
	return &store.CloudSyncPair{
		ID: id, SourceAccountID: "src-1", SourcePath: "docs",
		TargetAccountID: "dst-1", TargetPath: "backup",
		IntervalMinutes: interval, Enabled: enabled,
	}
}

func TestSyncServiceRunsDuePairAndMarksOK(t *testing.T) {
	svc, pairs, runner, _, tenID := fixture(t)
	pairs.add(tenID, syncTestPair("p1", 60, true))
	runner.jobIDs = []int64{42}
	runner.statuses = []storage.JobInfo{
		{ID: 42, Finished: false},
		{ID: 42, Finished: true, Success: true},
	}

	svc.sweep(context.Background())

	p := pairs.pairs["p1"]
	if p.LastStatus != store.SyncStatusOK {
		t.Fatalf("status = %q, want ok (last_error=%q)", p.LastStatus, p.LastError)
	}
	if len(runner.copied) != 1 || runner.copied[0] != [2]string{"src-1:docs", "dst-1:backup"} {
		t.Fatalf("copied = %v, want one src-1:docs → dst-1:backup", runner.copied)
	}
}

func TestSyncServiceMarksErrorOnFailedJob(t *testing.T) {
	svc, pairs, runner, _, tenID := fixture(t)
	pairs.add(tenID, syncTestPair("p1", 0, true)) // interval 0: only reachable via the queue
	runner.jobIDs = []int64{7}
	runner.statuses = []storage.JobInfo{{ID: 7, Finished: true, Success: false, Error: "boom"}}

	// Queue a manual run the way RunNow would (bypassing store.Get — the fake
	// store has the pair).
	svc.runReqQ = append(svc.runReqQ, syncRunReq{TenantID: tenID, PairID: "p1"})
	svc.drainQueue(context.Background())

	p := pairs.pairs["p1"]
	if p.LastStatus != store.SyncStatusError || p.LastError == "" {
		t.Fatalf("status = %q error = %q, want error with message", p.LastStatus, p.LastError)
	}
}

func TestSyncServiceSkipsNotYetDuePairs(t *testing.T) {
	svc, pairs, runner, _, tenID := fixture(t)
	p := syncTestPair("p1", 60, true)
	recent := time.Now().Add(-5 * time.Minute) // ran 5 min ago, interval 60 min
	p.LastRunAt = &recent
	p.LastStatus = store.SyncStatusOK
	pairs.add(tenID, p)

	svc.sweep(context.Background())

	if len(runner.copied) != 0 {
		t.Fatalf("pair ran although not due: %v", runner.copied)
	}
}

func TestSyncServiceStaleRunningCleanup(t *testing.T) {
	svc, pairs, _, _, tenID := fixture(t)
	p := syncTestPair("p1", 60, true)
	stale := time.Now().Add(-2 * time.Hour) // "running" since 2h ago: crash
	p.LastRunAt = &stale
	p.LastStatus = store.SyncStatusRunning
	pairs.add(tenID, p)
	fresh := syncTestPair("p2", 60, true)
	recent := time.Now().Add(-5 * time.Minute)
	fresh.LastRunAt = &recent
	fresh.LastStatus = store.SyncStatusRunning
	pairs.add(tenID, fresh)

	svc.cleanupStaleRunning(context.Background())

	if pairs.pairs["p1"].LastStatus != store.SyncStatusError {
		t.Fatalf("stale pair status = %q, want error", pairs.pairs["p1"].LastStatus)
	}
	if pairs.pairs["p2"].LastStatus != store.SyncStatusRunning {
		t.Fatalf("fresh pair status = %q, want still running", pairs.pairs["p2"].LastStatus)
	}
}

func TestSyncServiceMissingAccountMarksError(t *testing.T) {
	svc, pairs, runner, accounts, tenID := fixture(t)
	pairs.add(tenID, syncTestPair("p1", 60, true))
	accounts.accounts = accounts.accounts[:1] // source only — target gone

	svc.sweep(context.Background())

	if pairs.pairs["p1"].LastStatus != store.SyncStatusError {
		t.Fatalf("status = %q, want error for missing target account", pairs.pairs["p1"].LastStatus)
	}
	if len(runner.copied) != 0 {
		t.Fatalf("no copy should have been attempted, got %v", runner.copied)
	}
}

func TestSyncServiceRunNowRejectsDisabledAndMissing(t *testing.T) {
	svc, pairs, _, _, tenID := fixture(t)
	pairs.add(tenID, syncTestPair("p1", 0, false))

	ctx := context.Background()
	if err := svc.RunNow(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing pair")
	}
	if err := svc.RunNow(ctx, "p1"); err == nil {
		t.Fatal("expected error for disabled pair")
	}
	if len(svc.runReqQ) != 0 {
		t.Fatalf("queue should be empty, has %d entries", len(svc.runReqQ))
	}
}

func TestSyncServiceAwaitJobTimeout(t *testing.T) {
	svc, _, runner, _, _ := fixture(t)
	runner.statuses = []storage.JobInfo{{ID: 9, Finished: false}} // never finishes
	svc.maxRun = 50 * time.Millisecond
	svc.poll = 10 * time.Millisecond

	err := svc.awaitJob(context.Background(), 9, "p1")
	if err == nil {
		t.Fatal("expected run-budget timeout error")
	}
}
