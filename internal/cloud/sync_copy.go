package cloud

import (
	"context"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Sync primitives for the sync-pairs worker (phase 6). Unlike TransferAccount
// (user-facing, registry-guarded), these are worker-internal: the sync service
// only polls job IDs it started itself within this process.

// SyncCopyAccounts mirrors srcPath (a folder, "/" = drive root) from src into
// dstPath on dst using rclone sync/copy — ADDITIVE: files missing at the
// target are copied, files deleted from the source are NEVER deleted at the
// target. Always async (the rc client's 60s timeout makes blocking folder
// syncs a guaranteed hang for large trees); returns the rc job id for
// SyncJobStatus polling.
func (s *StorageService) SyncCopyAccounts(ctx context.Context, src, dst *store.CloudAccount, srcPath, dstPath string) (int64, error) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return 0, err
	}
	// CleanRemoteDir (not CleanRemotePath): the drive root "/" is a valid sync
	// endpoint and normalizes to "" (the bare fs spec).
	if srcPath, err = CleanRemoteDir(srcPath); err != nil {
		return 0, err
	}
	if dstPath, err = CleanRemoteDir(dstPath); err != nil {
		return 0, err
	}
	var jobID int64
	err = s.runWithTwoRemotes(ctx, src, dst, func(srcFS, dstFS string) error {
		id, syncErr := rc.SyncCopy(ctx, srcFS, srcPath, dstFS, dstPath, true)
		if syncErr != nil {
			return fmt.Errorf("cloud_sync: folder sync: %w", syncErr)
		}
		if id <= 0 {
			return fmt.Errorf("cloud_sync: rclone returned no async job id")
		}
		jobID = id
		return nil
	})
	return jobID, err
}

// SyncJobStatus polls one async rc job started by the sync worker. No registry
// ownership check — the worker never sees caller-supplied job IDs.
func (s *StorageService) SyncJobStatus(ctx context.Context, jobID int64) (*storage.JobInfo, error) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return nil, err
	}
	return rc.JobStatus(ctx, jobID)
}
