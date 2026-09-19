package vworker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCleanupSweepProtectsActiveDirs pins the contract that a sweep never
// removes a protected work dir even when it is older than the TTL — the
// production race (a hung scene render keeps the dir old; the sweep deleted
// it mid-job) must stay impossible.
func TestCleanupSweepProtectsActiveDirs(t *testing.T) {
	work := t.TempDir()
	stale := time.Now().Add(-3 * time.Hour)
	mk := func(t *testing.T, name string) string {
		t.Helper()
		dir := filepath.Join(work, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(dir, stale, stale); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	orphan := mk(t, tempPrefix+"orphan")
	active := mk(t, tempPrefix+"active-job")

	CleanupSweep(work, time.Hour, active)

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphan dir survived sweep: %v", err)
	}
	if _, err := os.Stat(active); err != nil {
		t.Errorf("protected active dir was removed: %v", err)
	}
}

// Without protection the same stale dir is swept (orphan hygiene unchanged).
func TestCleanupSweepRemovesUnprotectedStale(t *testing.T) {
	work := t.TempDir()
	stale := time.Now().Add(-3 * time.Hour)
	dir := filepath.Join(work, tempPrefix+"old")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, stale, stale); err != nil {
		t.Fatal(err)
	}

	CleanupSweep(work, time.Hour)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("stale dir survived sweep: %v", err)
	}
}
