package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// KitManifest is the on-disk kit descriptor (kit.yaml). The checksum field is
// optional: when present, VerifyChecksum compares the computed checksum against
// it; when absent, verification is skipped and the manifest is reported as
// verified. Checksums are never authored by hand — they change whenever a skill
// is added or edited, so the value is computed at runtime.
type KitManifest struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Description string   `yaml:"description"`
	Skills      []string `yaml:"skills"`
	Checksum    string   `yaml:"checksum,omitempty"`
}

// KitInfo is a point-in-time snapshot of a kit used for inspect/report output.
type KitInfo struct {
	Name       string
	Version    string
	Skills     []string
	Checksum   string
	Verified   bool
	SkillCount int
}

// KitManager reads a kit manifest from disk and computes its checksum from the
// underlying skill files. It never writes to the kit directory.
//
// Layout expectations:
//   - manifestPath points at <kitDir>/kit.yaml.
//   - skillsRoot points at the directory that contains one folder per skill,
//     each holding a SKILL.md (e.g. <repo>/skills/). Slugs listed in the
//     manifest resolve to <skillsRoot>/<slug>/SKILL.md.
//
// An empty skillsRoot yields an empty skill list (ListSkills returns nothing
// and ComputeChecksum hashes an empty set), so a manifest-only kit is still
// representable.
type KitManager struct {
	manifestPath string
	skillsRoot   string

	// managedDir is the versioned install destination (<data>/skills-store)
	// used by InstallFrom/Update/Rollback. Zero value disables them.
	managedDir string
}

// NewKitManager creates a KitManager.
// manifestPath: absolute path to the kit's kit.yaml file.
// skillsRoot: absolute path to the directory containing per-skill folders.
func NewKitManager(manifestPath, skillsRoot string) *KitManager {
	return &KitManager{
		manifestPath: manifestPath,
		skillsRoot:   skillsRoot,
	}
}

// SetManagedDir points the manager at the versioned skills-store directory
// that InstallFrom, Update, and Rollback write into.
func (k *KitManager) SetManagedDir(dir string) { k.managedDir = dir }

// Load reads and parses the kit manifest. Returns an error when the file is
// missing or malformed.
func (k *KitManager) Load(ctx context.Context) (*KitManifest, error) {
	raw, err := os.ReadFile(k.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("kit: read manifest %s: %w", k.manifestPath, err)
	}
	var m KitManifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("kit: parse manifest %s: %w", k.manifestPath, err)
	}
	if strings.TrimSpace(m.Name) == "" {
		return nil, fmt.Errorf("kit: manifest %s missing required field name", k.manifestPath)
	}
	if strings.TrimSpace(m.Version) == "" {
		return nil, fmt.Errorf("kit: manifest %s missing required field version", k.manifestPath)
	}
	return &m, nil
}

// ListSkills returns the slugs from the manifest whose SKILL.md actually exists
// under the skills root. Slugs listed in the manifest but missing on disk are
// skipped; a missing skills root yields an empty list. Results are sorted for
// deterministic output.
func (k *KitManager) ListSkills(ctx context.Context) []string {
	m, err := k.Load(ctx)
	if err != nil || k.skillsRoot == "" {
		return nil
	}
	var found []string
	for _, slug := range m.Skills {
		if _, err := os.Stat(filepath.Join(k.skillsRoot, slug, "SKILL.md")); err == nil {
			found = append(found, slug)
		}
	}
	sort.Strings(found)
	return found
}

// Version returns the kit version from the manifest, or "" when the manifest
// cannot be loaded.
func (k *KitManager) Version() string {
	m, err := k.Load(context.Background())
	if err != nil {
		return ""
	}
	return m.Version
}

