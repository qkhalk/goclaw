package cloud

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SyncService runs the tenant-configured cloud sync pairs (one-way additive
// mirrors) in the background. Design notes:
//
//   - A single goroutine sweeps every tenant on a fixed tick; pairs of the
//     same tenant run SEQUENTIALLY (provider rate-limit friendly, and an
//     accidental A→B + B→A loop degrades to idempotent re-copy, not ping-pong).
//   - Folder sync goes through sync/copy ASYNC + job polling — the rc client's
//     60s timeout makes blocking folder syncs a guaranteed hang for big trees.
//   - last_run_at is set at run START (MarkRunning); MarkResult only records
//     the outcome. The due check therefore never re-fires a running pair.
//   - Pairs stuck in "running" after a crash are failed at Start (stale
//     cleanup, 30 min cutoff).
//   - Manual "run now" pushes a request into a small queue the loop drains —
//     never executed inside the HTTP handler (no 60s timeout exposure).
type SyncService struct {
	pairs    store.CloudSyncPairStore
	accounts tenantAccountLister
	tenants  tenantLister
	runner   syncRunner
	now      func() time.Time

	tick   time.Duration // sweep cadence
	poll   time.Duration // async job poll cadence
	maxRun time.Duration // give up polling a single job after this

	runMu   sync.Mutex
	runReqQ []syncRunReq
	notify  chan struct{}
}

// syncRunReq is one queued manual run: the pair plus the tenant that owns it
// (the worker context has no tenant of its own).
type syncRunReq struct {
	TenantID string
	PairID   string
}

// tenantAccountLister is the account-store subset the worker needs: every
// account of one tenant without a user context (narrow for testability).
type tenantAccountLister interface {
	ListTenant(ctx context.Context) ([]store.CloudAccount, error)
}

// tenantLister is the tenant-store subset the worker needs.
type tenantLister interface {
	ListTenants(ctx context.Context) ([]store.TenantData, error)
}

// syncRunner is the storage subset the worker needs (fakeable in tests).
type syncRunner interface {
	SyncCopyAccounts(ctx context.Context, src, dst *store.CloudAccount, srcPath, dstPath string) (int64, error)
	SyncJobStatus(ctx context.Context, jobID int64) (*storage.JobInfo, error)
}

// Sync tuning defaults. The tick is a compromise: due precision of one minute
// is plenty for hourly/daily schedules, and each sweep is one cheap indexed
// query per tenant with pairs.
const (
	DefaultSyncTick     = 60 * time.Second
	syncJobPollInterval = 5 * time.Second
	syncJobMaxRun       = time.Hour
	// staleRunningCutoff: a pair "running" longer than this means the process
	// died mid-run (crash / kill -9) — the poll loop would have failed it.
	staleRunningCutoff = 30 * time.Minute
	// runQueueCap bounds manual run requests; beyond that the request is
	// rejected (the UI surfaces the error) instead of growing unbounded.
	runQueueCap = 64
)

// NewSyncService creates the sync worker. All stores may be nil-checked by the
// caller (wiring skips Start when any dependency is missing). Both stores are
// accepted as their concrete store types and narrow to what the worker needs.
func NewSyncService(pairs store.CloudSyncPairStore, accounts tenantAccountLister, tenants tenantLister, runner syncRunner) *SyncService {
	return &SyncService{
		pairs:    pairs,
		accounts: accounts,
		tenants:  tenants,
		runner:   runner,
		now:      time.Now,
		tick:     DefaultSyncTick,
		poll:     syncJobPollInterval,
		maxRun:   syncJobMaxRun,
		notify:   make(chan struct{}, 1),
	}
}

// Start launches the sweep loop; it returns immediately. The loop exits when
// ctx is cancelled (gateway shutdown). Stale "running" pairs are failed first.
func (s *SyncService) Start(ctx context.Context) {
	s.cleanupStaleRunning(ctx)
	go s.loop(ctx)
	slog.Info("cloud sync: worker started", "tick", s.tick.String())
}

// RunNow queues one manual run. The pair must exist and be enabled in the
// caller's tenant; execution happens on the worker loop.
func (s *SyncService) RunNow(ctx context.Context, pairID string) error {
	p, err := s.pairs.Get(ctx, pairID)
	if err != nil {
		return err
	}
	if !p.Enabled {
		return errors.New("cloud sync: pair is disabled")
	}
	s.runMu.Lock()
	if len(s.runReqQ) >= runQueueCap {
		s.runMu.Unlock()
		return errors.New("cloud sync: run queue is full, try again shortly")
	}
	s.runReqQ = append(s.runReqQ, syncRunReq{TenantID: store.TenantIDFromContext(ctx).String(), PairID: pairID})
	s.runMu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
	return nil
}

// loop is the worker main loop: wake (manual runs) or tick (due sweep).
func (s *SyncService) loop(ctx context.Context) {
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.notify:
		case <-ticker.C:
		}
		s.drainQueue(ctx)
		s.sweep(ctx)
	}
}

// drainQueue runs every queued manual request (skipping pairs that vanished).
func (s *SyncService) drainQueue(ctx context.Context) {
	for {
		s.runMu.Lock()
		if len(s.runReqQ) == 0 {
			s.runMu.Unlock()
			return
		}
		req := s.runReqQ[0]
		s.runReqQ = s.runReqQ[1:]
		s.runMu.Unlock()

		tenantID, err := parseTenantID(req.TenantID)
		if err != nil {
			continue
		}
		pairCtx := store.WithTenantID(ctx, tenantID)
		p, err := s.pairs.Get(pairCtx, req.PairID)
		if err != nil {
			slog.Warn("cloud sync: queued pair vanished", "pair_id", req.PairID, "error", err)
			continue
		}
		s.runPair(pairCtx, p, s.now())
	}
}

