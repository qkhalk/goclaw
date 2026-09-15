package cloud

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrTransferNotFound is returned by TransferStatus when the job ID is
// unknown to the caller (never started, pruned, or lost to a restart — native
// jobs live in this process only).
var ErrTransferNotFound = errors.New("cloud: transfer job not found")

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

// TransferAccount copies/moves one path between two accounts' remotes.
// Both accounts must already be resolved and authorized by the caller:
// source needs read access only, the target needs the write guard + write
// OAuth scopes.
//
// A single file transfers synchronously (returns jobID 0); "move" deletes
// the source file after a successful copy. A folder starts a native async
// copy job (folder "move" is a COPY — the source is never deleted wholesale)
// and returns the registered job ID for TransferStatus polling — HTTP
// timeouts make blocking folder transfers a guaranteed hang for anything but
// trivial trees.
func (s *StorageService) TransferAccount(ctx context.Context, src, dst *store.CloudAccount, srcPath, dstPath, mode string) (int64, error) {
	cleanSrc, err := CleanRemotePath(srcPath)
	if err != nil {
		return 0, err
	}
	cleanDst, err := CleanRemotePath(dstPath)
	if err != nil {
		return 0, err
	}
	if mode == "" {
		mode = "copy"
	}
	if mode != "copy" && mode != "move" {
		return 0, fmt.Errorf("cloud_transfer: unsupported mode %q (want copy|move)", mode)
	}

	// Single file: synchronous stream src → dst.
	info, err := s.StatAccount(ctx, src, cleanSrc)
	if err != nil {
		return 0, fmt.Errorf("cloud_transfer: source: %w", err)
	}
	if !info.IsDir {
		body, _, err := s.OpenAccount(ctx, src, cleanSrc, "", s.fileCapForTransfer())
		if err != nil {
			if errors.Is(err, ErrFileTooLarge) {
				// Transfers are not bound by the preview cap — open uncapped.
				body, _, err = s.backendOpenUncapped(ctx, src, cleanSrc)
				if err != nil {
					return 0, fmt.Errorf("cloud_transfer: open source: %w", err)
				}
			} else {
				return 0, fmt.Errorf("cloud_transfer: open source: %w", err)
			}
		}
		defer body.Close()
		dstBackend, err := s.backendFor(ctx, dst)
		if err != nil {
			return 0, err
		}
		if err := dstBackend.Upload(ctx, cleanDst, body, info.Size, "", parseListTime(info.ModTime)); err != nil {
			return 0, fmt.Errorf("cloud_transfer: upload: %w", err)
		}
		if mode == "move" {
			if err := s.DeleteAccount(ctx, src, cleanSrc, false); err != nil {
				return 0, fmt.Errorf("cloud_transfer: delete source after move: %w", err)
			}
		}
		return 0, nil
	}

	// Folder: detached async copy (never deletes the source wholesale).
	jobID, err := s.startFolderCopy(ctx, src, dst, cleanSrc, cleanDst, false, TransferRecord{
		TenantID:        store.TenantIDFromContext(ctx).String(),
		UserID:          store.UserIDFromContext(ctx),
		SourceAccountID: src.ID,
		TargetAccountID: dst.ID,
		Mode:            mode,
	})
	if err != nil {
		return 0, fmt.Errorf("cloud_transfer: folder sync: %w", err)
	}
	return jobID, nil
}

// backendOpenUncapped opens a file body without the size-cap pre-check
// (transfers are server-side plumbing, not user-facing downloads).
func (s *StorageService) backendOpenUncapped(ctx context.Context, acct *store.CloudAccount, path string) (*storage.Content, *storage.StatInfo, error) {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return nil, nil, err
	}
	cleaned, err := CleanRemotePath(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := backend.Stat(ctx, cleaned)
	if err != nil {
		return nil, nil, err
	}
	body, err := backend.Open(ctx, cleaned, "")
	if err != nil {
		return nil, nil, err
	}
	return body, info, nil
}

// fileCapForTransfer: effectively unbounded pre-check for transfers (the cap
// exists to protect interactive downloads, not server-side plumbing).
func (s *StorageService) fileCapForTransfer() int64 { return 1 << 40 } // 1 PiB sentinel

// TransferStatus polls one async transfer job started by the ctx caller.
func (s *StorageService) TransferStatus(ctx context.Context, jobID int64) (*storage.JobInfo, error) {
	if _, ok := s.transfers.ownedBy(ctx, jobID); !ok {
		return nil, ErrTransferNotFound
	}
	job, ok := s.transfers.get(jobID)
	if !ok {
		return nil, ErrTransferNotFound
	}
	return job.snapshot(jobID), nil
}