// ComputeChecksum returns a SHA-256 over the kit's skill content. The input is
// deterministic: for each skill slug (sorted) the hash covers the slug name and
// the raw bytes of its SKILL.md. Skill slugs without a SKILL.md on disk are
// ignored, so the checksum only changes when actually available content
// changes.
func (k *KitManager) ComputeChecksum(ctx context.Context) (string, error) {
	slugs := k.ListSkills(ctx)
	h := sha256.New()
	for _, slug := range slugs {
		content, err := os.ReadFile(filepath.Join(k.skillsRoot, slug, "SKILL.md"))
		if err != nil {
			return "", fmt.Errorf("kit: read skill %s: %w", slug, err)
		}
		// Length-prefix framing keeps slug/content boundaries unambiguous even
		// if one skill's content happens to be byte-identical to another's.
		if _, err := fmt.Fprintf(h, "%d:%s\n", len(slug), slug); err != nil {
			return "", fmt.Errorf("kit: hash slug %s: %w", slug, err)
		}
		if _, err := h.Write(content); err != nil {
			return "", fmt.Errorf("kit: hash skill %s: %w", slug, err)
		}
		if _, err := h.Write([]byte{0}); err != nil {
			return "", fmt.Errorf("kit: hash skill %s: %w", slug, err)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyChecksum compares the runtime-computed checksum against the value
// pinned in the manifest. When the manifest has no checksum field the
// verification is skipped: it returns (true, nil) so a manifest without a pin
// is reported as verified rather than failed. A mismatch returns (false, nil).
func (k *KitManager) VerifyChecksum(ctx context.Context) (bool, error) {
	m, err := k.Load(ctx)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(m.Checksum) == "" {
		return true, nil
	}
	actual, err := k.ComputeChecksum(ctx)
	if err != nil {
		return false, err
	}
	return actual == m.Checksum, nil
}

// RenderedManifest returns a YAML-ish text rendering of the current manifest
// and checksum state, used for inspect and dry-run output. It does not modify
// the on-disk manifest.
func (k *KitManager) RenderedManifest() string {
	m, err := k.Load(context.Background())
	if err != nil {
		return fmt.Sprintf("name: %s\nstatus: unreadable (%v)\n", filepath.Base(k.manifestPath), err)
	}
	checksum, csErr := k.ComputeChecksum(context.Background())
	csLine := "checksum: <unavailable>"
	if csErr == nil {
		csLine = "checksum: " + checksum
	}
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\n", m.Name)
	fmt.Fprintf(&b, "version: %s\n", m.Version)
	fmt.Fprintf(&b, "description: %s\n", m.Description)
	fmt.Fprintf(&b, "%s\n", csLine)
	fmt.Fprintf(&b, "skills:\n")
	for _, slug := range m.Skills {
		exists := ""
		if k.skillsRoot != "" {
			if _, err := os.Stat(filepath.Join(k.skillsRoot, slug, "SKILL.md")); err != nil {
				exists = " # missing on disk"
			}
		}
		fmt.Fprintf(&b, "  - %s%s\n", slug, exists)
	}
	return b.String()
}

// Inspect assembles a KitInfo snapshot: manifest metadata, the skills that
// actually exist on disk, the computed checksum, and whether it verifies.
func (k *KitManager) Inspect(ctx context.Context) (*KitInfo, error) {
	m, err := k.Load(ctx)
	if err != nil {
		return nil, err
	}
	checksum, err := k.ComputeChecksum(ctx)
	if err != nil {
		return nil, err
	}
	verified, err := k.VerifyChecksum(ctx)
	if err != nil {
		return nil, err
	}
	skills := k.ListSkills(ctx)
	return &KitInfo{
		Name:       m.Name,
		Version:    m.Version,
		Skills:     skills,
		Checksum:   checksum,
		Verified:   verified,
		SkillCount: len(skills),
	}, nil
}

// --- Kit installation -------------------------------------------------------
// Managed layout: <managedDir>/<slug>/<version>/ with integer versions
// starting at 1. The lockfile lives at <parent-of-managedDir>/.goclaw/kit.lock,
// the root convention VerifyKitLock expects.

// kitLockPath returns the .goclaw/kit.lock path for this manager's managed dir.
func (k *KitManager) lockPath() string {
	return filepath.Join(filepath.Dir(k.managedDir), ".goclaw", "kit.lock")
}

// nextVersion returns one past the highest integer version directory present
// for slug under managedDir; 1 when nothing is installed yet.
func (k *KitManager) nextVersion(slug string) int {
	v, _ := latestManagedVersion(k.managedDir, slug)
	if v < 1 {
		return 1 // nothing valid installed yet
	}
	return v + 1
}

// InstallOpts tunes InstallFrom. DryRun computes and returns the plan without
// writing anything.
type InstallOpts struct {
	DryRun bool
}

// InstallFrom resolves the manifest's skill closure against the source tree,
// copies each planned skill into a fresh versioned directory under the
// managed dir, and persists .goclaw/kit.lock. With opts.DryRun it returns the
// plan only: no directories are created, no lock is written.
//
// The lookup reads SKILL.md from <skillsRoot>/<slug> and parses its
// frontmatter version.

// lookup builds a deps.Resolve lookup over the source tree: it returns the
// SKILL.md content and frontmatter version for slug, ErrSkillInstalled for
// slugs already installed under managedDir, and ErrSkillNotFound otherwise.
func (k *KitManager) lookup(root string) func(string) (string, string, error) {
	return func(slug string) (string, string, error) {
		if v, _ := latestManagedVersion(k.managedDir, slug); v >= 1 {
			return "", "", ErrSkillInstalled
		}
		path := filepath.Join(root, slug, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			return "", "", ErrSkillNotFound
		}
		content := string(data)
		version := ""
		if meta := parseMetadata(path); meta != nil {
			version = meta.Version
		}
		return content, version, nil
	}
}

func (k *KitManager) InstallFrom(ctx context.Context, manifest KitManifest, opts InstallOpts) (*Plan, error) {
	if k.managedDir == "" {
		return nil, errors.New("kit install: SetManagedDir not called")
	}
	if len(manifest.Skills) == 0 {
		return nil, errors.New("kit install: manifest lists no skills")
	}
	if k.skillsRoot == "" {
		return nil, errors.New("kit install: no source root (skillsRoot not set)")
	}
	plan, err := Resolve(manifest.Skills, k.lookup(k.skillsRoot))
	if err != nil {
		return nil, err
	}
	if len(plan.Missing) > 0 || len(plan.Conflicts) > 0 || len(plan.Cycles) > 0 {
		return &plan, fmt.Errorf("kit install: unresolvable closure (missing=%d conflicts=%d cycles=%d)",
			len(plan.Missing), len(plan.Conflicts), len(plan.Cycles))
	}
	if opts.DryRun {
		return &plan, nil
	}
	lock, err := LoadKitLock(k.lockPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if lock == nil {
		lock = &LockFile{}
	}
	for _, slug := range plan.Install {
		version := k.nextVersion(slug)
		dst := filepath.Join(k.managedDir, slug, strconv.Itoa(version))
		if err := CopyDir(filepath.Join(k.skillsRoot, slug), dst); err != nil {
			return nil, fmt.Errorf("kit install: copy %s: %w", slug, err)
		}
		sum, _, err := HashDir(dst)
		if err != nil {
			return nil, fmt.Errorf("kit install: hash %s: %w", slug, err)
		}
		lock.Skills = append(lock.Skills, LockEntry{Slug: slug, Version: version, Checksum: sum})
	}
	lock.GeneratedAt = time.Now()
	if err := os.MkdirAll(filepath.Dir(k.lockPath()), 0o755); err != nil {
		return nil, fmt.Errorf("kit install: create %s: %w", filepath.Dir(k.lockPath()), err)
	}
	if err := SaveKitLock(k.lockPath(), lock); err != nil {
		return nil, fmt.Errorf("kit install: save lock: %w", err)
	}
	return &plan, nil
}

// Update installs slug from the source tree when its frontmatter version is
// newer than the version recorded in the lockfile (compareVersions), leaving
// the lock entry untouched otherwise. It reports an error for unknown or
// untracked skills.
func (k *KitManager) Update(ctx context.Context, slug string) error {
	lock, err := LoadKitLock(k.lockPath())
	if err != nil {
		return fmt.Errorf("kit update %s: load lock: %w", slug, err)
	}
	var entry *LockEntry
	for i := range lock.Skills {
		if lock.Skills[i].Slug == slug {
			entry = &lock.Skills[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("kit update %s: not tracked in kit.lock", slug)
	}
	srcPath := filepath.Join(k.skillsRoot, slug, "SKILL.md")
	meta := parseMetadata(srcPath)
	if meta == nil || meta.Version == "" {
		return fmt.Errorf("kit update %s: source has no parsable version", slug)
	}
	cur, _ := latestManagedVersion(k.managedDir, slug)
	curVersion := strconv.Itoa(cur)
	if cur >= 1 && compareVersions(meta.Version, curVersion) <= 0 {
		return nil // already up to date
	}
	version := k.nextVersion(slug)
	dst := filepath.Join(k.managedDir, slug, strconv.Itoa(version))
	if err := CopyDir(filepath.Join(k.skillsRoot, slug), dst); err != nil {
		return fmt.Errorf("kit update %s: copy: %w", slug, err)
	}
	sum, _, err := HashDir(dst)
	if err != nil {
		return fmt.Errorf("kit update %s: hash: %w", slug, err)
	}
	entry.Version = version
	entry.Checksum = sum
	lock.GeneratedAt = time.Now()
	if err := os.MkdirAll(filepath.Dir(k.lockPath()), 0o755); err != nil {
		return fmt.Errorf("kit update %s: create %s: %w", slug, filepath.Dir(k.lockPath()), err)
	}
	if err := SaveKitLock(k.lockPath(), lock); err != nil {
		return fmt.Errorf("kit update %s: save lock: %w", slug, err)
	}
	return nil
}

// Rollback restores slug to the content of its existing managed directory
// <managedDir>/<slug>/<toVersion> by copying it into a fresh next-version
// directory and repointing the lock entry. The original version directories
// are never modified.
func (k *KitManager) Rollback(slug string, toVersion int) error {
	if toVersion < 1 {
		return fmt.Errorf("kit rollback %s: invalid version %d", slug, toVersion)
	}
	src := filepath.Join(k.managedDir, slug, strconv.Itoa(toVersion))
	if info, err := os.Stat(src); err != nil || !info.IsDir() {
		return fmt.Errorf("kit rollback %s: version %d not installed", slug, toVersion)
	}
	lock, err := LoadKitLock(k.lockPath())
	if err != nil {
		return fmt.Errorf("kit rollback %s: load lock: %w", slug, err)
	}
	var entry *LockEntry
	for i := range lock.Skills {
		if lock.Skills[i].Slug == slug {
			entry = &lock.Skills[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("kit rollback %s: not tracked in kit.lock", slug)
	}
	version := k.nextVersion(slug)
	dst := filepath.Join(k.managedDir, slug, strconv.Itoa(version))
	if err := CopyDir(src, dst); err != nil {
		return fmt.Errorf("kit rollback %s: copy: %w", slug, err)
	}
	sum, _, err := HashDir(dst)
	if err != nil {
		return fmt.Errorf("kit rollback %s: hash: %w", slug, err)
	}
	entry.Version = version
	entry.Checksum = sum
	lock.GeneratedAt = time.Now()
	if err := os.MkdirAll(filepath.Dir(k.lockPath()), 0o755); err != nil {
		return fmt.Errorf("kit rollback %s: create %s: %w", slug, filepath.Dir(k.lockPath()), err)
	}
	if err := SaveKitLock(k.lockPath(), lock); err != nil {
		return fmt.Errorf("kit rollback %s: save lock: %w", slug, err)
	}
	return nil
}
