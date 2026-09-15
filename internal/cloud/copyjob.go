package cloud

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Native async folder-copy engine (replaces rclone's async sync/copy + job
// polling): a detached goroutine walks the source backend and streams every
// file into the destination backend, while the caller polls a snapshot.
// Jobs live in this process only — after a restart the client re-issues the
// transfer, and the additive sync sweep re-copies what is missing.

const (
	// transferTTL prunes records that were never polled (keeping stale rows
	// forever would leak memory).
	transferTTL = 24 * time.Hour
	// transferMaxEntries bounds the registry; the oldest records are evicted.
	transferMaxEntries = 1024
	// modTimeSkew tolerates provider timestamp rounding when deciding a
	// destination file is already identical (rclone uses a comparable slack).
	modTimeSkew = 2 * time.Second
)

// copyJob is one running/finished async copy with its ownership metadata.
type copyJob struct {
	rec TransferRecord

	cancel context.CancelFunc
	done   chan struct{}

	mu         sync.Mutex
	finished   bool
	success    bool
	errMsg     string
	filesDone  int64
	filesTotal int64
	firstErr   string
	startedAt  time.Time
}

// snapshot reads the progress fields atomically enough for status polling.
func (j *copyJob) snapshot(id int64) *storage.JobInfo {
	j.mu.Lock()
	defer j.mu.Unlock()
	errMsg := j.errMsg
	if errMsg == "" {
		errMsg = j.firstErr
	}
	return &storage.JobInfo{
		ID: id, Finished: j.finished, Success: j.success, Error: errMsg,
		FilesDone: j.filesDone, FilesTotal: j.filesTotal,
	}
}

func (j *copyJob) setTotal(n int64) {
	j.mu.Lock()
	j.filesTotal = n
	j.mu.Unlock()
}

func (j *copyJob) countFile() {
	j.mu.Lock()
	j.filesDone++
	j.mu.Unlock()
}

func (j *copyJob) noteError(err error) {
	j.mu.Lock()
	if j.firstErr == "" {
		j.firstErr = err.Error()
	}
	j.mu.Unlock()
}

// transferRegistry maps native job IDs to running copies. Ownership checks
// mirror the former rclone-job registry: same tenant AND same user.
type transferRegistry struct {
	mu   sync.Mutex
	jobs map[int64]*copyJob
	next int64
}

func newTransferRegistry() *transferRegistry {
	return &transferRegistry{jobs: make(map[int64]*copyJob)}
}

// register records a started job, pruning expired/overflowing entries.
// Overflow eviction is deterministic: the lowest job ID (first registered)
// goes first — mirroring the former rclone-job registry semantics.
func (r *transferRegistry) register(job *copyJob) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for id, j := range r.jobs {
		if now.Sub(j.startedAt) > transferTTL && j.isFinished() {
			delete(r.jobs, id)
		}
	}
	for len(r.jobs) >= transferMaxEntries {
		var oldestID int64
		first := true
		for id := range r.jobs {
			if first || id < oldestID {
				oldestID, first = id, false
			}
		}
		delete(r.jobs, oldestID)
	}
	r.next++
	job.rec.StartedAt = now
	r.jobs[r.next] = job
	return r.next
}

func (r *transferRegistry) get(jobID int64) (*copyJob, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[jobID]
	return j, ok
}

// ownedBy reports whether the ctx caller (same tenant AND same user — tenant
// admins do not see other users' transfer jobs) may poll jobID.
func (r *transferRegistry) ownedBy(ctx context.Context, jobID int64) (TransferRecord, bool) {
	j, ok := r.get(jobID)
	if !ok {
		return TransferRecord{}, false
	}
	if j.rec.UserID != store.UserIDFromContext(ctx) ||
		j.rec.TenantID != store.TenantIDFromContext(ctx).String() {
		return TransferRecord{}, false
	}
	return j.rec, true
}

func (j *copyJob) isFinished() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.finished
}

// shutdownAll cancels every unfinished job (gateway close).
func (r *transferRegistry) shutdownAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.jobs {
		j.cancel()
	}
}

