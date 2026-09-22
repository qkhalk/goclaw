package pg

import (
	"context"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Compile-time guard: the PG store implements the archive capability.
var _ store.SessionArchiveStore = (*PGSessionStore)(nil)

// ArchiveSession soft-hides a session from default listings by stamping
// archived_at. Scope mirrors Delete: session_key + tenant_id from ctx —
// per-user ownership is enforced by the WS handler before calling in.
// Idempotent: an already-archived row keeps its original archive timestamp.
// Messages, summary, and metadata are never touched — archive is not delete.
func (s *PGSessionStore) ArchiveSession(ctx context.Context, key string) error {
	tid := tenantIDForInsert(ctx)
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET archived_at = $1
		 WHERE session_key = $2 AND archived_at IS NULL AND tenant_id = $3`,
		time.Now(), key, tid)
	return err
}

// RestoreSession un-hides an archived session (archived_at = NULL). Rows that
// are not archived or belong to another tenant match nothing — no-op.
func (s *PGSessionStore) RestoreSession(ctx context.Context, key string) error {
	tid := tenantIDForInsert(ctx)
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET archived_at = NULL
		 WHERE session_key = $1 AND tenant_id = $2`,
		key, tid)
	return err
}
