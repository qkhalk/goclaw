package pg

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// approvalTestDB opens a migrated PG test database. Skips when
// TEST_DATABASE_URL is unset or PG is unreachable.
func approvalTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skipf("TEST_DATABASE_URL not set; skipping PG approval store tests")
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

// seedApprovalTenantPG inserts a minimal tenant + agent row and registers
// cleanup. Slug/agent_key use the FULL UUID: UUIDv7 values generated in the
// same millisecond share their 8-char prefix, so deriving from [:8] collides
// and ON CONFLICT DO NOTHING swallows the insert — leaving the tenant row
// absent and the agent insert violating agents_tenant_id_fkey.
func seedApprovalTenantPG(t *testing.T, db *sql.DB) (tenantID, agentID uuid.UUID) {
	t.Helper()
	tenantID = uuid.Must(uuid.NewV7())
	agentID = uuid.Must(uuid.NewV7())

	_, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES ($1,$2,$3,'active') ON CONFLICT DO NOTHING`,
		tenantID, "approval-pg-"+tenantID.String(), "apg"+tenantID.String())
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO agents (id, tenant_id, agent_key, agent_type, status, provider, model, owner_id)
		 VALUES ($1,$2,$3,'predefined','active','test','test-model','owner') ON CONFLICT DO NOTHING`,
		agentID, tenantID, "aa-"+agentID.String())
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM approval_requests WHERE tenant_id=$1", tenantID)
		db.Exec("DELETE FROM agents WHERE id=$1", agentID)
		db.Exec("DELETE FROM tenants WHERE id=$1", tenantID)
	})
	return tenantID, agentID
}

