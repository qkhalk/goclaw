package pg

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// cloudSyncTestDB opens a migrated PG test database. Skips when
// TEST_DATABASE_URL is unset or PG is unreachable.
func cloudSyncTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skipf("TEST_DATABASE_URL not set; skipping PG cloud sync store tests")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("PG not reachable: %v", err)
	}

	m, err := migrate.New("file://../../../migrations", dsn)
	if err != nil {
		db.Close()
		t.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		db.Close()
		t.Fatalf("migrate up: %v", err)
	}
	m.Close()

	InitSqlx(db)
	t.Cleanup(func() { db.Close() })
	return db
}

// seedCloudSyncTenant inserts a tenant plus two cloud accounts (sync pair
// endpoints) and registers cleanup.
func seedCloudSyncTenant(t *testing.T, db *sql.DB) (tenantID string, srcAcctID, dstAcctID string) {
	t.Helper()
	tenantUUID := uuid.Must(uuid.NewV7())
	tenantID = tenantUUID.String()
	srcAcctID, dstAcctID = uuid.NewString(), uuid.NewString()

	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES ($1,$2,$3,'active') ON CONFLICT DO NOTHING`,
		tenantUUID, "cloudsync-pg-"+tenantID, "csp"+tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	for id, suffix := range map[string]string{srcAcctID: "src", dstAcctID: "dst"} {
		if _, err := db.Exec(
			`INSERT INTO cloud_accounts (id, tenant_id, user_id, provider, email, access_token)
			 VALUES ($1,$2,$3,'google',$4,'tok')`,
			id, tenantUUID, "system", suffix+"-cloudsync-test@example.com"); err != nil {
			t.Fatalf("seed cloud account %s: %v", suffix, err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM cloud_sync_pairs WHERE tenant_id=$1", tenantUUID)
		db.Exec("DELETE FROM cloud_accounts WHERE tenant_id=$1", tenantUUID)
		db.Exec("DELETE FROM tenants WHERE id=$1", tenantUUID)
	})
	return tenantID, srcAcctID, dstAcctID
}

// TestPGCloudSyncPairStore_EmptyAndStringCreatedBy covers the due-pairs
// regression: pairs created without a user (created_by NULL) or with a
// non-uuid GoClaw user id ("system") used to fail the SELECT with
// `invalid input syntax for type uuid: ""` (COALESCE against the uuid-typed
// column) — user-ish columns are TEXT since migration 000124.
func TestPGCloudSyncPairStore_EmptyAndStringCreatedBy(t *testing.T) {
	db := cloudSyncTestDB(t)
	tenantID, srcAcctID, dstAcctID := seedCloudSyncTenant(t, db)
	s := NewPGCloudSyncPairStore(db)
	ctx := store.WithTenantID(context.Background(), uuid.MustParse(tenantID))

	// Empty CreatedBy → NULL in DB (never bind "" for a uuid-ish column).
	noUser := &store.CloudSyncPair{
		SourceAccountID: srcAcctID,
		TargetAccountID: dstAcctID,
		IntervalMinutes: 15,
		Enabled:         true,
	}
	if err := s.Create(ctx, noUser); err != nil {
		t.Fatalf("Create(empty CreatedBy): %v", err)
	}
	// Non-uuid user id — GoClaw user ids are arbitrary strings ("system",
	// telegram numeric ids); this failed the uuid column before migration 124.
	systemUser := &store.CloudSyncPair{
		SourceAccountID: dstAcctID,
		TargetAccountID: srcAcctID,
		IntervalMinutes: 30,
		Enabled:         true,
		CreatedBy:       "system",
	}
	if err := s.Create(ctx, systemUser); err != nil {
		t.Fatalf("Create(created_by=\"system\"): %v", err)
	}

	// Get exercises the COALESCE(created_by,'') scan — crashed on NULL
	// created_by rows while the column was uuid-typed.
	gotNoUser, err := s.Get(ctx, noUser.ID)
	if err != nil {
		t.Fatalf("Get(NULL created_by): %v", err)
	}
	if gotNoUser.CreatedBy != "" {
		t.Errorf("NULL created_by must scan back as %q, got %q", "", gotNoUser.CreatedBy)
	}
	gotSystem, err := s.Get(ctx, systemUser.ID)
	if err != nil {
		t.Fatalf("Get(created_by=\"system\"): %v", err)
	}
	if gotSystem.CreatedBy != "system" {
		t.Errorf("created_by = %q, want %q", gotSystem.CreatedBy, "system")
	}

	// DuePairs — the query from the live-server failure log.
	due, err := s.DuePairs(ctx, time.Now())
	if err != nil {
		t.Fatalf("DuePairs: %v", err)
	}
	gotIDs := make([]string, 0, len(due))
	for _, p := range due {
		gotIDs = append(gotIDs, p.ID)
	}
	for _, want := range []string{noUser.ID, systemUser.ID} {
		if !slices.Contains(gotIDs, want) {
			t.Errorf("DuePairs missing pair %s (got %v)", want, gotIDs)
		}
	}
}
