package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// kitInstallFixture builds a kit source tree with the given slug → SKILL.md
// bodies plus a manifest listing those slugs, and a KitManager pointed at it
// with a fresh (not yet created) managed dir. The lock path is therefore
// <tmp>/data/.goclaw/kit.lock.
func kitInstallFixture(t *testing.T, skills map[string]string) (mgr *KitManager, manifest KitManifest, managedDir string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "src")
	for slug, body := range skills {
		writeKitSkillFile(t, filepath.Join(root, slug, "SKILL.md"), body)
	}
	slugs := make([]string, 0, len(skills))
	for slug := range skills {
		slugs = append(slugs, slug)
	}
	manifest = KitManifest{Name: "test-kit", Version: "1.0.0", Skills: slugs}
	managedDir = filepath.Join(t.TempDir(), "data", "skills-store")
	mgr = NewKitManager(filepath.Join(root, "kit.yaml"), root)
	mgr.SetManagedDir(managedDir)
	return mgr, manifest, managedDir
}

// writeKitSkillFile creates parent directories and writes content.
func writeKitSkillFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// kitLockEntry returns the lock entry for slug, failing when absent.
func kitLockEntry(t *testing.T, lockPath, slug string) LockEntry {
	t.Helper()
	lock, err := LoadKitLock(lockPath)
	if err != nil {
		t.Fatalf("LoadKitLock: %v", err)
	}
	for _, e := range lock.Skills {
		if e.Slug == slug {
			return e
		}
	}
	t.Fatalf("kit.lock has no entry for %q", slug)
	return LockEntry{}
}

