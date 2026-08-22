package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// LockEntry records one skill pinned in .goclaw/kit.lock: its slug, the
// integer kit version it was installed from, and a content checksum.
type LockEntry struct {
	Slug     string `json:"slug"`
	Version  int    `json:"version"`
	Checksum string `json:"checksum"`
}

// LockFile is the on-disk representation of .goclaw/kit.lock. Entries are
// kept sorted by slug so that saves are byte-stable across runs.
type LockFile struct {
	Skills      []LockEntry `json:"skills"`
	GeneratedAt time.Time   `json:"generated_at"`
}

// LoadKitLock reads and decodes .goclaw/kit.lock at path. Entries are
// normalized to slug order regardless of how the file was written.
func LoadKitLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock LockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse kit lock %s: %w", path, err)
	}
	sortLockEntries(lock.Skills)
	return &lock, nil
}

// SaveKitLock writes lock to path as indented JSON with entries sorted by
// slug, so repeated saves of identical state are byte-identical. The file is
// written to a sibling temp file and renamed into place.
func SaveKitLock(path string, lock *LockFile) error {
	if lock == nil {
		return fmt.Errorf("save kit lock %s: nil lock", path)
	}
	sortLockEntries(lock.Skills)
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".kit-lock-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// sortLockEntries orders entries by slug in place.
func sortLockEntries(entries []LockEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
}

// VerifyKitLock compares lock against the skills installed under root
// (.goclaw's parent). For each entry, the skill directory root/skills/<slug>
// is checksummed via checksum and compared with the locked value; version
// mismatches, missing directories, and skills present on disk but absent from
// the lock are reported. Returns one line per problem, entries first then
// untracked skills sorted by slug:
//
//	<slug>: checksum drift
//	<slug>: version drift
//	<slug>: missing
//	<slug>: untracked
func VerifyKitLock(root string, lock *LockFile, checksum func(string) (string, error)) []string {
	var drift []string
	if lock == nil {
		return drift
	}
	skillsDir := filepath.Join(root, "skills")
	for _, entry := range lock.Skills {
		dir := filepath.Join(skillsDir, entry.Slug)
		info, err := os.Stat(dir)
		switch {
		case err != nil || !info.IsDir():
			drift = append(drift, fmt.Sprintf("%s: missing", entry.Slug))
			continue
		case entry.Version != 1:
			drift = append(drift, fmt.Sprintf("%s: version drift", entry.Slug))
			continue
		}
		sum, sumErr := checksum(dir)
		if sumErr != nil {
			drift = append(drift, fmt.Sprintf("%s: checksum error: %v", entry.Slug, sumErr))
			continue
		}
		if sum != entry.Checksum {
			drift = append(drift, fmt.Sprintf("%s: checksum drift", entry.Slug))
			continue
		}
	}

	// Any skill directory present on disk but absent from the lock is untracked.
	names, err := os.ReadDir(skillsDir)
	if err == nil {
		var untracked []string
		for _, name := range names {
			if !name.IsDir() || isLocked(name.Name(), lock.Skills) {
				continue
			}
			untracked = append(untracked, name.Name())
		}
		sort.Strings(untracked)
		for _, slug := range untracked {
			drift = append(drift, fmt.Sprintf("%s: untracked", slug))
		}
	}
	return drift
}

// isLocked reports whether slug appears among entries (even drifted ones),
// so a locked-but-broken skill is never double-reported as untracked.
func isLocked(slug string, entries []LockEntry) bool {
	for _, e := range entries {
		if e.Slug == slug {
			return true
		}
	}
	return false
}
