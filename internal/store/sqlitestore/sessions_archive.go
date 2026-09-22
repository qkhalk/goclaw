//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Compile-time guard: the SQLite store implements the archive capability.
var _ store.SessionArchiveStore = (*SQLiteSessionStore)(nil)

// archivedAtPtr converts a scanned nullable timestamp to *time.Time.
func archivedAtPtr(nt nullSqliteTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	t := nt.Time
	return &t
}

// ArchiveSession soft-hides a session from default listings by stamping
// archived_at (RFC3339Nano text — matches the column's TEXT timestamps).
// Scope mirrors Delete: session_key + tenant_id from ctx — per-user ownership
// is enforced by the WS handler before calling in. Idempotent: an
// already-archived row keeps its original archive timestamp. Messages,
// summary, and metadata are never touched — archive is not delete.
func (s *SQLiteSessionStore) ArchiveSession(ctx context.Context, key string) error {
	tid := tenantIDForInsert(ctx)
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET archived_at = ?
		 WHERE session_key = ? AND archived_at IS NULL AND tenant_id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), key, tid)
	return err
}

// RestoreSession un-hides an archived session (archived_at = NULL). Rows that
// are not archived or belong to another tenant match nothing — no-op.
func (s *SQLiteSessionStore) RestoreSession(ctx context.Context, key string) error {
	tid := tenantIDForInsert(ctx)
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET archived_at = NULL
		 WHERE session_key = ? AND tenant_id = ?`,
		key, tid)
	return err
}