func TestInstallFromDryRunWritesNothing(t *testing.T) {
	mgr, _, managedDir := kitInstallFixture(t, map[string]string{
		"app": "---\nname: app\nversion: 2.0\ndepends:\n  - lib@>=1.0\n---\n\n# App\n",
		"lib": "---\nname: lib\nversion: 1.4.2\n---\n\n# Lib\n",
	})
	ctx := context.Background()

	// Requesting an unknown slug must surface it as missing, not silently
	// drop it.
	bad := KitManifest{Name: "test-kit", Version: "1.0.0", Skills: []string{"app", "gauge"}}
	if plan, err := mgr.InstallFrom(ctx, bad, InstallOpts{DryRun: true}); err == nil {
		t.Fatalf("dry run with unknown slug returned no error; plan = %+v", plan)
	}

	plan, err := mgr.InstallFrom(ctx,
		KitManifest{Name: "test-kit", Version: "1.0.0", Skills: []string{"app"}}, InstallOpts{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(plan.Install) != 2 || plan.Install[0] != "lib" || plan.Install[1] != "app" {
		t.Fatalf("plan.Install = %v, want [lib app] (dependency first)", plan.Install)
	}

	if entries, err := os.ReadDir(managedDir); !os.IsNotExist(err) && len(entries) > 0 {
		t.Fatalf("dry run wrote under managed dir: %v", entries)
	}
	if _, err := os.Stat(mgr.lockPath()); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote kit.lock: %v", err)
	}
}

func TestInstallFromWritesDirsAndLock(t *testing.T) {
	mgr, manifest, managedDir := kitInstallFixture(t, map[string]string{
		"app": "---\nname: app\nversion: 1.0\ndepends:\n  - lib\n---\n\n# App\n",
		"lib": "---\nname: lib\nversion: 1.0\n---\n\n# Lib\n",
	})
	ctx := context.Background()
	lockPath := mgr.lockPath()

	plan, err := mgr.InstallFrom(ctx, manifest, InstallOpts{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(plan.Install) != 2 {
		t.Fatalf("plan.Install = %v, want both slugs", plan.Install)
	}

	appEntry := kitLockEntry(t, lockPath, "app")
	libEntry := kitLockEntry(t, lockPath, "lib")
	if appEntry.Version != 1 || libEntry.Version != 1 {
		t.Fatalf("versions = (%d, %d), want (1, 1)", appEntry.Version, libEntry.Version)
	}

	sumApp, _, err := HashDir(filepath.Join(managedDir, "app", "1"))
	if err != nil {
		t.Fatalf("HashDir app: %v", err)
	}
	if appEntry.Checksum != sumApp {
		t.Fatal("lock checksum does not match installed app content")
	}
	body, err := os.ReadFile(filepath.Join(managedDir, "lib", "1", "SKILL.md"))
	if err != nil {
		t.Fatalf("installed SKILL.md unreadable: %v", err)
	}
	if string(body) != "---\nname: lib\nversion: 1.0\n---\n\n# Lib\n" {
		t.Fatalf("installed content drifted: %q", body)
	}

	// Re-install over an existing install: existing slugs are AlreadySatisfied
	// and nothing is re-copied — versions stay at 1.
	plan2, err := mgr.InstallFrom(ctx, manifest, InstallOpts{})
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if len(plan2.AlreadySatisfied) != 2 || len(plan2.Install) != 0 {
		t.Fatalf("reinstall plan = %+v, want all AlreadySatisfied", plan2)
	}
	if e := kitLockEntry(t, lockPath, "lib"); e.Version != 1 {
		t.Fatalf("reinstall bumped version to %d, want 1", e.Version)
	}
}

func TestUpdateBumpsVersionAndLock(t *testing.T) {
	mgr, manifest, managedDir := kitInstallFixture(t, map[string]string{
		"widget": "---\nname: widget\nversion: 1.0\n---\n\n# Widget v1\n",
	})
	ctx := context.Background()
	lockPath := mgr.lockPath()

	if _, err := mgr.InstallFrom(ctx, manifest, InstallOpts{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	before := kitLockEntry(t, lockPath, "widget")

	// Same source version → no-op.
	if err := mgr.Update(ctx, "widget"); err != nil {
		t.Fatalf("no-op update: %v", err)
	}
	if e := kitLockEntry(t, lockPath, "widget"); e.Version != before.Version || e.Checksum != before.Checksum {
		t.Fatal("up-to-date update mutated the lock entry")
	}

	// Bump the source version and update.
	writeKitSkillFile(t, filepath.Join(mgr.skillsRoot, "widget", "SKILL.md"),
		"---\nname: widget\nversion: 1.1\n---\n\n# Widget v2\n")
	if err := mgr.Update(ctx, "widget"); err != nil {
		t.Fatalf("update: %v", err)
	}
	after := kitLockEntry(t, lockPath, "widget")
	if after.Version != before.Version+1 {
		t.Fatalf("version = %d, want %d", after.Version, before.Version+1)
	}
	if after.Checksum == before.Checksum {
		t.Fatal("checksum unchanged after content bump")
	}
	got, err := os.ReadFile(filepath.Join(managedDir, "widget", "2", "SKILL.md"))
	if err != nil {
		t.Fatalf("read version dir 2: %v", err)
	}
	if string(got) != "---\nname: widget\nversion: 1.1\n---\n\n# Widget v2\n" {
		t.Fatalf("version dir 2 content = %q", got)
	}
}

func TestRollbackRestoresContentAndLock(t *testing.T) {
	mgr, manifest, managedDir := kitInstallFixture(t, map[string]string{
		"tool": "---\nname: tool\nversion: 1.0\n---\n\n# Tool v1\n",
	})
	ctx := context.Background()
	lockPath := mgr.lockPath()

	if _, err := mgr.InstallFrom(ctx, manifest, InstallOpts{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	v1Body, err := os.ReadFile(filepath.Join(managedDir, "tool", "1", "SKILL.md"))
	if err != nil {
		t.Fatalf("read v1: %v", err)
	}

	// Update twice so there is history to roll back through.
	writeKitSkillFile(t, filepath.Join(mgr.skillsRoot, "tool", "SKILL.md"),
		"---\nname: tool\nversion: 2.0\n---\n\n# Tool v2\n")
	if err := mgr.Update(ctx, "tool"); err != nil {
		t.Fatalf("update: %v", err)
	}
	writeKitSkillFile(t, filepath.Join(mgr.skillsRoot, "tool", "SKILL.md"),
		"---\nname: tool\nversion: 3.0\n---\n\n# Tool v3\n")
	if err := mgr.Update(ctx, "tool"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if cur := kitLockEntry(t, lockPath, "tool"); cur.Version != 3 {
		t.Fatalf("current version = %d, want 3", cur.Version)
	}

	// Rollback to v1: new version dir 4 carries v1's content; dirs 1–3 untouched.
	if err := mgr.Rollback("tool", 1); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	entry := kitLockEntry(t, lockPath, "tool")
	if entry.Version != 4 {
		t.Fatalf("post-rollback version = %d, want 4", entry.Version)
	}
	rolled, err := os.ReadFile(filepath.Join(managedDir, "tool", "4", "SKILL.md"))
	if err != nil {
		t.Fatalf("read rolled-back dir: %v", err)
	}
	if string(rolled) != string(v1Body) {
		t.Fatalf("rolled-back content = %q, want original %q", rolled, v1Body)
	}
	for _, keep := range []string{"1", "2", "3"} {
		if _, err := os.Stat(filepath.Join(managedDir, "tool", keep)); err != nil {
			t.Fatalf("history dir %s vanished: %v", keep, err)
		}
	}
	wantSum, _, err := HashDir(filepath.Join(managedDir, "tool", "4"))
	if err != nil {
		t.Fatalf("HashDir rolled-back dir: %v", err)
	}
	if entry.Checksum != wantSum {
		t.Fatal("lock checksum does not match rolled-back content")
	}

	// Unknown target version errors without touching anything.
	if err := mgr.Rollback("tool", 99); err == nil {
		t.Fatal("rollback to unknown version succeeded")
	}
	if e := kitLockEntry(t, lockPath, "tool"); e.Version != 4 {
		t.Fatalf("failed rollback mutated lock: version = %d", e.Version)
	}
}

// managedDirOf returns the manager's managed dir (test visibility helper).
func managedDirOf(mgr *KitManager) string {
	return mgr.managedDir
}
