//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
)

// rebuildAgentHooksV21 recreates the agent_hooks table with relaxed CHECK
// constraints — handler_type accepts `script` and source accepts `builtin`.
// Uniqueness indexes on (event, handler_type) per scope are dropped because
// a tenant may want many small script hooks per event (e.g. one redactor per
// PII class).
//
// SQLite cannot ALTER a CHECK constraint: the table must be dropped and
// recreated, then data copied. PRAGMA foreign_keys is a no-op inside a
// transaction, so the sequence runs OFF outside any tx, then wraps the rename
// + recreate + copy in one transaction for atomicity:
//
//  1. PRAGMA foreign_keys = OFF  (outside tx)
//  2. BEGIN
//  3. ALTER TABLE agent_hooks RENAME TO agent_hooks_old
//  4. CREATE TABLE agent_hooks (...new CHECKs)
//  5. INSERT INTO agent_hooks SELECT * FROM agent_hooks_old
//  6. DROP TABLE agent_hooks_old
//  7. recreate the post-rename index that survives the rebuild
//  8. COMMIT
//  9. PRAGMA foreign_keys = ON
//
// Runs after the v20→v21 migration tx commits (parallel to backfillV16 at
// EnsureSchema:677) so the caller's outer migration loop sees v21 only when
// the rebuild also succeeded.
func rebuildAgentHooksV21(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("v21 fk off: %w", err)
	}
	defer func() { _, _ = db.ExecContext(ctx, "PRAGMA foreign_keys = ON") }()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("v21 begin tx: %w", err)
	}

	steps := []string{
		`ALTER TABLE agent_hooks RENAME TO agent_hooks_old`,
		`CREATE TABLE agent_hooks (
            id           TEXT NOT NULL PRIMARY KEY,
            tenant_id    TEXT NOT NULL DEFAULT '0193a5b0-7000-7000-8000-000000000001',
            agent_id     TEXT REFERENCES agents(id) ON DELETE CASCADE,
            scope        TEXT NOT NULL CHECK (scope IN ('global', 'tenant', 'agent')),
            event        TEXT NOT NULL,
            handler_type TEXT NOT NULL CHECK (handler_type IN ('command', 'http', 'prompt', 'script')),
            config       TEXT NOT NULL DEFAULT '{}',
            matcher      TEXT,
            if_expr      TEXT,
            timeout_ms   INTEGER NOT NULL DEFAULT 5000,
            on_timeout   TEXT NOT NULL DEFAULT 'block' CHECK (on_timeout IN ('block', 'allow')),
            priority     INTEGER NOT NULL DEFAULT 0,
            enabled      INTEGER NOT NULL DEFAULT 1,
            version      INTEGER NOT NULL DEFAULT 1,
            source       TEXT NOT NULL DEFAULT 'ui' CHECK (source IN ('ui', 'api', 'seed', 'builtin')),
            metadata     TEXT NOT NULL DEFAULT '{}',
            created_by   TEXT,
            created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
            updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
        )`,
		`INSERT INTO agent_hooks SELECT * FROM agent_hooks_old`,
		`DROP TABLE agent_hooks_old`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hooks_lookup ON agent_hooks (tenant_id, agent_id, event) WHERE enabled = 1`,
	}
	for _, s := range steps {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			_ = tx.Rollback()
			head := s
			if len(head) > 60 {
				head = head[:60]
			}
			return fmt.Errorf("v21 step failed (%q): %w", head, err)
		}
	}
	return tx.Commit()
}
