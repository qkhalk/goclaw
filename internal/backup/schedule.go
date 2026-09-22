package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Scheduled-backup configuration, persisted in the encrypted config_secrets
// store (master scope) so it survives restarts and never touches config.json.
type ScheduleConfig struct {
	Enabled       bool   `json:"enabled"`
	IntervalHours int    `json:"interval_hours"` // 1..168
	Destination   string `json:"destination"`    // "s3" | "cloud"
	// CloudAccountID pins the upload to one connected drive account; empty =
	// tenant-default binding (cloud access resolution).
	CloudAccountID string `json:"cloud_account_id,omitempty"`
	// CloudPath is the remote directory for "cloud" uploads
	// (default "GoClaw Backups").
	CloudPath string `json:"cloud_path,omitempty"`
	// KeepCount prunes old S3 backups beyond the newest N (0 = keep all).
	KeepCount int `json:"keep_count"`
}

const (
	ScheduleKey     = "backup.schedule"
	ScheduleLastKey = "backup.schedule.last"
)

// ScheduleRun is the outcome of one scheduled (or manual run-now) backup.
type ScheduleRun struct {
	At         time.Time `json:"at"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
	Artifact   string    `json:"artifact,omitempty"` // remote key/path
	SizeBytes  int64     `json:"size_bytes,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

// Validate normalizes and checks a schedule config.
func (c ScheduleConfig) Validate() error {
	if c.IntervalHours < 1 || c.IntervalHours > 168 {
		return fmt.Errorf("interval_hours must be 1..168")
	}
	if c.Destination != "s3" && c.Destination != "cloud" {
		return fmt.Errorf(`destination must be "s3" or "cloud"`)
	}
	if c.KeepCount < 0 || c.KeepCount > 100 {
		return fmt.Errorf("keep_count must be 0..100")
	}
	if c.CloudPath == "" {
		c.CloudPath = DefaultCloudBackupPath
	}
	return nil
}

// DefaultCloudBackupPath is the remote directory used for scheduled cloud
// uploads when the config leaves CloudPath empty.
const DefaultCloudBackupPath = "GoClaw Backups"

// LoadScheduleConfig reads the schedule config; missing = zero value (disabled).
func LoadScheduleConfig(ctx context.Context, secrets store.ConfigSecretsStore) (ScheduleConfig, error) {
	var cfg ScheduleConfig
	if secrets == nil {
		return cfg, nil
	}
	raw, err := secrets.Get(ctx, ScheduleKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("get %q: %w", ScheduleKey, err)
	}
	if raw == "" {
		return cfg, nil
	}
	if err := jsonUnmarshalStrict(raw, &cfg); err != nil {
		return ScheduleConfig{}, fmt.Errorf("parse %q: %w", ScheduleKey, err)
	}
	return cfg, nil
}

// SaveScheduleConfig validates and persists the schedule config.
func SaveScheduleConfig(ctx context.Context, secrets store.ConfigSecretsStore, cfg ScheduleConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.CloudPath == "" {
		cfg.CloudPath = DefaultCloudBackupPath
	}
	raw, err := jsonMarshalCompact(cfg)
	if err != nil {
		return err
	}
	return secrets.Set(ctx, ScheduleKey, raw)
}

// LoadScheduleLastRun returns the last recorded run, or nil when none ran yet.
func LoadScheduleLastRun(ctx context.Context, secrets store.ConfigSecretsStore) (*ScheduleRun, error) {
	if secrets == nil {
		return nil, nil
	}
	raw, err := secrets.Get(ctx, ScheduleLastKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || raw == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("get %q: %w", ScheduleLastKey, err)
	}
	var run ScheduleRun
	if err := jsonUnmarshalStrict(raw, &run); err != nil {
		return nil, fmt.Errorf("parse %q: %w", ScheduleLastKey, err)
	}
	return &run, nil
}

func saveScheduleLastRun(ctx context.Context, secrets store.ConfigSecretsStore, run ScheduleRun) error {
	raw, err := jsonMarshalCompact(run)
	if err != nil {
		return err
	}
	return secrets.Set(ctx, ScheduleLastKey, raw)
}

func jsonUnmarshalStrict(raw string, v any) error { return json.Unmarshal([]byte(raw), v) }

func jsonMarshalCompact(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