// sweep runs every due scheduled pair of every tenant, sequentially.
func (s *SyncService) sweep(ctx context.Context) {
	tenants, err := s.tenants.ListTenants(ctx)
	if err != nil {
		slog.Warn("cloud sync: list tenants failed", "error", err)
		return
	}
	now := s.now()
	for _, tn := range tenants {
		if ctx.Err() != nil {
			return
		}
		tenantCtx := store.WithTenantID(ctx, tn.ID)
		due, err := s.pairs.DuePairs(tenantCtx, now)
		if err != nil {
			slog.Warn("cloud sync: due pairs query failed", "tenant", tn.ID, "error", err)
			continue
		}
		for i := range due {
			s.runPair(tenantCtx, &due[i], now)
			if ctx.Err() != nil {
				return
			}
		}
	}
}

// cleanupStaleRunning fails pairs still marked "running" from a previous
// process lifetime (crash mid-run — the poll loop would have closed them out).
func (s *SyncService) cleanupStaleRunning(ctx context.Context) {
	tenants, err := s.tenants.ListTenants(ctx)
	if err != nil {
		slog.Warn("cloud sync: stale cleanup skipped (list tenants failed)", "error", err)
		return
	}
	cutoff := s.now().Add(-staleRunningCutoff)
	for _, tn := range tenants {
		fixed, err := s.pairs.FailStaleRunning(store.WithTenantID(ctx, tn.ID), cutoff)
		if err != nil {
			slog.Warn("cloud sync: stale cleanup failed", "tenant", tn.ID, "error", err)
			continue
		}
		if fixed > 0 {
			slog.Warn("cloud sync: marked stale running pairs as errored", "tenant", tn.ID, "count", fixed)
		}
	}
}

// parseTenantID parses a queued tenant UUID (invalid queue entries are skipped).
func parseTenantID(raw string) (uuid.UUID, error) {
	return uuid.Parse(raw)
}

// runPair executes one pair end-to-end: resolve accounts (one ListTenant per
// tenant sweep — never per pair), mark running, async copy + poll, mark
// result. Never panics into the loop.
func (s *SyncService) runPair(ctx context.Context, p *store.CloudSyncPair, now time.Time) {
	if err := s.pairs.MarkRunning(ctx, p.ID, now); err != nil {
		slog.Warn("cloud sync: mark running failed", "pair_id", p.ID, "error", err)
		return
	}
	runErr := s.executePair(ctx, p)
	if runErr != nil {
		slog.Warn("cloud sync: pair failed", "pair_id", p.ID, "source", p.SourceAccountID, "target", p.TargetAccountID, "error", runErr)
		if merr := s.pairs.MarkResult(ctx, p.ID, store.SyncStatusError, truncateSyncErr(runErr)); merr != nil {
			slog.Warn("cloud sync: mark result failed", "pair_id", p.ID, "error", merr)
		}
		return
	}
	if merr := s.pairs.MarkResult(ctx, p.ID, store.SyncStatusOK, ""); merr != nil {
		slog.Warn("cloud sync: mark result failed", "pair_id", p.ID, "error", merr)
	}
}

// executePair resolves the pair's accounts and performs the mirror copy.
func (s *SyncService) executePair(ctx context.Context, p *store.CloudSyncPair) error {
	accounts, err := s.accounts.ListTenant(ctx)
	if err != nil {
		return fmt.Errorf("resolve tenant accounts: %w", err)
	}
	byID := make(map[string]*store.CloudAccount, len(accounts))
	for i := range accounts {
		byID[accounts[i].ID] = &accounts[i]
	}
	src := byID[p.SourceAccountID]
	dst := byID[p.TargetAccountID]
	if src == nil || dst == nil {
		return errors.New("source or target account is no longer available in this tenant")
	}
	jobID, err := s.runner.SyncCopyAccounts(ctx, src, dst, p.SourcePath, p.TargetPath)
	if err != nil {
		return err
	}
	return s.awaitJob(ctx, jobID, p.ID)
}

// awaitJob polls the async rc job until it finishes, fails, or the deadline
// (or the gateway shutdown) cuts the run short.
func (s *SyncService) awaitJob(ctx context.Context, jobID int64, pairID string) error {
	deadline := s.now().Add(s.maxRun)
	for {
		job, err := s.runner.SyncJobStatus(ctx, jobID)
		if err != nil {
			// An rcd restart invalidates job ids — treat as a failed run, the
			// next sweep re-copies what is missing (additive mirror is idempotent).
			return fmt.Errorf("poll job %d: %w", jobID, err)
		}
		if job.Finished {
			if job.Success {
				return nil
			}
			if job.Error != "" {
				return fmt.Errorf("job %d: %s", jobID, job.Error)
			}
			return fmt.Errorf("job %d failed", jobID)
		}
		if !s.now().Before(deadline) {
			return fmt.Errorf("job %d exceeded the %s run budget", jobID, s.maxRun)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("run cancelled: %w", ctx.Err())
		case <-time.After(s.poll):
		}
	}
}

func truncateSyncErr(err error) string {
	msg := err.Error()
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}
