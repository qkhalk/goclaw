package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/adhocore/gronx"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// KeySchedule is the config_secrets key holding the backup schedule as a single
// JSON blob. Stored via the same ConfigSecretsStore the S3 config uses
// (backup.s3.* keys), so no new persistence surface is introduced.
const KeySchedule = "backup.schedule"

// DefaultScheduleInterval is used when a schedule is enabled without an
// explicit interval: daily at 03:00 UTC.
const DefaultScheduleInterval = "0 3 * * *"

// MinScheduleInterval is the smallest allowed duration-based interval. Backup
// runs dump the whole database + filesystem, so anything tighter is abusive.
const MinScheduleInterval = 1 * time.Hour

// maxScheduledRunDuration bounds a single scheduled backup run.
const maxScheduledRunDuration = 24 * time.Hour

// BackupSchedule describes a periodic backup-to-cloud schedule.
// Interval is either a Go duration string ("6h", "24h") or a 5-field cron
// expression ("0 3 * * *"). Destination is currently only "s3".
type BackupSchedule struct {
	Enabled     bool       `json:"enabled"`
	Interval    string     `json:"interval"`
	Destination string     `json:"destination"`
	Retention   int        `json:"retention"` // keep N most recent objects in cloud (0 = unlimited)
	LastRun     *time.Time `json:"last_run,omitempty"`
	NextRun     *time.Time `json:"next_run,omitempty"`
	LastStatus  string     `json:"last_status,omitempty"` // "ok" | "error" | "running"
	LastError   string     `json:"last_error,omitempty"`
}

// S3ObjectStore is the subset of *S3Client used by the scheduler. Method
// signatures match S3Client exactly so the concrete client satisfies the
// interface; tests substitute a fake.
type S3ObjectStore interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64) error
	ListBackups(ctx context.Context) ([]BackupEntry, error)
	Delete(ctx context.Context, key string) error
}

// ScheduleDeps carries everything the scheduler needs to execute a backup run.
// It mirrors the fields the one-shot HTTP handler (backup_s3_handler) passes
// to Run(), so scheduled backups produce identical archives.
type ScheduleDeps struct {
	DSN           string
	DataDir       string
	WorkspacePath string
	GoclawVersion string
	SchemaVersion int
}

// ScheduleService runs periodic backups to S3. It owns a single ticker
// goroutine; enabled/disabled and interval changes are picked up live on each
// tick by re-reading the persisted schedule (no restart needed).
type ScheduleService struct {
	secrets store.ConfigSecretsStore
	deps    ScheduleDeps

	// Now is indirected for tests. Defaults to time.Now.
	Now func() time.Time
	// NewClient builds the S3 object store from a config. Indirected for tests.
	NewClient func(cfg *S3Config) (S3ObjectStore, error)
	// TickInterval overrides the scheduler check cadence (production: 30s).
	TickInterval time.Duration

	lifecycleMu sync.Mutex // guards running + stopCh
	running     bool
	stopCh      chan struct{}
	loopDone    sync.WaitGroup

	runMu sync.Mutex // serializes backup runs (scheduled ticks vs manual run-now)
}

// NewScheduleService creates a scheduler persisting its schedule in secrets.
func NewScheduleService(secrets store.ConfigSecretsStore, deps ScheduleDeps) *ScheduleService {
	return &ScheduleService{
		secrets:      secrets,
		deps:         deps,
		Now:          time.Now,
		NewClient:    func(cfg *S3Config) (S3ObjectStore, error) { return NewS3Client(cfg) },
		TickInterval: 30 * time.Second,
	}
}

// Start launches the scheduling loop. Idempotent: a second call is a no-op.
func (s *ScheduleService) Start() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.running {
		return nil
	}
	s.stopCh = make(chan struct{})
	s.running = true
	s.loopDone.Add(1)
	go func() {
		defer s.loopDone.Done()
		s.runLoop(s.stopCh)
	}()
	slog.Info("backup scheduler started")
	return nil
}