// startFolderCopy launches the detached walker for srcPath → dstPath and
// returns the registered job ID. skipIdentical mirrors rclone sync/copy
// semantics (additive mirror); copy-overwrite when false.
func (s *StorageService) startFolderCopy(srcCtx context.Context, src, dst *store.CloudAccount, srcPath, dstPath string, skipIdentical bool, rec TransferRecord) (int64, error) {
	srcBackend, err := s.backendFor(srcCtx, src)
	if err != nil {
		return 0, err
	}
	dstBackend, err := s.backendFor(srcCtx, dst)
	if err != nil {
		return 0, err
	}
	if srcPath, err = CleanRemoteDir(srcPath); err != nil {
		return 0, err
	}
	if dstPath, err = CleanRemoteDir(dstPath); err != nil {
		return 0, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	job := &copyJob{rec: rec, cancel: cancel, done: make(chan struct{}), startedAt: time.Now()}
	id := s.transfers.register(job)
	go func() {
		defer close(job.done)
		runErr := copyFolderTree(ctx, srcBackend, dstBackend, srcPath, dstPath, skipIdentical, job)
		job.mu.Lock()
		job.finished = true
		job.success = runErr == nil && job.firstErr == ""
		if runErr != nil {
			job.errMsg = runErr.Error()
		} else if job.firstErr != "" {
			job.errMsg = job.firstErr
		}
		job.mu.Unlock()
		if runErr != nil || job.firstErr != "" {
			slog.Warn("cloud storage: async copy job finished with errors", "job_id", id, "error", job.errMsg)
		} else {
			slog.Info("cloud storage: async copy job finished", "job_id", id)
		}
	}()
	return id, nil
}

// copyFolderTree mirrors every file under srcPath into dstPath. Directories
// are created as discovered (Stat-then-Mkdir avoids duplicate-folder
// creation, which Drive silently allows and Graph renames). Per-file errors
// are recorded and skipped; the walk continues (rclone sync/copy behavior).
func copyFolderTree(ctx context.Context, src, dst storage.Backend, srcPath, dstPath string, skipIdentical bool, job *copyJob) error {
	entries, err := src.List(ctx, srcPath, 0)
	if err != nil {
		return err
	}
	var files int64
	for _, e := range entries {
		if !e.IsDir {
			files++
		}
	}
	job.mu.Lock()
	job.filesTotal += files
	job.mu.Unlock()

	for _, e := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		childSrc := joinRemotePath(srcPath, e.Name)
		childDst := joinRemotePath(dstPath, e.Name)
		if e.IsDir {
			if _, serr := dst.Stat(ctx, childDst); serr != nil {
				if serr != storage.ErrNotFound {
					job.noteError(serr)
					continue
				}
				if merr := dst.Mkdir(ctx, childDst); merr != nil {
					job.noteError(merr)
					continue
				}
			}
			if rerr := copyFolderTree(ctx, src, dst, childSrc, childDst, skipIdentical, job); rerr != nil {
				return rerr
			}
			continue
		}
		if skipIdentical {
			if existing, serr := dst.Stat(ctx, childDst); serr == nil {
				if existing.Size == e.Size && modTimeClose(existing.ModTime, e.ModTime) {
					job.countFile()
					continue
				}
			} else if serr != storage.ErrNotFound {
				job.noteError(serr)
				job.countFile()
				continue
			}
		}
		body, oerr := src.Open(ctx, childSrc, "")
		if oerr != nil {
			job.noteError(oerr)
			job.countFile()
			continue
		}
		uerr := dst.Upload(ctx, childDst, body, e.Size, "", parseListTime(e.ModTime))
		body.Close()
		job.countFile()
		if uerr != nil {
			job.noteError(uerr)
		}
	}
	return nil
}

// parseListTime parses a normalized RFC3339 listing timestamp (zero when
// unparseable — the provider then stamps "now").
func parseListTime(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// joinRemotePath joins a (possibly empty = root) dir with a child name.
func joinRemotePath(dir, name string) string {
	dir = strings.Trim(dir, "/")
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// modTimeClose compares two RFC3339 timestamps within modTimeSkew
// (unparseable values never compare close → the file is re-copied).
func modTimeClose(a, b string) bool {
	ta, aerr := time.Parse(time.RFC3339, a)
	tb, berr := time.Parse(time.RFC3339, b)
	if aerr != nil || berr != nil {
		return false
	}
	diff := ta.Sub(tb)
	if diff < 0 {
		diff = -diff
	}
	return diff <= modTimeSkew
}
