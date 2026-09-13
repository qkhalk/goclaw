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
func CleanupSweep(workDir string, ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultTTL
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

		info, err := entry.Info()
		if err != nil {
			continue
		}

		age := now.Sub(info.ModTime())
		if age > ttl {
			dirPath := filepath.Join(workDir, entry.Name())
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
