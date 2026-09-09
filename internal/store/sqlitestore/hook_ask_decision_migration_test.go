//go:build sqlite || sqliteonly

package sqlitestore

import (
	"testing"
)

// TestSQLiteSchemaUpgrade_79_to_80_HookAskDecision covers the hook_executions
// rebuild that extends the decision CHECK vocabulary with 'ask'/'defer'
// (Approval Engine v2). Verifies existing audit rows survive the rebuild, the
// old constraint no longer rejects 'ask', and the indexes are recreated.
func TestSQLiteSchemaUpgrade_79_to_80_HookAskDecision(t *testing.T) {
	db := openTestDBAtVersion(t, 79)

	// openTestDBAtVersion applies the LATEST schema.sql then rewinds the
	// version row. To simulate a genuine pre-v80 DB, rebuild hook_executions
	// with the OLD decision CHECK first (indexes dropped and recreated after,
	// mirroring the migration under test).
	mustExec(t, db, `DROP INDEX IF EXISTS uq_hook_executions_dedup`)
	mustExec(t, db, `DROP INDEX IF EXISTS idx_hook_executions_session`)
	mustExec(t, db, `ALTER TABLE hook_executions RENAME TO hook_executions_old_shape`)
	mustExec(t, db, `CREATE TABLE hook_executions (
    id           TEXT NOT NULL PRIMARY KEY,
    hook_id      TEXT REFERENCES hooks(id) ON DELETE SET NULL,
    session_id   TEXT,
    event        TEXT NOT NULL,
    input_hash   TEXT,
    decision     TEXT NOT NULL CHECK (decision IN ('allow', 'block', 'error', 'timeout')),
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    retry        INTEGER NOT NULL DEFAULT 0,
    dedup_key    TEXT,
    error        TEXT,
    error_detail BLOB,
    metadata     TEXT NOT NULL DEFAULT '{}',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
)`)
	mustExec(t, db, `INSERT INTO hook_executions SELECT * FROM hook_executions_old_shape`)
	mustExec(t, db, `DROP TABLE hook_executions_old_shape`)
	mustExec(t, db, `CREATE INDEX IF NOT EXISTS idx_hook_executions_session ON hook_executions (session_id, created_at)`)
	mustExec(t, db, `CREATE UNIQUE INDEX IF NOT EXISTS uq_hook_executions_dedup ON hook_executions (dedup_key) WHERE dedup_key IS NOT NULL`)

	// Sanity: at v79 the old constraint rejects 'ask'.
	if _, err := db.Exec(`INSERT INTO hook_executions (id, event, decision) VALUES ('pre-1', 'pre_tool_use', 'ask')`); err == nil {
		t.Fatal("v79 accepted decision 'ask'; expected old CHECK to reject it")
	}
	mustExec(t, db, `INSERT INTO hook_executions (id, event, decision, session_id) VALUES ('keep-1', 'pre_tool_use', 'allow', 's1')`)

	// Run the migration v79 → v80.
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema (v79→v80): %v", err)
	}
	var version int
	if err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, SchemaVersion)
	}

	// Pre-existing audit row must have survived the rebuild.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM hook_executions WHERE id = 'keep-1' AND decision = 'allow'`).Scan(&n); err != nil {
		t.Fatalf("query surviving row: %v", err)
	}
	if n != 1 {
		t.Fatalf("surviving rows = %d, want 1", n)
	}

	// The new constraint accepts 'ask' and 'defer'.
	mustExec(t, db, `INSERT INTO hook_executions (id, event, decision) VALUES ('ask-1', 'pre_tool_use', 'ask')`)
	mustExec(t, db, `INSERT INTO hook_executions (id, event, decision) VALUES ('defer-1', 'pre_tool_use', 'defer')`)

	// Indexes must exist again after the rebuild.
	for _, idx := range []string{"idx_hook_executions_session", "uq_hook_executions_dedup"} {
		var c int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, idx).Scan(&c); err != nil {
			t.Fatalf("query index %s: %v", idx, err)
		}
		if c != 1 {
			t.Errorf("index %s missing after rebuild", idx)
		}
	}

	// Dedup unique index must still enforce uniqueness.
	mustExec(t, db, `INSERT INTO hook_executions (id, event, decision, dedup_key) VALUES ('dup-2', 'pre_tool_use', 'allow', 'k1')`)
	if _, err := db.Exec(`INSERT INTO hook_executions (id, event, decision, dedup_key) VALUES ('dup-3', 'pre_tool_use', 'allow', 'k1')`); err == nil {
		t.Error("duplicate dedup_key accepted; unique index missing after rebuild")
	}
}
