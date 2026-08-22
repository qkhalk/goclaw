package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// kitLockTestChecksum hashes a skill directory deterministically: relative
// paths and regular-file contents feed sha256, so any content change flips
// the value while identical trees stay byte-stable across runs.
func kitLockTestChecksum(dir string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		fmt.Fprintf(h, "%s\x00", rel)
		if info.Mode().IsRegular() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			h.Write(data)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func mustSum(t *testing.T, dir string) string {
	t.Helper()
	sum, err := kitLockTestChecksum(dir)
	if err != nil {
		t.Fatalf("checksum %s: %v", dir, err)
	}
	return sum
}

func TestKitLockSaveLoadByteStable(t *testing.T) {
	stamp := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	build := func() *LockFile {
		return &LockFile{
			Skills: []LockEntry{
				{Slug: "zeta", Version: 3, Checksum: "c3"},
				{Slug: "alpha", Version: 1, Checksum: "c1"},
				{Slug: "mid", Version: 2, Checksum: "c2"},
			},
			GeneratedAt: stamp,
		}
	}

	dir := t.TempDir()
	p1, p2, p3 := filepath.Join(dir, "a.lock"), filepath.Join(dir, "b.lock"), filepath.Join(dir, "c.lock")

	if err := SaveKitLock(p1, build()); err != nil {
		t.Fatalf("save p1: %v", err)
	}
	// Same logical state fed in a different order must serialize identically.
	reversed := build()
	for i, j := 0, len(reversed.Skills)-1; i < j; i, j = i+1, j-1 {
		reversed.Skills[i], reversed.Skills[j] = reversed.Skills[j], reversed.Skills[i]
	}
	if err := SaveKitLock(p2, reversed); err != nil {
		t.Fatalf("save p2: %v", err)
	}

	b1, err := os.ReadFile(p1)
	if err != nil {
		t.Fatalf("read p1: %v", err)
	}
	b2, err := os.ReadFile(p2)
	if err != nil {
		t.Fatalf("read p2: %v", err)
	}
	if string(b1) != string(b2) {
		t.Fatalf("saves differ:\np1=%s\np2=%s", b1, b2)
	}

	loaded, err := LoadKitLock(p1)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Skills) != 3 {
		t.Fatalf("loaded %d entries, want 3", len(loaded.Skills))
	}
	if loaded.Skills[0].Slug != "alpha" || loaded.Skills[1].Slug != "mid" || loaded.Skills[2].Slug != "zeta" {
		t.Fatalf("entries not slug-sorted: %v", loaded.Skills)
	}
	if !loaded.GeneratedAt.Equal(stamp) {
		t.Fatalf("GeneratedAt = %v, want %v", loaded.GeneratedAt, stamp)
	}
	// Load -> save must be byte-identical to the original file.
	if err := SaveKitLock(p3, loaded); err != nil {
		t.Fatalf("resave: %v", err)
	}
	b3, err := os.ReadFile(p3)
	if err != nil {
		t.Fatalf("read p3: %v", err)
	}
	if string(b1) != string(b3) {
		t.Fatalf("round trip not byte-stable:\norig=%s\nre=%s", b1, b3)
	}

	if _, err := LoadKitLock(filepath.Join(dir, "missing.lock")); err == nil {
		t.Fatal("LoadKitLock(missing) = nil error, want error")
	}
	if err := SaveKitLock(p1, nil); err == nil {
		t.Fatal("SaveKitLock(nil) = nil error, want error")
	}
}

func TestKitLockVerifyDriftKinds(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")

	mustDir := func(slug string) string {
		dir := filepath.Join(skillsDir, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", slug, err)
		}
		return dir
	}
	writeSkill := func(slug, body string) string {
		dir := mustDir(slug)
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", slug, err)
		}
		return dir
	}

	alpha := writeSkill("alpha", "alpha v1") // healthy
	writeSkill("beta", "beta v1")            // version drift (locked at 2)
	writeSkill("epsilon", "epsilon v1")      // checksum drift (stale locked sum)
	mustDir("delta")                         // on disk but left out of the lock
	// gamma: locked but never written -> missing

	lock := &LockFile{
		Skills: []LockEntry{
			{Slug: "epsilon", Version: 1, Checksum: "stale-sum"},
			{Slug: "gamma", Version: 1, Checksum: "whatever"},
			{Slug: "alpha", Version: 1, Checksum: mustSum(t, alpha)},
			{Slug: "beta", Version: 2, Checksum: "x"},
		},
	}

	got := VerifyKitLock(root, lock, kitLockTestChecksum)
	want := []string{
		"epsilon: checksum drift",
		"gamma: missing",
		"beta: version drift",
		"delta: untracked",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("drift lines =\n%q\nwant\n%q", got, want)
	}

	// Healthy lock over matching disk yields no drift.
	ok := &LockFile{
		GeneratedAt: time.Now().UTC(),
		Skills: []LockEntry{
			{Slug: "alpha", Version: 1, Checksum: mustSum(t, alpha)},
			{Slug: "beta", Version: 1, Checksum: mustSum(t, filepath.Join(skillsDir, "beta"))},
			{Slug: "delta", Version: 1, Checksum: mustSum(t, filepath.Join(skillsDir, "delta"))},
			{Slug: "epsilon", Version: 1, Checksum: mustSum(t, filepath.Join(skillsDir, "epsilon"))},
		},
	}
	if drift := VerifyKitLock(root, ok, kitLockTestChecksum); len(drift) != 0 {
		t.Fatalf("healthy tree reported drift: %q", drift)
	}
}

func TestKitLockEmptyLock(t *testing.T) {
	root := t.TempDir()
	lock := &LockFile{GeneratedAt: time.Now().UTC()}
	if drift := VerifyKitLock(root, lock, kitLockTestChecksum); drift != nil {
		t.Fatalf("empty lock vs empty root = %q, want nil", drift)
	}
	if drift := VerifyKitLock(root, nil, kitLockTestChecksum); drift != nil {
		t.Fatalf("nil lock = %q, want nil", drift)
	}

	// An empty lock still flags anything on disk as untracked.
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "rogue"), 0o755); err != nil {
		t.Fatalf("mkdir rogue: %v", err)
	}
	drift := VerifyKitLock(root, lock, kitLockTestChecksum)
	if len(drift) != 1 || drift[0] != "rogue: untracked" {
		t.Fatalf("empty lock drift = %q, want [rogue: untracked]", drift)
	}

	path := filepath.Join(t.TempDir(), ".goclaw", "kit.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir .goclaw: %v", err)
	}
	if err := SaveKitLock(path, lock); err != nil {
		t.Fatalf("save empty lock: %v", err)
	}
	got, err := LoadKitLock(path)
	if err != nil {
		t.Fatalf("load empty lock: %v", err)
	}
	if len(got.Skills) != 0 {
		t.Fatalf("loaded %d entries, want 0", len(got.Skills))
	}
}
