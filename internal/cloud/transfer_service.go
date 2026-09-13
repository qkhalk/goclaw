package cloud

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrTransferNotFound is returned by TransferStatus when the job ID is
// unknown to the caller (never started, pruned, or lost to an rcd restart).
var ErrTransferNotFound = errors.New("cloud: transfer job not found")

const (
	// transferTTL prunes records that were never polled (async jobs die with
	// the rcd process; keeping stale rows forever would leak memory).
	transferTTL = 24 * time.Hour
	// transferMaxEntries bounds the registry; the oldest records are evicted.
	transferMaxEntries = 1024
)

// TransferRecord is the ownership metadata captured when an async transfer
// starts. It answers "may THIS caller poll THIS job" — nothing else.
type TransferRecord struct {
	TenantID        string
	UserID          string
	SourceAccountID string
	TargetAccountID string
	Mode            string
	StartedAt       time.Time
}

// transferRegistry maps rclone async job IDs to the tenant/user that started
// them, so polling cannot leak job status across tenants. In-memory by
// design: rclone job IDs are only valid for the lifetime of the rcd process
// — after a supervisor restart the client sees ErrTransferNotFound and
// re-issues the transfer.
type transferRegistry struct {
	mu   sync.Mutex
	jobs map[int64]TransferRecord
}

func newTransferRegistry() *transferRegistry {
	return &transferRegistry{jobs: make(map[int64]TransferRecord)}
}

// register records a started async job, pruning expired/overflowing entries.
func (r *transferRegistry) register(jobID int64, rec TransferRecord) {
	if jobID <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for id, j := range r.jobs {
		if now.Sub(j.StartedAt) > transferTTL {
			delete(r.jobs, id)
		}
	}
	for len(r.jobs) >= transferMaxEntries {
		var oldestID int64
		var oldest time.Time
		first := true
		for id, j := range r.jobs {
			if first || j.StartedAt.Before(oldest) {
				oldestID, oldest, first = id, j.StartedAt, false
			}
		}
		delete(r.jobs, oldestID)
	}
	rec.StartedAt = now
	r.jobs[jobID] = rec
}

// ownedBy reports whether the ctx caller (same tenant AND same user — tenant
// admins do not see other users' transfer jobs) may poll jobID.
func (r *transferRegistry) ownedBy(ctx context.Context, jobID int64) (TransferRecord, bool) {
	r.mu.Lock()
	rec, ok := r.jobs[jobID]
	r.mu.Unlock()
	if !ok {
		return TransferRecord{}, false
	}
	if rec.UserID != store.UserIDFromContext(ctx) ||
		rec.TenantID != store.TenantIDFromContext(ctx).String() {
		return TransferRecord{}, false
	}
	return rec, true
}

// TransferAccount copies/moves one path between two accounts' remotes.
// Both accounts must already be resolved and authorized by the caller:
// source needs read access only, the target needs the write guard + write
// OAuth scopes.
//
// A single file transfers synchronously (returns jobID 0). A folder uses
// rclone's async sync/copy (folder "move" is a COPY — the source is never
// deleted wholesale) and returns the registered rclone job ID for
// TransferStatus polling; the rc client's 60s timeout makes sync folder
// transfers a guaranteed hang for anything but trivial trees.
func (s *StorageService) TransferAccount(ctx context.Context, src, dst *store.CloudAccount, srcPath, dstPath, mode string) (int64, error) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return 0, err
	}
	if srcPath, err = CleanRemotePath(srcPath); err != nil {
		return 0, err
	}
	if dstPath, err = CleanRemotePath(dstPath); err != nil {
		return 0, err
	}
	if mode == "" {
		mode = "copy"
	}
	if mode != "copy" && mode != "move" {
		return 0, fmt.Errorf("cloud_transfer: unsupported mode %q (want copy|move)", mode)
	}

	var jobID int64
	err = s.runWithTwoRemotes(ctx, src, dst, func(srcFS, dstFS string) error {
		info, statErr := rc.OperationsStat(ctx, srcFS, srcPath)
		if statErr != nil {
			return fmt.Errorf("cloud_transfer: source: %w", statErr)
		}
		if info.IsDir {
			id, syncErr := rc.SyncCopy(ctx, srcFS, srcPath, dstFS, dstPath, true)
			if syncErr != nil {
				return fmt.Errorf("cloud_transfer: folder sync: %w", syncErr)
			}
			if id <= 0 {
				return errors.New("cloud_transfer: rclone returned no async job id")
			}
			jobID = id
			return nil
		}
		if mode == "move" {
			return rc.OperationsMoveFile(ctx, srcFS+":", srcPath, dstFS+":", dstPath)
		}
		return rc.OperationsCopyFile(ctx, srcFS+":", srcPath, dstFS+":", dstPath)
	})
	if err != nil {
		return 0, err
	}
	if jobID > 0 {
		s.transfers.register(jobID, TransferRecord{
			TenantID:        store.TenantIDFromContext(ctx).String(),
			UserID:          store.UserIDFromContext(ctx),
			SourceAccountID: src.ID,
			TargetAccountID: dst.ID,
			Mode:            mode,
		})
	}
	return jobID, nil
}

// TransferStatus polls one async transfer job started by the ctx caller.
func (s *StorageService) TransferStatus(ctx context.Context, jobID int64) (*storage.JobInfo, error) {
	if _, ok := s.transfers.ownedBy(ctx, jobID); !ok {
		return nil, ErrTransferNotFound
	}
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return nil, err
	}
	return rc.JobStatus(ctx, jobID)
}
