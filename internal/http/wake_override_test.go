package http

import (
	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A2: a body user_id different from the authenticated user must be rejected
// for non-master-scope callers instead of silently impersonating.
func TestWake_UserOverrideBlockedForTenantScope(t *testing.T) {
	h := NewWakeHandler(agent.NewRouter())

	body := `{"message":"hi","user_id":"victim-user"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/a1/wake", strings.NewReader(body))
	req.SetPathValue("id", "a1")
	req.Header.Set("Content-Type", "application/json")

	// Tenant-scoped operator principal (not master scope): tenant set +
	// non-owner role. resolveAuth would normally populate this via
	// enrichContext; the test injects the same context values directly so
	// only the override gate is exercised (agents router is nil — the gate
	// must fire before any agent lookup).
	ctx := store.WithUserID(req.Context(), "caller-1")
	ctx = store.WithTenantID(ctx, uuid.New())
	ctx = store.WithRole(ctx, "operator")
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	h.handleWake(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("wake user_id override as tenant scope: got %d, want 403", rec.Code)
	}
}

// Master-scope callers bypass the override gate (they proceed past it; with
// an empty agent router the lookup 404s later, but never at the override check).
func TestWake_UserOverrideAllowedForMasterScope(t *testing.T) {
	h := NewWakeHandler(agent.NewRouter())

	body := `{"message":"hi","user_id":"other-user"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/a1/wake", strings.NewReader(body))
	req.SetPathValue("id", "a1")
	ctx := store.WithUserID(req.Context(), "root")
	ctx = store.WithTenantID(ctx, uuid.New())
	ctx = store.WithRole(ctx, store.RoleOwner)
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	h.handleWake(rec, req)
	ctx = store.WithRole(ctx, store.RoleOwner)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("master-scope override must not be blocked by the override gate: got 403")
	}
}
