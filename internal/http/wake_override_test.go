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
	// enrichContext; the test injects the same context values directly.
	// The override gate runs BEFORE the agent lookup, so the empty router
	// is never consulted: the expected status is exactly 403 from
	// security.wake_user_override_blocked.
	ctx := store.WithUserID(req.Context(), "caller-1")
	ctx = store.WithTenantID(ctx, uuid.New())
	ctx = store.WithRole(ctx, "operator")
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	h.handleWake(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("tenant-scoped wake with foreign user_id: got %d (%s), want 403 impersonation block", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "master scope") {
		t.Fatalf("403 body does not name the override rule: %s", rec.Body.String())
	}
}

// Master-scope callers bypass the override gate: with an empty agent router
// they must reach the lookup and get 404 (agent not found), never the 403
// override rejection.
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

	if rec.Code != http.StatusNotFound {
		t.Fatalf("master-scope wake with foreign user_id: got %d (%s), want 404 (past the override gate, agent unknown)", rec.Code, rec.Body.String())
	}
}
