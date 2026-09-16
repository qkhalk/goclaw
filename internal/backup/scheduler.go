package backup

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrBackupInProgress is returned by RunNow when a run is already executing.
var ErrBackupInProgress = errors.New("a backup is already in progress")

// Scheduler runs the periodic backup: a 1-minute ticker re-reads the config
// (so enable/interval changes apply without a restart), and when due it
// executes the injected pipeline (backup → upload → retention). The pipeline
// is injected by cmd wiring so this package stays free of cloud/S3 imports.
type Scheduler struct {
	secrets store.ConfigSecretsStore
	runOnce func(ctx context.Context, cfg ScheduleConfig) (artifact string, size int64, err error)

	mu      sync.Mutex
	last    *ScheduleRun
	nextDue time.Time
	busy    bool
	stopCh  chan struct{}
	stopped bool
}

// NewScheduler builds a scheduler. runOnce is only ever called from one
// goroutine at a time (the busy flag serializes runs).
func NewScheduler(
	secrets store.ConfigSecretsStore,
	runOnce func(ctx context.Context, cfg ScheduleConfig) (string, int64, error),
) *Scheduler {
	return &Scheduler{secrets: secrets, runOnce: runOnce, stopCh: make(chan struct{})}
}

// Save validates and persists a new schedule config (API write path).
func (s *Scheduler) Save(ctx context.Context, cfg ScheduleConfig) error {
	return SaveScheduleConfig(ctx, s.secrets, cfg)
}

// Start launches the loop goroutine.
func (s *Scheduler) Start() {
	go s.loop()
}

// Stop terminates the loop. An in-flight run finishes in the background
// (its result is still persisted).
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		s.stopped = true
		close(s.stopCh)
	}
}

// masterCtx returns a background context scoped to the master tenant, where
// the schedule config lives.
func masterCtx() context.Context {
	return store.WithTenantID(context.Background(), store.MasterTenantID)
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	ctx := masterCtx()
	cfg, err := LoadScheduleConfig(ctx, s.secrets)
	if err != nil {
		slog.Warn("backup.schedule: load config failed", "error", err)
		return
	}
	s.mu.Lock()
	due := cfg.Enabled && !s.busy && time.Now().After(s.nextDue)
	s.mu.Unlock()
	if !due {
		return
	}
	run := s.execute(ctx, cfg)
	_ = run
}

// RunNow triggers a run immediately (manual "run now" API). A run already in
// progress returns ErrBackupInProgress.
func (s *Scheduler) RunNow(ctx context.Context) (ScheduleRun, error) {
	cfg, err := LoadScheduleConfig(ctx, s.secrets)
	if err != nil {
		return ScheduleRun{At: time.Now().UTC(), Error: err.Error()}, err
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return ScheduleRun{At: time.Now().UTC()}, ErrBackupInProgress
	}
	s.busy = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()
	return s.execute(ctx, cfg), nil
}

func (s *Scheduler) execute(ctx context.Context, cfg ScheduleConfig) ScheduleRun {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return ScheduleRun{At: time.Now().UTC(), Error: ErrBackupInProgress.Error()}
	}
	s.busy = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	start := time.Now().UTC()
	run := ScheduleRun{At: start}
	defer func() {
		run.DurationMS = time.Since(start).Milliseconds()
		if r := recover(); r != nil {
			run.Error = "panic during scheduled backup"
			slog.Error("backup.schedule: panic", "recover", r)
		}
		s.recordRun(ctx, run, cfg)
	}()

	artifact, size, err := s.runOnce(ctx, cfg)
	if err != nil {
		run.Error = err.Error()
		slog.Error("backup.schedule: run failed", "error", err)
		return run
	}
	run.OK = true
	run.Artifact = artifact
	run.SizeBytes = size
	slog.Info("backup.schedule: run completed", "artifact", artifact, "size_bytes", size)
	return run
}

func (s *Scheduler) recordRun(ctx context.Context, run ScheduleRun, cfg ScheduleConfig) {
	interval := cfg.IntervalHours
	if interval < 1 {
		interval = 24
	}
	s.mu.Lock()
	s.last = &run
	s.nextDue = run.At.Add(time.Duration(interval) * time.Hour)
	s.mu.Unlock()
	if s.secrets != nil {
		if raw, err := json.Marshal(run); err == nil {
			if err := s.secrets.Set(ctx, ScheduleLastKey, string(raw)); err != nil {
				slog.Warn("backup.schedule: persist last-run failed", "error", err)
			}
		}
	}
}

// Status reports config, last run, and the next due time for the API. It
// re-seeds in-memory state from the persisted last-run on first call after
// boot so restarts don't lose the cadence.
func (s *Scheduler) Status(ctx context.Context) (cfg ScheduleConfig, last *ScheduleRun, nextDue time.Time, err error) {
	cfg, err = LoadScheduleConfig(ctx, s.secrets)
	if err != nil {
		return
	}
	interval := max(cfg.IntervalHours, 1)
	s.mu.Lock()
	last = s.last
	nextDue = s.nextDue
	seed := s.last == nil
	s.mu.Unlock()
	if seed {
		if persisted, lerr := LoadScheduleLastRun(ctx, s.secrets); lerr == nil && persisted != nil {
			s.mu.Lock()
			if s.last == nil {
				s.last = persisted
				s.nextDue = persisted.At.Add(time.Duration(interval) * time.Hour)
			}
			last = s.last
			nextDue = s.nextDue
			s.mu.Unlock()
		}
	}
	return
}
