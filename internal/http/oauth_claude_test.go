package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func newTestClaudeOAuthHandler(t *testing.T) *ClaudeOAuthHandler {
	t.Helper()
	old := pkgGatewayToken
	pkgGatewayToken = ""
	t.Cleanup(func() { pkgGatewayToken = old })
	return NewClaudeOAuthHandler(newMockProviderStore(), newMockSecretsStore(), nil, nil)
}

func TestClaudeOAuthHandlerStatusNoToken(t *testing.T) {
	h := newTestClaudeOAuthHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/v1/auth/claude/claude-pro/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}
	var result map[string]any
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result["authenticated"] != false {
		t.Errorf("authenticated = %v, want false", result["authenticated"])
	}
}

func TestClaudeOAuthHandlerStart(t *testing.T) {
	h := newTestClaudeOAuthHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/v1/auth/claude/claude-pro/start", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("start code = %d body=%s", w.Code, w.Body.String())
	}
	var result map[string]any
	_ = json.NewDecoder(w.Body).Decode(&result)
	authURL, _ := result["auth_url"].(string)
	if authURL == "" {
		t.Fatalf("missing auth_url in %v", result)
	}
	if !strings.Contains(authURL, "code_challenge_method=S256") {
		t.Errorf("auth URL missing PKCE challenge: %q", authURL)
	}

	// Second start returns the same pending auth_url (no 409 — paste-only flow).
	req2 := httptest.NewRequest("POST", "/v1/auth/claude/claude-pro/start", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second start code = %d", w2.Code)
	}
	var result2 map[string]any
	_ = json.NewDecoder(w2.Body).Decode(&result2)
	if result2["auth_url"] != authURL {
		t.Errorf("second start returned a different flow")
	}
}

func TestClaudeOAuthHandlerCallbackNoPending(t *testing.T) {
	h := newTestClaudeOAuthHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/v1/auth/claude/claude-pro/callback", strings.NewReader(`{"redirect_url":"https://console.anthropic.com/oauth/code/callback?code=a&state=b"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("callback without pending flow: code = %d body=%s", w.Code, w.Body.String())
	}
}

func TestClaudeOAuthHandlerTypeConflict(t *testing.T) {
	provStore := newMockProviderStore()
	_ = provStore.CreateProvider(nil, &store.LLMProviderData{Name: "dup", ProviderType: store.ProviderAnthropicNative, APIKey: "sk-1"})

	old := pkgGatewayToken
	pkgGatewayToken = ""
	t.Cleanup(func() { pkgGatewayToken = old })

	h := NewClaudeOAuthHandler(provStore, newMockSecretsStore(), nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/v1/auth/claude/dup/start", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("conflict code = %d body=%s", w.Code, w.Body.String())
	}
}
