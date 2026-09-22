package pg

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// sessionArchiveTestDB opens a migrated PG test database. Skips when
// TEST_DATABASE_URL is unset or PG is unreachable (same convention as
// approvalTestDB).
func sessionArchiveTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skipf("TEST_DATABASE_URL not set; skipping PG session archive tests")
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

// TestPGSessionArchiveRestoreCycle mirrors the SQLite cycle test against PG:
// default lists hide archived rows, IncludeArchived reveals them with
// ArchivedAt stamped, messages survive, restore brings the row back, and a
// foreign-tenant archive call is a no-op.
func TestPGSessionArchiveRestoreCycle(t *testing.T) {
	db := sessionArchiveTestDB(t)

	tenantID := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES ($1,$2,$3,'active') ON CONFLICT DO NOTHING`,
		tenantID, "sess-archive-"+tenantID.String(), "sarc"+tenantID.String()); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM sessions WHERE tenant_id = $1`, tenantID)
		db.Exec(`DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	s := NewPGSessionStore(db)
	ctx := store.WithTenantID(context.Background(), tenantID)
	key := "agent:pg-archive-agent:ws:direct:" + uuid.NewString()

	s.GetOrCreate(ctx, key)
	s.SetHistory(ctx, key, []providers.Message{{Role: "user", Content: "keep me"}})
	if err := s.Save(ctx, key); err != nil {
		t.Fatalf("Save: %v", err)
	}

	activeOpts := store.SessionListOpts{Limit: 50, TenantID: tenantID}
	find := func(res store.SessionListResult) (store.SessionInfo, bool) {
		for _, info := range res.Sessions {
			if info.Key == key {
				return info, true
			}
		}
		return store.SessionInfo{}, false
	}

	if _, ok := find(s.ListPaged(ctx, activeOpts)); !ok {
		t.Fatal("active session missing from default ListPaged")
	}

	if err := s.ArchiveSession(ctx, key); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}
	if _, ok := find(s.ListPaged(ctx, activeOpts)); ok {
		t.Fatal("archived session present in default ListPaged")
	}
	if _, ok := findRichSession(s.ListPagedRich(ctx, activeOpts), key); ok {
		t.Fatal("archived session present in default ListPagedRich")
	}

	allOpts := activeOpts
	allOpts.IncludeArchived = true
	info, ok := find(s.ListPaged(ctx, allOpts))
	if !ok {
		t.Fatal("archived session missing from IncludeArchived ListPaged")
	}
	if info.ArchivedAt == nil {
		t.Fatal("ArchivedAt = nil, want stamp after archive")
	}
	rich, ok := findRichSession(s.ListPagedRich(ctx, allOpts), key)
	if !ok || rich.ArchivedAt == nil {
		t.Fatalf("IncludeArchived ListPagedRich row = %+v, want ArchivedAt stamped", rich)
	}

	// Messages survive archiving (cold store = fresh DB read).
	cold := NewPGSessionStore(db).GetHistory(ctx, key)
	if len(cold) != 1 || cold[0].Content != "keep me" {
		t.Fatalf("history while archived = %+v, want message intact", cold)
	}

	// Foreign tenant cannot archive this row.
	foreign := store.WithTenantID(context.Background(), uuid.Must(uuid.NewV7()))
	if err := s.ArchiveSession(foreign, key); err != nil {
		t.Fatalf("ArchiveSession (foreign tenant): %v", err)
	}
	if err := s.RestoreSession(foreign, key); err != nil {
		t.Fatalf("RestoreSession (foreign tenant): %v", err)
	}
	// Row is still archived (foreign restore was a no-op).
	if _, ok := find(s.ListPaged(ctx, activeOpts)); ok {
		t.Fatal("foreign-tenant restore unarchived the session — scope leak")
	}

	// Owner tenant restores.
	if err := s.RestoreSession(ctx, key); err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	info, ok = find(s.ListPaged(ctx, activeOpts))
	if !ok {
		t.Fatal("restored session missing from default ListPaged")
	}
	if info.ArchivedAt != nil {
		t.Fatalf("restored ArchivedAt = %v, want nil", info.ArchivedAt)
	}
}

// findRichSession locates a key inside a ListPagedRich result.
func findRichSession(res store.SessionListRichResult, key string) (store.SessionInfoRich, bool) {
	for _, info := range res.Sessions {
		if info.Key == key {
			return info, true
		}
	}
	return store.SessionInfoRich{}, false
}
