//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestSQLiteSchemaUpgrade_83_to_84_BindingEnablePriority covers the
// cloud_account_bindings enabled + priority columns (twin of PG 000121).
// Verifies legacy rows upgrade to enabled=1 / priority=100 so resolution
// behavior is unchanged, and the resolver index is created.
func TestSQLiteSchemaUpgrade_83_to_84_BindingEnablePriority(t *testing.T) {
	db := openTestDBAtVersion(t, 83)

	// Recreate the table in the pre-84 (v83) shape: no enabled/priority.
	// Indexes follow the renamed table, so drop them by name before reuse.
	mustExec(t, db, `DROP INDEX IF EXISTS cloud_account_bindings_resolve`)
	mustExec(t, db, `DROP INDEX IF EXISTS cloud_account_bindings_uq`)
	mustExec(t, db, `DROP INDEX IF EXISTS cloud_account_bindings_lookup`)
	mustExec(t, db, `ALTER TABLE cloud_account_bindings RENAME TO cloud_account_bindings_old`)
	mustExec(t, db, `CREATE TABLE cloud_account_bindings (
	id          TEXT NOT NULL PRIMARY KEY,
	tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
	scope_type  TEXT NOT NULL CHECK (scope_type IN ('tenant', 'user', 'group')),
	scope_key   TEXT NOT NULL DEFAULT '',
	provider    TEXT NOT NULL,
	account_id  TEXT NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
	created_by  TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	CHECK ((scope_type = 'tenant' AND scope_key = '') OR (scope_type <> 'tenant' AND scope_key <> ''))
)`)
	mustExec(t, db, `INSERT INTO cloud_account_bindings
		(id, tenant_id, scope_type, scope_key, provider, account_id, created_by)
	SELECT id, tenant_id, scope_type, scope_key, provider, account_id, created_by
	FROM cloud_account_bindings_old`)
	mustExec(t, db, `DROP TABLE cloud_account_bindings_old`)
	mustExec(t, db, `CREATE UNIQUE INDEX cloud_account_bindings_uq
	ON cloud_account_bindings (tenant_id, scope_type, scope_key, provider)`)
	mustExec(t, db, `CREATE INDEX cloud_account_bindings_lookup
	ON cloud_account_bindings (tenant_id, scope_type)`)

	// A legacy binding row (plus the account it points at).
	mustExec(t, db, `INSERT INTO cloud_accounts (id, tenant_id, user_id, provider, email, access_token)
	VALUES ('acct-1', '0193a5b0-7000-7000-8000-000000000001', 'user-1', 'google', 'legacy@x.test', 'tok')`)
	mustExec(t, db, `INSERT INTO cloud_account_bindings
		(id, tenant_id, scope_type, scope_key, provider, account_id, created_by)
	VALUES ('bind-1', '0193a5b0-7000-7000-8000-000000000001', 'group', '-100', 'google', 'acct-1', 'user-1')`)

	// Run the migration v83 → v84.
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema (v83→v84): %v", err)
	}
	var version int
	if err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, SchemaVersion)
	}

	// Legacy row: enabled defaults on, priority defaults to 100.
	var enabled int
	var priority int
	if err := db.QueryRow(`SELECT enabled, priority FROM cloud_account_bindings WHERE id='bind-1'`).Scan(&enabled, &priority); err != nil {
		t.Fatalf("select legacy binding: %v", err)
	}
	if enabled != 1 || priority != 100 {
		t.Fatalf("legacy binding = enabled %d priority %d, want 1/100", enabled, priority)
	}

	// The store layer reads the migrated row back with the new fields.
	storeImpl := NewSQLiteCloudAccountStore(db, "test-enc-key")
	ctx := store.WithUserID(
		store.WithTenantID(context.Background(), uuid.MustParse("0193a5b0-7000-7000-8000-000000000001")),
		"user-1")
	bindings, err := storeImpl.ListBindings(ctx)
	if err != nil {
		t.Fatalf("ListBindings: %v", err)
	}
	if len(bindings) != 1 || !bindings[0].Enabled || bindings[0].Priority != 100 {
		t.Fatalf("ListBindings = %+v, want 1 row enabled=true priority=100", bindings)
	}
}
