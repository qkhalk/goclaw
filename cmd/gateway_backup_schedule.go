package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/backup"
	"github.com/nextlevelbuilder/goclaw/internal/cloud"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// startBackupSchedule wires the periodic cloud/S3 backup: the scheduler loop
// (internal/backup) re-reads its config each minute and calls the pipeline
// below — backup.Run to a temp archive, then upload to S3 or a connected
// cloud drive account, then S3 retention pruning.
func startBackupSchedule(
	cfg *config.Config,
	stores *store.Stores,
	cloudMgr *cloud.Manager,
	cloudStorage *cloud.StorageService,
) (stop func(), sched *backup.Scheduler) {
	noop := func() {}
	if stores == nil || stores.ConfigSecrets == nil {
		return noop, nil
	}
	sched = backup.NewScheduler(stores.ConfigSecrets, func(ctx context.Context, sc backup.ScheduleConfig) (string, int64, error) {
		return runScheduledBackup(ctx, cfg, stores, cloudMgr, cloudStorage, sc)
	})
	sched.Start()
	slog.Info("backup.schedule: scheduler started")
	return sched.Stop, sched
}

// runScheduledBackup produces one archive and ships it to the configured
// destination. Returns the remote artifact identifier and archive size.
func runScheduledBackup(
	ctx context.Context,
	cfg *config.Config,
	stores *store.Stores,
	cloudMgr *cloud.Manager,
	cloudStorage *cloud.StorageService,
	sc backup.ScheduleConfig,
) (string, int64, error) {
	tmp, err := os.CreateTemp("", "goclaw-sched-backup-*.tar.gz")
	if err != nil {
		return "", 0, fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	opts := backup.Options{
		DSN:           cfg.Database.PostgresDSN,
		DataDir:       cfg.ResolvedDataDir(),
		WorkspacePath: cfg.WorkspacePath(),
		OutputPath:    tmpPath,
		CreatedBy:     "scheduler",
		GoclawVersion: Version,
	}
	if _, err := backup.Run(ctx, opts); err != nil {
		return "", 0, fmt.Errorf("backup run: %w", err)
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		return "", 0, err
	}

	ts := time.Now().UTC().Format("20060102-150405")
	fileName := fmt.Sprintf("backup-%s.tar.gz", ts)

	switch sc.Destination {
	case "s3":
		s3cfg, err := backup.LoadS3Config(ctx, stores.ConfigSecrets)
		if err != nil {
			return "", 0, fmt.Errorf("s3 config: %w", err)
		}
		if s3cfg == nil {
			return "", 0, fmt.Errorf("s3 destination selected but no S3 credentials are configured")
		}
		client, err := backup.NewS3Client(s3cfg)
		if err != nil {
			return "", 0, fmt.Errorf("s3 client: %w", err)
		}
		f, err := os.Open(tmpPath)
		if err != nil {
			return "", 0, err
		}
		err = client.Upload(ctx, fileName, f, info.Size())
		f.Close()
		if err != nil {
			return "", 0, fmt.Errorf("s3 upload: %w", err)
		}
		if sc.KeepCount > 0 {
			pruneS3Backups(ctx, client, sc.KeepCount)
		}
		return fileName, info.Size(), nil

	case "cloud":
		if cloudStorage == nil || cloudMgr == nil {
			return "", 0, fmt.Errorf("cloud storage is not available on this instance")
		}
		usable := func(a *store.CloudAccount) bool { return a.Status == "active" }
		acct, err := cloudMgr.ResolveAccount(ctx, sc.CloudAccountID, nil, usable)
		if err != nil {
			return "", 0, fmt.Errorf("resolve cloud account: %w", err)
		}
		remoteDir := sc.CloudPath
		if remoteDir == "" {
			remoteDir = backup.DefaultCloudBackupPath
		}
		if err := cloudStorage.UploadAccount(ctx, acct, filepath.Dir(tmpPath), filepath.Base(tmpPath), remoteDir+"/"+fileName); err != nil {
			return "", 0, fmt.Errorf("cloud upload: %w", err)
		}
		return acct.Email + ":" + remoteDir + "/" + fileName, info.Size(), nil

	default:
		return "", 0, fmt.Errorf("unknown destination %q", sc.Destination)
	}
}

// pruneS3Backups keeps the newest keep remote backups and deletes the rest.
func pruneS3Backups(ctx context.Context, client *backup.S3Client, keep int) {
	entries, err := client.ListBackups(ctx)
	if err != nil {
		slog.Warn("backup.schedule: s3 list for prune failed", "error", err)
		return
	}
	if len(entries) <= keep {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].LastModified.After(entries[j].LastModified) })
	for _, e := range entries[keep:] {
		if err := client.Delete(ctx, e.Key); err != nil {
			slog.Warn("backup.schedule: s3 prune delete failed", "key", e.Key, "error", err)
		} else {
			slog.Info("backup.schedule: pruned old s3 backup", "key", e.Key)
		}
	}
}
