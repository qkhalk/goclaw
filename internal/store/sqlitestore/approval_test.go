//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// seedApprovalTenant inserts a minimal tenant + agent row for FK satisfaction.
func seedApprovalTenant(t *testing.T, db *sql.DB) (tenantID, agentID uuid.UUID) {
	t.Helper()
	tenantID = uuid.Must(uuid.NewV7())
	agentID = uuid.Must(uuid.NewV7())
	_, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES (?,?,?,'active')`,
		tenantID.String(), "approval-test-"+tenantID.String()[:8], "at"+tenantID.String()[:8])
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO agents (id, tenant_id, agent_key, agent_type, status, provider, model, owner_id)
		 VALUES (?,?,?,'predefined','active','test','test-model','owner')`,
		agentID.String(), tenantID.String(), "aa-"+agentID.String()[:8])
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return tenantID, agentID
}

// newApprovalTestDB opens a scratch SQLite DB with the full schema applied.
func newApprovalTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "approval_test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := EnsureSchema(db); err != nil {
		db.Close()
		t.Fatalf("EnsureSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteApprovalStore_CRUD(t *testing.T) {
	db := newApprovalTestDB(t)
	tenantID, agentID := seedApprovalTenant(t, db)
	s := NewSQLiteApprovalStore(db)
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
	if req.TenantID != tenantID {
		t.Fatalf("CreateRequest tenant = %s, want %s", req.TenantID, tenantID)
	}
	if req.Status != store.ApprovalStatusPending {
		t.Fatalf("CreateRequest status = %q, want pending", req.Status)
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
	if got.AgentID == nil || *got.AgentID != agentID {
		t.Fatalf("GetByID agent = %v, want %v", got.AgentID, agentID)
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
	if hist[0].DecidedBy == nil || *hist[0].DecidedBy != decidedBy {
		t.Fatalf("history decided_by = %v, want %v", hist[0].DecidedBy, decidedBy)
	}
}

func TestSQLiteApprovalStore_MarkExpired(t *testing.T) {
	db := newApprovalTestDB(t)
	tenantID, _ := seedApprovalTenant(t, db)
	s := NewSQLiteApprovalStore(db)
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
	if got.DecidedAt == nil {
		t.Fatal("expired row should have decided_at set")
	}

	// Expired rows are not pending.
	pending, err := s.ListPending(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("ListPending count = %d, want 0 after expire", len(pending))
	}

	// Cannot resolve an already-expired row.
	err = s.ResolveWithScope(ctx, req.ID, store.ApprovalDecisionDeny, nil, store.ApprovalGrant{})
	if !errors.Is(err, store.ErrApprovalAlreadyResolved) {
		t.Fatalf("resolve after expire error = %v, want ErrApprovalAlreadyResolved", err)
	}
}

func TestSQLiteApprovalStore_TenantIsolation(t *testing.T) {
	db := newApprovalTestDB(t)
	tenantA, _ := seedApprovalTenant(t, db)
	tenantB := uuid.Must(uuid.NewV7())
	_, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES (?,?,?,'active')`,
		tenantB.String(), "approval-test-b-"+tenantB.String()[:8], "atb"+tenantB.String()[:8])
	if err != nil {
		t.Fatalf("seed tenant B: %v", err)
	}

	s := NewSQLiteApprovalStore(db)
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

	pending, err := s.ListPending(ctxB, tenantB)
	if err != nil {
		t.Fatalf("ListPending tenant B: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("tenant B ListPending count = %d, want 0", len(pending))
	}

	// Tenant B cannot resolve tenant A's request.
	err = s.ResolveWithScope(ctxB, req.ID, store.ApprovalDecisionDeny, nil, store.ApprovalGrant{})
	if !errors.Is(err, store.ErrApprovalAlreadyResolved) {
		t.Fatalf("tenant B resolve error = %v, want ErrApprovalAlreadyResolved", err)
	}
}

// TestSQLiteApprovalStore_ScopesAndListPendingAll covers the Approval Engine
// v2 columns (session_key, args_digest, grant_expires_at) and the
// cross-tenant pending listing used at startup reinstatement.
func TestSQLiteApprovalStore_ScopesAndListPendingAll(t *testing.T) {
	db := newApprovalTestDB(t)
	tenantA, _ := seedApprovalTenant(t, db)
	tenantB := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES (?,?,?,'active')`,
		tenantB.String(), "approval-test-c-"+tenantB.String()[:8], "atc"+tenantB.String()[:8]); err != nil {
		t.Fatalf("seed tenant B: %v", err)
	}

	s := NewSQLiteApprovalStore(db)
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	// Two tenants, one pending row each: allow-for-session with expiry and a
	// plain pending row with digest.
	ctxA := store.WithTenantID(context.Background(), tenantA)
	reqA := &store.ApprovalRequest{
		ActionType:     "browser",
		Command:        `browser {"action":"navigate"}`,
		SessionKey:     "sess-a",
		ArgsDigest:     "digest-a",
		GrantExpiresAt: nil,
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
	if len(all) != 2 {
		t.Fatalf("ListPendingAll count = %d, want 2", len(all))
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

	// History exposes the resolved row for the approvals history tab.
	hist, err := s.ListHistory(ctxA, tenantA, store.ApprovalListOpts{Limit: 10})
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(hist) != 1 || hist[0].ID != reqA.ID {
		t.Fatalf("history = %+v, want the resolved row", hist)
	}
}
