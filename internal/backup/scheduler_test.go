package backup

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Fakes ---

// memSecrets is an in-memory ConfigSecretsStore. Missing keys return
// sql.ErrNoRows, matching the real pg/sqlitestore behavior LoadSchedule checks.
type memSecrets struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemSecrets() *memSecrets { return &memSecrets{data: map[string]string{}} }

func (m *memSecrets) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return "", sql.ErrNoRows
	}
	return v, nil
}

func (m *memSecrets) Set(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *memSecrets) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memSecrets) GetAll(_ context.Context) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.data))
	for k, v := range m.data {
		out[k] = v
	}
	return out, nil
}

// fakeS3 implements S3ObjectStore for retention/upload tests.
type fakeS3 struct {
	mu      sync.Mutex
	objects []BackupEntry // sorted newest-first (as ListBackups returns)
	prefix  string
	deleted []string // relative keys passed to Delete
	uploaded []string
	// failAt makes the client reject objects whose key contains the substring.
	failDeleteSubstr string
}

func (f *fakeS3) Upload(_ context.Context, key string, _ io.Reader, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploaded = append(f.uploaded, key)
	f.objects = append(f.objects, BackupEntry{
		Key:          f.prefix + key,
		LastModified: time.Now().Add(time.Duration(len(f.uploaded)) * time.Second),
	})
	f.sortLocked()
	return nil
}

func (f *fakeS3) ListBackups(_ context.Context) ([]BackupEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]BackupEntry, len(f.objects))
	copy(out, f.objects)
	return out, nil
}

