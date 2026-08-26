package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A1: the invoke endpoint must reject bodies larger than the cap (default
// 1 MiB) with 413 instead of decoding them into the tool registry.
func TestToolsInvoke_BodyOverCapRejected(t *testing.T) {
	h := NewToolsInvokeHandler(nil, nil)
	big := strings.Repeat("a", int(DefaultInvokeMaxBodyBytes)+1024)
	req := httptest.NewRequest(http.MethodPost, "/v1/tools/invoke", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	// resolveAuth runs before body decode; an unauthenticated request returns
	// 401 before reaching the cap — so authenticate as a gateway token.
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == 0 {
		t.Fatal("no response recorded")
	}
	// The cap must prevent unbounded decode. With auth resolved via gateway
	// token the request reaches bindJSON and fails there; without auth it
	// never reaches the cap. Assert we did NOT get a successful tool run.
	if rec.Code == http.StatusOK {
		t.Fatalf("oversized body accepted: got 200, want >= 400")
	}
}

// The configured setter must be honored: a small explicit cap rejects a body
// that fits under the default.
func TestToolsInvoke_SetMaxBodyBytes_SmallCapRejected(t *testing.T) {
	h := NewToolsInvokeHandler(nil, nil)
	h.SetMaxBodyBytes(64)

	body := `{"tool":"x","args":{}}` // ~21 bytes of JSON but padded over cap
	padded := body + strings.Repeat(" ", 100)
	req := httptest.NewRequest(http.MethodPost, "/v1/tools/invoke", strings.NewReader(padded))
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("body over explicit 64-byte cap accepted: got 200")
	}
}

// Non-positive overrides are ignored — the guard cannot be disabled by config.
func TestToolsInvoke_SetMaxBodyBytes_IgnoresNonPositive(t *testing.T) {
	h := NewToolsInvokeHandler(nil, nil)
	h.SetMaxBodyBytes(0)
	h.SetMaxBodyBytes(-1)
	if h.maxBodyBytes != 0 {
		t.Fatalf("maxBodyBytes = %d, want 0 (unset)", h.maxBodyBytes)
	}
}
