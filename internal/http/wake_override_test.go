package http

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A2: a body user_id different from the authenticated user must be rejected
// for non-master-scope callers instead of silently impersonating.
func TestWake_UserOverrideBlockedForTenantScope(t *testing.T) {
	h := NewWakeHandler(nil)

	req := httptest.NewRequest("POST", "/v1/agents/a1/wake", strings.NewReader(`{"message":"hi","user_id":"victim"}`))
	// No gateway token: resolveAuth fails -> 401 before the gate. To exercise
	// the override gate we need an authenticated, tenant-scoped principal.
	// Master scope (no tenant) is bypassed by IsMasterScope; simulate a
	// tenant-scoped request by injecting tenant + user into context.
	ctx := store.WithUserID(req.Context(), "owner-1")
	ctx = store.WithTenantID(ctx, uuid.New())
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	h.handleWake(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("wake user_id override: got %d, want 403 (master-scope only)", rec.Code)
	}
}