func (f *fakeS3) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDeleteSubstr != "" && strings.Contains(key, f.failDeleteSubstr) {
		return errors.New("delete failed")
	}
	f.deleted = append(f.deleted, key)
	full := f.prefix + key
	for i := range f.objects {
		if f.objects[i].Key == full {
			f.objects = append(f.objects[:i], f.objects[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeS3) sortLocked() {
	sort.Slice(f.objects, func(i, j int) bool {
		return f.objects[i].LastModified.After(f.objects[j].LastModified)
	})
}

// seed adds pre-existing objects with increasing ages (oldest first arg order).
func (f *fakeS3) seed(keys ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	base := time.Now().Add(-24 * time.Hour)
	for i, k := range keys {
		f.objects = append(f.objects, BackupEntry{
			Key:          k,
			LastModified: base.Add(time.Duration(i) * time.Hour),
		})
	}
	f.sortLocked()
}

func (f *fakeS3) deletedKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

// --- NextRunTime: tick math ---

func TestNextRunTime_Duration(t *testing.T) {
	after := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	got, err := NextRunTime("6h", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := after.Add(6 * time.Hour); !got.Equal(want) {
		t.Fatalf("duration interval: got %v, want %v", got, want)
	}
}

func TestNextRunTime_DurationBelowMinimum(t *testing.T) {
	if _, err := NextRunTime("30m", time.Now()); err == nil {
		t.Fatal("expected error for interval below 1h minimum")
	}
}

func TestNextRunTime_CronDaily(t *testing.T) {
	// 10:00 UTC → next 03:00 is tomorrow.
	after := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	got, err := NextRunTime("0 3 * * *", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := time.Date(2026, 9, 19, 3, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("daily cron: got %v, want %v", got, want)
	}

	// 02:00 UTC → next 03:00 is same day.
	after = time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)
	got, err = NextRunTime("0 3 * * *", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := time.Date(2026, 9, 18, 3, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("daily cron same day: got %v, want %v", got, want)
	}
}

func TestNextRunTime_CronWeekly(t *testing.T) {
	// Friday 2026-09-18 12:00 UTC → next Monday (weekday 1) 03:00 is 2026-09-21.
	after := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	got, err := NextRunTime("0 3 * * 1", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := time.Date(2026, 9, 21, 3, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("weekly cron: got %v, want %v", got, want)
	}
}

func TestNextRunTime_EmptyDefaultsToDailyCron(t *testing.T) {
	after := time.Date(2026, 9, 18, 4, 0, 0, 0, time.UTC)
	got, err := NextRunTime("  ", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := time.Date(2026, 9, 19, 3, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("default interval: got %v, want %v", got, want)
	}
}

func TestNextRunTime_InvalidSpec(t *testing.T) {
	if _, err := NextRunTime("not-a-schedule", time.Now()); err == nil {
		t.Fatal("expected error for garbage interval")
	}
}

// --- Retention pruning ---

func TestEnforceRetention_PrunesOldestBeyondN(t *testing.T) {
	fake := &fakeS3{prefix: "backups/"}
	fake.seed(
		"backups/backup-old1.tar.gz",
		"backups/backup-old2.tar.gz",
		"backups/backup-mid.tar.gz",
		"backups/backup-new1.tar.gz",
		"backups/backup-new2.tar.gz",
	)
	cfg := &S3Config{Bucket: "b", Prefix: "backups/"}

	if err := enforceRetention(context.Background(), fake, cfg, 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deleted := fake.deletedKeys()
	if len(deleted) != 2 {
		t.Fatalf("expected 2 deletions, got %d: %v", len(deleted), deleted)
	}
	// Keys must be RELATIVE to the prefix (Delete re-prepends it).
	for _, k := range deleted {
		if strings.HasPrefix(k, "backups/") {
			t.Fatalf("Delete received full key %q — must be prefix-relative", k)
		}
	}
	if deleted[0] != "backup-old1.tar.gz" || deleted[1] != "backup-old2.tar.gz" {
		t.Fatalf("expected oldest two pruned, got %v", deleted)
	}
	remaining, _ := fake.ListBackups(context.Background())
	if len(remaining) != 3 {
		t.Fatalf("expected 3 remaining, got %d", len(remaining))
	}
}

func TestEnforceRetention_ZeroKeepsAll(t *testing.T) {
	fake := &fakeS3{prefix: "backups/"}
	fake.seed("backups/a.tar.gz", "backups/b.tar.gz")

	if err := enforceRetention(context.Background(), fake, &S3Config{Prefix: "backups/"}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fake.deletedKeys(); len(got) != 0 {
		t.Fatalf("retention 0 must not delete, deleted %v", got)
	}
}

func TestEnforceRetention_OnlyCurrentPrefix(t *testing.T) {
	fake := &fakeS3{prefix: "backups/"}
	fake.seed(
		"other/keep-me.tar.gz",
		"backups/backup-1.tar.gz",
		"backups/backup-2.tar.gz",
	)
	// Retention 1: only 1 of the 2 objects under backups/ may be deleted.
	if err := enforceRetention(context.Background(), fake, &S3Config{Prefix: "backups/"}, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deleted := fake.deletedKeys()
	if len(deleted) != 1 {
		t.Fatalf("expected exactly 1 deletion, got %v", deleted)
	}
	if !strings.HasPrefix(fake.prefix+deleted[0], "backups/") {
		t.Fatalf("deleted key outside prefix: %v", deleted)
	}
}

func TestEnforceRetention_DeleteFailurePropagates(t *testing.T) {
	fake := &fakeS3{prefix: "backups/", failDeleteSubstr: "old1"}
	fake.seed("backups/old1.tar.gz", "backups/new.tar.gz")

	err := enforceRetention(context.Background(), fake, &S3Config{Prefix: "backups/"}, 1)
	if err == nil {
		t.Fatal("expected delete error to propagate")
	}
}

func TestEnforceRetention_DefaultPrefixWhenEmpty(t *testing.T) {
	// cfg.Prefix empty → defaults to backups/ in prune scope.
	fake := &fakeS3{prefix: "backups/"}
	fake.seed("backups/1.tar.gz", "backups/2.tar.gz", "backups/3.tar.gz")
	if err := enforceRetention(context.Background(), fake, &S3Config{}, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fake.deletedKeys(); len(got) != 1 {
		t.Fatalf("expected 1 deletion with default prefix, got %v", got)
	}
}

// --- Schedule persistence ---

func TestSaveLoadSchedule_RoundTrip(t *testing.T) {
	secrets := newMemSecrets()
	ctx := context.Background()

	if s, err := LoadSchedule(ctx, secrets); err != nil || s != nil {
		t.Fatalf("expected (nil, nil) for missing schedule, got (%v, %v)", s, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	in := &BackupSchedule{
		Enabled:     true,
		Interval:    "0 3 * * *",
		Destination: "s3",
		Retention:   5,
		NextRun:     &now,
	}
	if err := SaveSchedule(ctx, secrets, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := LoadSchedule(ctx, secrets)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !out.Enabled || out.Interval != "0 3 * * *" || out.Retention != 5 || out.Destination != "s3" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if out.NextRun == nil || !out.NextRun.Equal(now) {
		t.Fatalf("next run mismatch: %v vs %v", out.NextRun, now)
	}
}

func TestSaveSchedule_DefaultsAndValidation(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name    string
		in      BackupSchedule
		wantErr bool
	}{
		{"disabled with empty interval gets defaults", BackupSchedule{}, false},
		{"enabled with valid cron", BackupSchedule{Enabled: true, Interval: "0 4 * * *"}, false},
		{"enabled with garbage interval", BackupSchedule{Enabled: true, Interval: "whenever"}, true},
		{"negative retention", BackupSchedule{Enabled: true, Interval: "6h", Retention: -1}, true},
		{"unsupported destination", BackupSchedule{Enabled: true, Interval: "6h", Destination: "gcs"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			secrets := newMemSecrets()
			in := tc.in
			err := SaveSchedule(ctx, secrets, &in)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	// Defaults applied on save.
	secrets := newMemSecrets()
	in := BackupSchedule{}
	if err := SaveSchedule(ctx, secrets, &in); err != nil {
		t.Fatalf("save: %v", err)
	}
	if in.Interval != DefaultScheduleInterval || in.Destination != "s3" {
		t.Fatalf("defaults not applied: %+v", in)
	}
}

// --- Scheduler behavior ---

func newTestService(t *testing.T, secrets *memSecrets, fake *fakeS3) *ScheduleService {
	t.Helper()
	return &ScheduleService{
		secrets: secrets,
		deps:    ScheduleDeps{}, // empty dirs + empty DSN → archive contains just the manifest
		Now:     time.Now,
		NewClient: func(cfg *S3Config) (S3ObjectStore, error) {
			return fake, nil
		},
		TickInterval: 10 * time.Millisecond,
	}
}

// seedS3Config writes minimal S3 credentials so LoadS3Config succeeds.
func seedS3Config(t *testing.T, secrets *memSecrets) {
	t.Helper()
	ctx := context.Background()
	for k, v := range map[string]string{
		S3KeyAccessKeyID:     "AKIATEST",
		S3KeySecretAccessKey: "secret",
		S3KeyBucket:          "bkt",
		S3KeyRegion:          "us-east-1",
		S3KeyPrefix:          "backups/",
	} {
		if err := secrets.Set(ctx, k, v); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}
}

func TestCheckDue_DisabledNoRun(t *testing.T) {
	secrets := newMemSecrets()
	fake := &fakeS3{prefix: "backups/"}
	svc := newTestService(t, secrets, fake)
	ctx := context.Background()

	if err := SaveSchedule(ctx, secrets, &BackupSchedule{Enabled: false, Interval: "1h"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Also persist a past-due next_run — disabled must still not run.
	s, _ := LoadSchedule(ctx, secrets)
	past := time.Now().Add(-2 * time.Hour)
	s.NextRun = &past
	_ = SaveSchedule(ctx, secrets, s)

	svc.CheckDue(ctx)

	if got := fake.deletedKeys(); len(got) != 0 {
		t.Fatalf("unexpected deletions: %v", got)
	}
	if len(fake.uploaded) != 0 {
		t.Fatalf("disabled schedule must not upload, uploaded %v", fake.uploaded)
	}
	after, err := LoadSchedule(ctx, secrets)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if after.LastStatus == "running" || after.LastRun != nil {
		t.Fatalf("disabled schedule recorded a run: %+v", after)
	}
}

func TestCheckDue_BootstrapNextRunWithoutRunning(t *testing.T) {
	secrets := newMemSecrets()
	fake := &fakeS3{prefix: "backups/"}
	svc := newTestService(t, secrets, fake)
	ctx := context.Background()

	if err := SaveSchedule(ctx, secrets, &BackupSchedule{Enabled: true, Interval: "2h"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	svc.CheckDue(ctx)

	if len(fake.uploaded) != 0 {
		t.Fatal("first tick must only bootstrap next_run, not run immediately")
	}
	after, err := LoadSchedule(ctx, secrets)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if after.NextRun == nil {
		t.Fatal("next_run not bootstrapped")
	}
	if after.NextRun.Before(time.Now()) {
		t.Fatalf("bootstrapped next_run in the past: %v", after.NextRun)
	}
}

func TestRunNow_FullRunAndRetention(t *testing.T) {
	secrets := newMemSecrets()
	seedS3Config(t, secrets)
	fake := &fakeS3{prefix: "backups/"}
	// Pre-existing cloud backups, oldest → newest.
	fake.seed(
		"backups/backup-old1.tar.gz",
		"backups/backup-old2.tar.gz",
		"backups/backup-old3.tar.gz",
	)
	svc := newTestService(t, secrets, fake)
	ctx := context.Background()

	if err := SaveSchedule(ctx, secrets, &BackupSchedule{Enabled: true, Interval: "1h", Retention: 2}); err != nil {
		t.Fatalf("save: %v", err)
	}
	schedule, _ := LoadSchedule(ctx, secrets)

	before := time.Now()
	svc.RunNow(ctx, schedule)

	if len(fake.uploaded) != 1 {
		t.Fatalf("expected 1 upload, got %v", fake.uploaded)
	}
	if !strings.HasPrefix(fake.uploaded[0], "backup-") || !strings.HasSuffix(fake.uploaded[0], ".tar.gz") {
		t.Fatalf("unexpected upload key %q", fake.uploaded[0])
	}

	// 3 pre-existing + 1 new = 4; retention 2 → 2 oldest pre-existing pruned.
	deleted := fake.deletedKeys()
	if len(deleted) != 2 {
		t.Fatalf("expected 2 retention deletions, got %v", deleted)
	}
	if deleted[0] != "backup-old1.tar.gz" || deleted[1] != "backup-old2.tar.gz" {
		t.Fatalf("wrong objects pruned: %v", deleted)
	}

	after, err := LoadSchedule(ctx, secrets)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if after.LastStatus != "ok" {
		t.Fatalf("expected ok status, got %q (err %q)", after.LastStatus, after.LastError)
	}
	if after.LastRun == nil || after.LastRun.Before(before) {
		t.Fatalf("last_run not recorded: %+v", after)
	}
	if after.NextRun == nil || !after.NextRun.After(before) {
		t.Fatalf("next_run not advanced: %+v", after)
	}
}

func TestRunNow_MissingS3ConfigFails(t *testing.T) {
	secrets := newMemSecrets() // no S3 credentials seeded
	fake := &fakeS3{prefix: "backups/"}
	svc := newTestService(t, secrets, fake)
	ctx := context.Background()

	if err := SaveSchedule(ctx, secrets, &BackupSchedule{Enabled: true, Interval: "1h"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	schedule, _ := LoadSchedule(ctx, secrets)

	svc.RunNow(ctx, schedule)

	after, err := LoadSchedule(ctx, secrets)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if after.LastStatus != "error" {
		t.Fatalf("expected error status, got %q", after.LastStatus)
	}
	if after.LastError == "" {
		t.Fatal("expected last_error to be recorded")
	}
	if len(fake.uploaded) != 0 {
		t.Fatal("nothing must be uploaded without S3 config")
	}
}

func TestCheckDue_InvalidIntervalRecordsFailureAndRetriesLater(t *testing.T) {
	secrets := newMemSecrets()
	fake := &fakeS3{prefix: "backups/"}
	svc := newTestService(t, secrets, fake)
	ctx := context.Background()

	// Persist an invalid schedule directly (bypassing SaveSchedule validation)
	// to simulate legacy/corrupt state with a nil next_run.
	raw := `{"enabled":true,"interval":"garbage","destination":"s3","retention":0}`
	if err := secrets.Set(ctx, KeySchedule, raw); err != nil {
		t.Fatalf("seed: %v", err)
	}

	before := time.Now()
	svc.CheckDue(ctx)

	if len(fake.uploaded) != 0 {
		t.Fatal("invalid interval must not run a backup")
	}
	after, err := LoadSchedule(ctx, secrets)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if after.LastStatus != "error" {
		t.Fatalf("expected recorded failure, got %q", after.LastStatus)
	}
	if after.NextRun == nil || !after.NextRun.After(before.Add(50*time.Minute)) {
		t.Fatalf("failure must schedule a distant retry, got %+v", after.NextRun)
	}
}

func TestStartStop_Idempotent(t *testing.T) {
	secrets := newMemSecrets()
	fake := &fakeS3{prefix: "backups/"}
	svc := newTestService(t, secrets, fake)

	if err := svc.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.Start(); err != nil {
		t.Fatalf("second start must be a no-op: %v", err)
	}
	svc.Stop()
	svc.Stop() // second stop must not panic
}

// The real backup.Run with empty deps must produce a loadable archive so the
// scheduled-run tests exercise the genuine code path (manifest-only tar.gz).
func TestRun_EmptyDepsProducesArchive(t *testing.T) {
	tmp, err := os.CreateTemp("", "sched-verify-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	if _, err := Run(context.Background(), Options{OutputPath: path, CreatedBy: "test"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("archive is empty")
	}
}