// Stop halts the scheduling loop. It waits for the loop goroutine, which in
// turn waits for any in-flight backup run (bounded by maxScheduledRunDuration)
// so archives are never truncated by a graceful stop.
func (s *ScheduleService) Stop() {
	s.lifecycleMu.Lock()
	if !s.running {
		s.lifecycleMu.Unlock()
		return
	}
	close(s.stopCh)
	s.running = false
	s.lifecycleMu.Unlock()
	s.loopDone.Wait()
	slog.Info("backup scheduler stopped")
}

func (s *ScheduleService) runLoop(stopCh chan struct{}) {
	tick := s.TickInterval
	if tick <= 0 {
		tick = 30 * time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			s.safeCheck(context.Background())
		}
	}
}

// safeCheck runs one scheduler check with panic recovery so a panic in run
// logic cannot kill the loop goroutine.
func (s *ScheduleService) safeCheck(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("backup scheduler tick panicked — loop continues", "panic", fmt.Sprint(r))
		}
	}()
	s.CheckDue(ctx)
}

// CheckDue loads the persisted schedule and executes a run when it is due.
// Runs are serialized via runMu, so a manual run-now cannot overlap a tick run.
func (s *ScheduleService) CheckDue(ctx context.Context) {
	schedule, err := LoadSchedule(ctx, s.secrets)
	if err != nil {
		slog.Warn("backup scheduler: load schedule failed", "error", err)
		return
	}
	if schedule == nil || !schedule.Enabled {
		return // disabled or never configured — nothing to do
	}

	now := s.Now()

	// Bootstrap next_run on first enable.
	if schedule.NextRun == nil {
		next, err := NextRunTime(schedule.Interval, now)
		if err != nil {
			slog.Error("backup scheduler: invalid interval", "interval", schedule.Interval, "error", err)
			s.recordFailure(ctx, schedule, now, fmt.Errorf("invalid interval %q: %w", schedule.Interval, err))
			return
		}
		schedule.NextRun = &next
		if err := SaveSchedule(ctx, s.secrets, schedule); err != nil {
			slog.Warn("backup scheduler: persist next_run failed", "error", err)
		}
		return
	}

	if now.Before(*schedule.NextRun) {
		return // not due yet
	}

	s.RunNow(ctx, schedule)
}

// RunNow executes one backup run immediately (scheduled tick or manual
// trigger) and records last/next run status. Blocks until the run completes.
func (s *ScheduleService) RunNow(ctx context.Context, schedule *BackupSchedule) {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	runCtx, cancel := context.WithTimeout(ctx, maxScheduledRunDuration)
	defer cancel()

	start := s.Now()
	schedule.LastRun = &start
	schedule.LastStatus = "running"
	schedule.LastError = ""
	if err := persistScheduleState(runCtx, s.secrets, schedule); err != nil {
		slog.Warn("backup scheduler: persist running status failed", "error", err)
	}

	runErr := s.runOnce(runCtx, schedule)

	finish := s.Now()
	schedule.LastRun = &finish
	if runErr != nil {
		schedule.LastStatus = "error"
		schedule.LastError = runErr.Error()
		slog.Error("backup scheduler run failed", "error", runErr)
	} else {
		schedule.LastStatus = "ok"
		schedule.LastError = ""
		slog.Info("backup scheduler run completed")
	}
	next, err := NextRunTime(schedule.Interval, finish)
	if err != nil {
		// Should not happen — interval is validated on save. Keep a sane retry.
		next = finish.Add(time.Hour)
	}
	schedule.NextRun = &next
	if err := persistScheduleState(context.Background(), s.secrets, schedule); err != nil {
		slog.Warn("backup scheduler: persist run result failed", "error", err)
	}
}

