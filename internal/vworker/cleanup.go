package vworker

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// tempPrefix is the prefix for worker temp directories.
	tempPrefix = "vwjob-"
	// defaultTTL is the default TTL for orphaned temp directories.
	defaultTTL = 2 * time.Hour
)

// CleanupSweep removes orphaned temp directories older than ttl.
// Called at startup and periodically. Safe to call concurrently.
// Protected dirs (typically the in-flight jobs' work dirs, from
// Runner.ActiveWorkDirs) are never removed — an age-based sweep alone
// deletes a long-running job's working set out from under it.
func CleanupSweep(workDir string, ttl time.Duration, protected ...string) {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	keep := make(map[string]bool, len(protected))
	for _, p := range protected {
		keep[p] = true
	}

	entries, err := os.ReadDir(workDir)
	if err != nil {
		slog.Warn("cleanup: read work dir failed", "dir", workDir, "err", err)
		return
	}

	now := time.Now()
	cleaned := 0

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), tempPrefix) {
			continue
		}

		dirPath := filepath.Join(workDir, entry.Name())
		if keep[dirPath] {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		age := now.Sub(info.ModTime())
		if age > ttl {
			if err := os.RemoveAll(dirPath); err != nil {
				slog.Warn("cleanup: remove orphan dir failed", "path", dirPath, "err", err)
			} else {
				slog.Info("cleanup: removed orphan temp dir", "path", dirPath, "age", age)
				cleaned++
			}
		}
	}

	if cleaned > 0 {
		slog.Info("cleanup sweep complete", "removed", cleaned, "work_dir", workDir)
	}
}

// CleanupDir removes a specific temp directory, ignoring errors.
func CleanupDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("cleanup: remove temp dir failed", "path", path, "err", err)
	}
}

// NewTempDir creates a new temporary directory for a job.
func NewTempDir(workDir, jobID string) (string, error) {
	dirName := tempPrefix + jobID
	path := filepath.Join(workDir, dirName)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}
