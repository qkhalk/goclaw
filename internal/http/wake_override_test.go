package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/crypto"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// wakeHarness authenticates through the real auth stack (tenant-scoped API
// key) so handleWake sees exactly what production sees, including the
// enrichContext values the override gate reads.
func wakeHarness(t *testing.T, apiKey string, keyTenant uuid.UUID, ownerID string) *httptest.ResponseRecorder {
	t.Helper()
	setupTestCache(t, map[string]*store.APIKeyData{
		crypto.HashAPIKey(apiKey): {
			ID:       uuid.New(),
			Scopes:   []string{"operator.write"},
			TenantID: keyTenant,
			OwnerID:  ownerID,
		},
	})
	setupTestToken(t, "unrelated-gateway-token")

	body := `{"message":"hi","user_id":"victim-user"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/a1/wake", strings.NewReader(body))
	req.SetPathValue("id", "a1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	h := NewWakeHandler(agent.NewRouter())
	rec := httptest.NewRecorder()
	h.handleWake(rec, req)
	return rec
}

// A2: a body user_id different from the authenticated user must be rejected
// with 403 for non-master-scope callers instead of silently impersonating.
func TestWake_UserOverrideBlockedForTenantScope(t *testing.T) {
	rec := wakeHarness(t, "tenant-scoped-key-1", uuid.New(), "owner-1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("tenant-scoped wake with foreign user_id: got %d (%s), want 403 impersonation block", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "master scope") {
		t.Fatalf("403 body does not name the override rule: %s", rec.Body.String())
	}
}

// Master-scope callers (system-level API key: no tenant binding) pass the
// override gate and reach the agent lookup — with an empty router that is a
// 404, never the 403 override rejection.
func TestWake_UserOverrideAllowedForMasterScope(t *testing.T) {
	rec := wakeHarness(t, "system-level-key-1", uuid.Nil, "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("master-scope wake with foreign user_id: got %d (%s), want 404 (past the override gate, agent unknown)", rec.Code, rec.Body.String())
	}
}

// An API key bound to an owner must not be able to override user_id even to
// its own owner id via the body — identity comes from the key binding only.
func TestWake_OwnerBoundKeyCannotOverrideUser(t *testing.T) {
	rec := wakeHarness(t, "owner-bound-key-1", uuid.New(), "bound-owner")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("owner-bound key body override: got %d (%s), want 403", rec.Code, rec.Body.String())
	}
}