// runOnce does the archive → upload → retention pipeline for one run.
func (s *ScheduleService) runOnce(ctx context.Context, schedule *BackupSchedule) error {
	s3Cfg, err := LoadS3Config(ctx, s.secrets)
	if err != nil {
		return fmt.Errorf("load s3 config: %w", err)
	}
	if s3Cfg == nil {
		return errors.New("s3 not configured — save S3 credentials before enabling the schedule")
	}
	client, err := s.NewClient(s3Cfg)
	if err != nil {
		return fmt.Errorf("s3 client: %w", err)
	}

	tmp, err := os.CreateTemp("", "goclaw-scheduled-backup-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	opts := Options{
		DSN:           s.deps.DSN,
		DataDir:       s.deps.DataDir,
		WorkspacePath: s.deps.WorkspacePath,
		OutputPath:    tmpPath,
		CreatedBy:     "scheduler",
		GoclawVersion: s.deps.GoclawVersion,
		SchemaVersion: s.deps.SchemaVersion,
	}
	if _, err := Run(ctx, opts); err != nil {
		return fmt.Errorf("backup run: %w", err)
	}

	key, err := uploadArchive(ctx, client, tmpPath, s.deps.GoclawVersion, s.Now())
	if err != nil {
		return err
	}

	if schedule.Retention > 0 {
		if err := enforceRetention(ctx, client, s3Cfg, schedule.Retention); err != nil {
			// Retention failure must not fail the whole run — the archive is
			// safely uploaded. Surface via last_error so the UI shows it.
			slog.Warn("backup scheduler: retention prune failed", "error", err)
			return fmt.Errorf("uploaded %s but retention prune failed: %w", key, err)
		}
	}
	return nil
}

// uploadArchive opens the produced archive and uploads it under a
// timestamped key, mirroring the manual upload naming scheme.
func uploadArchive(ctx context.Context, client S3ObjectStore, path, version string, now time.Time) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open backup file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat backup file: %w", err)
	}

	ts := now.UTC().Format("20060102-150405")
	key := fmt.Sprintf("backup-%s.tar.gz", ts)
	if version != "" {
		key = fmt.Sprintf("backup-%s-v%s.tar.gz", ts, version)
	}
	if err := client.Upload(ctx, key, f, info.Size()); err != nil {
		return "", err
	}
	return key, nil
}

// enforceRetention keeps the retention most recent backup objects under the
// configured prefix and deletes the rest.
//
// NOTE on key semantics: ListBackups returns FULL keys (prefix included) while
// Delete/Upload/Download take prefix-RELATIVE keys (they re-prepend the
// prefix via fullKey). The prefix must therefore be stripped from listed keys
// before deletion — passing the full key would produce "backups/backups/...".
func enforceRetention(ctx context.Context, client S3ObjectStore, cfg *S3Config, retention int) error {
	if retention <= 0 {
		return nil
	}
	entries, err := client.ListBackups(ctx)
	if err != nil {
		return fmt.Errorf("list s3 backups: %w", err)
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "backups/"
	}
	prefix = strings.TrimSuffix(prefix, "/") + "/"

	// Only prune keys under the current prefix (a prefix change must not
	// delete another prefix's history). entries are sorted newest-first by
	// ListBackups, so the OLDEST prunable objects sit at the tail — delete
	// from there, newest kept.
	var prunable []BackupEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Key, prefix) {
			prunable = append(prunable, e)
		}
	}
	excess := len(prunable) - retention
	for i := 1; i <= excess; i++ {
		e := prunable[len(prunable)-i] // oldest first
		key := strings.TrimPrefix(e.Key, prefix)
		if err := client.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete %q: %w", e.Key, err)
		}
	}
	if excess > 0 {
		slog.Info("backup scheduler: retention pruned old backups", "deleted", excess, "kept", retention)
	}
	return nil
}