func TestPGApprovalStore_CRUD(t *testing.T) {
	db := approvalTestDB(t)
	tenantID, agentID := seedApprovalTenantPG(t, db)
	s := NewPGApprovalStore(db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	// Create
	req := &store.ApprovalRequest{
		AgentID:        &agentID,
		RequesterType:  "agent",
		ActionType:     "exec",
		Payload:        []byte(`{"command":"pip install foo"}`),
		Command:        "pip install foo",
		Status:         store.ApprovalStatusPending,
		TimeoutSeconds: 120,
	}
	if err := s.CreateRequest(ctx, req); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if req.ID == uuid.Nil {
		t.Fatal("CreateRequest should assign an ID")
	}

	// GetByID
	got, err := s.GetByID(ctx, req.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil for existing row")
	}
	if got.Command != "pip install foo" {
		t.Fatalf("GetByID command = %q, want %q", got.Command, "pip install foo")
	}

	// ListPending
	pending, err := s.ListPending(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("ListPending count = %d, want 1", len(pending))
	}

	// Resolve → approved
	decidedBy := uuid.Must(uuid.NewV7())
	if err := s.ResolveWithScope(ctx, req.ID, store.ApprovalDecisionAllowOnce, &decidedBy, store.ApprovalGrant{AllowOnce: true}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Idempotent: second resolve must fail closed.
	err = s.ResolveWithScope(ctx, req.ID, store.ApprovalDecisionDeny, &decidedBy, store.ApprovalGrant{})
	if !errors.Is(err, store.ErrApprovalAlreadyResolved) {
		t.Fatalf("second Resolve error = %v, want ErrApprovalAlreadyResolved", err)
	}

	// History
	hist, err := s.ListHistory(ctx, tenantID, store.ApprovalListOpts{Limit: 10})
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("ListHistory count = %d, want 1", len(hist))
	}
	if hist[0].Decision != store.ApprovalDecisionAllowOnce {
		t.Fatalf("history decision = %q, want allow-once", hist[0].Decision)
	}
}

func TestPGApprovalStore_MarkExpired(t *testing.T) {
	db := approvalTestDB(t)
	tenantID, _ := seedApprovalTenantPG(t, db)
	s := NewPGApprovalStore(db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	req := &store.ApprovalRequest{
		ActionType: "exec",
		Command:    "deploy --prod",
		Status:     store.ApprovalStatusPending,
	}
	if err := s.CreateRequest(ctx, req); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	if err := s.MarkExpired(ctx, req.ID); err != nil {
		t.Fatalf("MarkExpired: %v", err)
	}

	got, err := s.GetByID(ctx, req.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Status != store.ApprovalStatusExpired {
		t.Fatalf("status = %q, want expired", got.Status)
	}

	// Cannot resolve an already-expired row.
	err = s.ResolveWithScope(ctx, req.ID, store.ApprovalDecisionDeny, nil, store.ApprovalGrant{})
	if !errors.Is(err, store.ErrApprovalAlreadyResolved) {
		t.Fatalf("resolve after expire error = %v, want ErrApprovalAlreadyResolved", err)
	}
}

func TestPGApprovalStore_TenantIsolation(t *testing.T) {
	db := approvalTestDB(t)
	tenantA, _ := seedApprovalTenantPG(t, db)
	tenantB := uuid.Must(uuid.NewV7())
	_, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES ($1,$2,$3,'active') ON CONFLICT DO NOTHING`,
		tenantB, "approval-pg-b-"+tenantB.String()[:8], "apgb"+tenantB.String()[:8])
	if err != nil {
		t.Fatalf("seed tenant B: %v", err)
	}

	s := NewPGApprovalStore(db)
	ctxA := store.WithTenantID(context.Background(), tenantA)
	req := &store.ApprovalRequest{ActionType: "exec", Command: "whoami"}
	if err := s.CreateRequest(ctxA, req); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	// Tenant B cannot see tenant A's request.
	ctxB := store.WithTenantID(context.Background(), tenantB)
	got, err := s.GetByID(ctxB, req.ID)
	if err != nil {
		t.Fatalf("GetByID cross-tenant: %v", err)
	}
	if got != nil {
		t.Fatalf("tenant B read tenant A approval: %+v", got)
	}

	// Tenant B cannot resolve tenant A's request.
	err = s.ResolveWithScope(ctxB, req.ID, store.ApprovalDecisionDeny, nil, store.ApprovalGrant{})
	if !errors.Is(err, store.ErrApprovalAlreadyResolved) {
		t.Fatalf("tenant B resolve error = %v, want ErrApprovalAlreadyResolved", err)
	}
}

// TestPGApprovalStore_ScopesAndListPendingAll covers the Approval Engine v2
// columns (session_key, args_digest, grant_expires_at) and the cross-tenant
// pending listing used at startup reinstatement.
func TestPGApprovalStore_ScopesAndListPendingAll(t *testing.T) {
	db := approvalTestDB(t)
	tenantA, _ := seedApprovalTenantPG(t, db)
	tenantB := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES ($1,$2,$3,'active') ON CONFLICT DO NOTHING`,
		tenantB, "approval-pg-c-"+tenantB.String()[:8], "apgc"+tenantB.String()[:8]); err != nil {
		t.Fatalf("seed tenant B: %v", err)
	}

	s := NewPGApprovalStore(db)
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	ctxA := store.WithTenantID(context.Background(), tenantA)
	reqA := &store.ApprovalRequest{
		ActionType: "browser",
		Command:    `browser {"action":"navigate"}`,
		SessionKey: "sess-a",
		ArgsDigest: "digest-a",
	}
	if err := s.CreateRequest(ctxA, reqA); err != nil {
		t.Fatalf("CreateRequest A: %v", err)
	}

	ctxB := store.WithTenantID(context.Background(), tenantB)
	reqB := &store.ApprovalRequest{
		ActionType: "workstation_exec",
		Command:    `workstation_exec {"cmd":"deploy"}`,
	}
	if err := s.CreateRequest(ctxB, reqB); err != nil {
		t.Fatalf("CreateRequest B: %v", err)
	}

	// ListPendingAll sees both rows with their tenants intact.
	all, err := s.ListPendingAll(context.Background())
	if err != nil {
		t.Fatalf("ListPendingAll: %v", err)
	}
	byTenant := map[uuid.UUID]store.ApprovalRequest{}
	for _, r := range all {
		byTenant[r.TenantID] = r
	}
	gotA, ok := byTenant[tenantA]
	if !ok {
		t.Fatal("ListPendingAll missing tenant A row")
	}
	if gotA.SessionKey != "sess-a" || gotA.ArgsDigest != "digest-a" {
		t.Errorf("tenant A row = %+v, want session/digest round-trip", gotA)
	}
	if _, ok := byTenant[tenantB]; !ok {
		t.Fatal("ListPendingAll missing tenant B row")
	}

	// ResolveWithScope stores the allow-for-session grant + expiry.
	decidedBy := uuid.Must(uuid.NewV7())
	if err := s.ResolveWithScope(ctxA, reqA.ID, store.ApprovalDecisionAllowForSession, &decidedBy, store.ApprovalGrant{
		AllowForSession: true,
		SessionKey:      "sess-a",
		ExpiresAt:       &expires,
	}); err != nil {
		t.Fatalf("ResolveWithScope: %v", err)
	}

	got, err := s.GetByID(ctxA, reqA.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Decision != store.ApprovalDecisionAllowForSession {
		t.Errorf("decision = %q, want %q", got.Decision, store.ApprovalDecisionAllowForSession)
	}
	if got.SessionKey != "sess-a" {
		t.Errorf("session_key = %q, want sess-a", got.SessionKey)
	}
	if got.GrantExpiresAt == nil || !got.GrantExpiresAt.Equal(expires) {
		t.Errorf("grant_expires_at = %v, want %v", got.GrantExpiresAt, expires)
	}

	hist, err := s.ListHistory(ctxA, tenantA, store.ApprovalListOpts{Limit: 10})
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(hist) != 1 || hist[0].ID != reqA.ID {
		t.Fatalf("history = %+v, want the resolved row", hist)
	}
}
