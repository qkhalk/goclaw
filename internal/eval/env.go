package eval

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"
)

// Env is the database environment the eval drivers run against. It mirrors
// the integration-test bootstrap (tests/integration/v3_test_helper.go):
// connect via DSN, run migrations, InitSqlx, seed ephemeral tenants.
type Env struct {
	DB           *sql.DB
	MemoryFabric store.MemoryFabricStore
	Runs         store.RunsStore
	RunTimeline  store.RunTimelineStore
}

// DefaultDSN matches the integration-test default (pgvector on port 5433).
const DefaultDSN = "postgres://postgres:test@localhost:5433/goclaw_test?sslmode=disable"

// NewEnv connects to the eval database, applies migrations and wires the
// stores the drivers need. dsn falls back to TEST_DATABASE_URL then DefaultDSN
// (same precedence as the integration tests). migrationsDir defaults to
// "migrations" relative to the working directory.
func NewEnv(dsn, migrationsDir string) (*Env, error) {
	if dsn == "" {
		dsn = os.Getenv("TEST_DATABASE_URL")
	}
	if dsn == "" {
		dsn = DefaultDSN
	}
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}
	abs, err := filepath.Abs(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("eval migrations path: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("eval migrations dir %s not found (run from repo root or pass --migrations): %w", abs, err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("eval open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("eval db unreachable (start the test PG: docker run -d --name pgtest -p 5433:5432 -e POSTGRES_PASSWORD=test -e POSTGRES_DB=goclaw_test pgvector/pgvector:pg18): %w", err)
	}

	m, err := migrate.New("file://"+filepath.ToSlash(abs), dsn)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("eval migrate init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		m.Close()
		db.Close()
		return nil, fmt.Errorf("eval migrate up: %w", err)
	}
	m.Close()

	pg.InitSqlx(db)

	return &Env{
		DB:           db,
		MemoryFabric: pg.NewPGMemoryFabricStore(db),
		Runs:         pg.NewPGRunStore(db),
		RunTimeline:  pg.NewPGRunTimelineStore(db),
	}, nil
}

// Close releases the database connection.
func (e *Env) Close() error { return e.DB.Close() }

// SeedTenant creates an ephemeral tenant for one suite execution. Rows written
// under it (memories, agent_runs, run_timeline_items) cascade on delete, so
// Cleanup removes the whole footprint. A cleanup failure is logged, not
// fatal: identities are per-run unique so leftovers never affect the next run.
func (e *Env) SeedTenant(prefix string) (uuid.UUID, func()) {
	tenantID := uuid.New()
	name := fmt.Sprintf("eval-%s-%s", prefix, tenantID.String()[:8])
	_, err := e.DB.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES ($1, $2, $3, 'active')`,
		tenantID, name, "e"+tenantID.String()[:8])
	if err != nil {
		// A tenant-insert failure is fatal for every case in the suite; surface
		// it via the store calls that follow rather than panicking here.
		slog.Error("eval.seed_tenant_failed", "tenant_id", tenantID, "error", err)
	}
	return tenantID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := e.DB.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID); err != nil {
			slog.Warn("eval.cleanup_tenant_failed", "tenant_id", tenantID, "error", err)
		}
	}
}

// TenantCtx returns a tenant-scoped context for store calls.
func (e *Env) TenantCtx(tenantID uuid.UUID) context.Context {
	return store.WithTenantID(context.Background(), tenantID)
}