// recordFailure persists a scheduler-level failure (bad interval etc.) and
// schedules a retry instead of hammering a broken schedule every tick.
func (s *ScheduleService) recordFailure(ctx context.Context, schedule *BackupSchedule, at time.Time, err error) {
	schedule.LastRun = &at
	schedule.LastStatus = "error"
	schedule.LastError = err.Error()
	retry := at.Add(time.Hour)
	schedule.NextRun = &retry
	if serr := persistScheduleState(ctx, s.secrets, schedule); serr != nil {
		slog.Warn("backup scheduler: persist failure status failed", "error", serr)
	}
}

// --- Interval math ---

// NextRunTime computes the next run time strictly after "after" for an
// interval spec. The spec is either a Go duration ("6h") or a 5-field cron
// expression ("0 3 * * *"). Empty defaults to DefaultScheduleInterval.
func NextRunTime(interval string, after time.Time) (time.Time, error) {
	spec := strings.TrimSpace(interval)
	if spec == "" {
		spec = DefaultScheduleInterval
	}
	if d, err := time.ParseDuration(spec); err == nil {
		if d < MinScheduleInterval {
			return time.Time{}, fmt.Errorf("interval %s is below the %s minimum", d, MinScheduleInterval)
		}
		return after.Add(d), nil
	}
	gx := gronx.New()
	if !gx.IsValid(spec) {
		return time.Time{}, fmt.Errorf("interval %q is neither a duration nor a valid cron expression", spec)
	}
	return gronx.NextTickAfter(spec, after, false)
}

// ValidateScheduleInterval checks an interval spec without computing a run.
func ValidateScheduleInterval(interval string) error {
	_, err := NextRunTime(interval, time.Now())
	return err
}

// --- Persistence (config_secrets store, same as backup.s3.*) ---

// LoadSchedule reads the schedule from the encrypted config_secrets store.
// Returns (nil, nil) when no schedule has been stored yet.
func LoadSchedule(ctx context.Context, secrets store.ConfigSecretsStore) (*BackupSchedule, error) {
	raw, err := secrets.Get(ctx, KeySchedule)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get %q: %w", KeySchedule, err)
	}
	if raw == "" {
		return nil, nil
	}
	var schedule BackupSchedule
	if err := json.Unmarshal([]byte(raw), &schedule); err != nil {
		return nil, fmt.Errorf("parse %q: %w", KeySchedule, err)
	}
	return &schedule, nil
}

// SaveSchedule validates and persists the schedule as one JSON blob — a
// single key keeps reads atomic (no partially-updated field sets).
func SaveSchedule(ctx context.Context, secrets store.ConfigSecretsStore, schedule *BackupSchedule) error {
	if schedule == nil {
		return errors.New("schedule is nil")
	}
	if schedule.Destination != "" && schedule.Destination != "s3" {
		return fmt.Errorf("unsupported destination %q (only \"s3\")", schedule.Destination)
	}
	if schedule.Retention < 0 {
		return errors.New("retention must be >= 0")
	}
	if schedule.Enabled {
		if err := ValidateScheduleInterval(schedule.Interval); err != nil {
			return err
		}
	}
	if schedule.Destination == "" {
		schedule.Destination = "s3"
	}
	if schedule.Interval == "" {
		schedule.Interval = DefaultScheduleInterval
	}
	return persistScheduleState(ctx, secrets, schedule)
}

// persistScheduleState writes the schedule JSON without validation. Used for
// run-state transitions (last/next run, status) where the config was already
// validated at set time — a hand-corrupted interval must still be recordable
// as errored, not silently dropped by validation.
func persistScheduleState(ctx context.Context, secrets store.ConfigSecretsStore, schedule *BackupSchedule) error {
	raw, err := json.Marshal(schedule)
	if err != nil {
		return fmt.Errorf("marshal schedule: %w", err)
	}
	if err := secrets.Set(ctx, KeySchedule, string(raw)); err != nil {
		return fmt.Errorf("set %q: %w", KeySchedule, err)
	}
	return nil
}
