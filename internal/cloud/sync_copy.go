package cloud

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Sync primitives for the sync-pairs worker. Unlike TransferAccount
// (user-facing, registry-guarded), these are worker-internal: the sync
// service only polls job IDs it started itself within this process.

// SyncCopyAccounts mirrors srcPath (a folder, "/" = drive root) from src into
// dstPath on dst — ADDITIVE, rclone sync/copy semantics preserved: files
// missing at the target are copied, files identical at the target (same size,
// mtime within slack) are skipped, and files deleted from the source are
// NEVER deleted at the target. Always async (a detached native copy job —
// blocking folder syncs would hang the worker sweep for large trees);
// returns the job id for SyncJobStatus polling.
func (s *StorageService) SyncCopyAccounts(ctx context.Context, src, dst *store.CloudAccount, srcPath, dstPath string) (int64, error) {
	// CleanRemoteDir (not CleanRemotePath): the drive root "/" is a valid sync
	// endpoint and normalizes to "".
	return s.startFolderCopy(ctx, src, dst, srcPath, dstPath, true, TransferRecord{
		TenantID:        store.TenantIDFromContext(ctx).String(),
		UserID:          store.UserIDFromContext(ctx),
		SourceAccountID: src.ID,
		TargetAccountID: dst.ID,
		Mode:            "sync",
	})
}

// SyncJobStatus polls one async copy job started by the sync worker. No
// registry ownership check — the worker never sees caller-supplied job IDs.
func (s *StorageService) SyncJobStatus(ctx context.Context, jobID int64) (*storage.JobInfo, error) {
	job, ok := s.transfers.get(jobID)
	if !ok {
		return nil, ErrTransferNotFound
	}
	return job.snapshot(jobID), nil
}
