package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/oauth"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func newTestCopilotOAuthHandler(t *testing.T) *CopilotOAuthHandler {
	t.Helper()
	old := pkgGatewayToken
	pkgGatewayToken = ""
	t.Cleanup(func() { pkgGatewayToken = old })
	return NewCopilotOAuthHandler(newMockProviderStore(), nil, nil)
}

func TestCopilotOAuthHandlerStatusNoToken(t *testing.T) {
	h := newTestCopilotOAuthHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/v1/auth/copilot/github-copilot/status", nil)
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

func TestCopilotOAuthHandlerStart(t *testing.T) {
	// Stub the device flow so the test never touches github.com.
	oldStart := startLoginCopilotFunc
	startLoginCopilotFunc = func() (*oauth.PendingCopilotLogin, error) {
		return &oauth.PendingCopilotLogin{UserCode: "TEST-CODE", VerificationURI: "https://example.com/device"}, nil
	}
	t.Cleanup(func() { startLoginCopilotFunc = oldStart })

	h := newTestCopilotOAuthHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/v1/auth/copilot/github-copilot/start", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("start code = %d body=%s", w.Code, w.Body.String())
	}
	var result map[string]any
	_ = json.NewDecoder(w.Body).Decode(&result)
	userCode, _ := result["user_code"].(string)
	verificationURI, _ := result["verification_uri"].(string)
	if userCode == "" || verificationURI == "" {
		t.Fatalf("missing user_code/verification_uri in %v", result)
	}

	// A second start returns the same in-flight device flow (no duplicate poller).
	req2 := httptest.NewRequest("POST", "/v1/auth/copilot/github-copilot/start", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second start code = %d", w2.Code)
	}
	var result2 map[string]any
	_ = json.NewDecoder(w2.Body).Decode(&result2)
	if result2["user_code"] != userCode {
		t.Errorf("second start spawned a different flow")
	}

	// Logout cancels the in-flight flow.
	req3 := httptest.NewRequest("POST", "/v1/auth/copilot/github-copilot/logout", nil)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("logout code = %d", w3.Code)
	}
}

func TestCopilotOAuthHandlerTypeConflict(t *testing.T) {
	provStore := newMockProviderStore()
	_ = provStore.CreateProvider(nil, &store.LLMProviderData{Name: "dup", ProviderType: store.ProviderOpenAICompat, APIKey: "sk-1"})

	h := newTestCopilotOAuthHandler(t)
	// Rebuild with the store holding a conflicting row.
	h = NewCopilotOAuthHandler(provStore, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/v1/auth/copilot/dup/start", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("conflict code = %d body=%s", w.Code, w.Body.String())
	}
}
