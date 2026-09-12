package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type claudeStaticTokenSource struct{ token string }

func (s claudeStaticTokenSource) Token() (string, error) { return s.token, nil }

// TestClaudeOAuthProviderTransport pins the OAuth wire contract: Bearer auth
// (no x-api-key), the OAuth beta merged with the thinking beta, and no
// service_tier injection (Anthropic rejects it on OAuth tokens).
func TestClaudeOAuthProviderTransport(t *testing.T) {
	var got http.Header
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	p := NewClaudeOAuthProvider("claude-pro", claudeStaticTokenSource{token: "oauth-tok-1"}, srv.URL, "", nil)
	resp, err := p.Chat(context.Background(), ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Options: map[string]any{
			OptThinkingLevel: "medium",
			OptFastMode:      true, // would inject service_tier on API-key auth
			OptServiceTier:   "auto",
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp == nil || resp.Content == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	if got := got.Get("Authorization"); got != "Bearer oauth-tok-1" {
		t.Errorf("Authorization = %q, want Bearer oauth-tok-1", got)
	}
	if got.Get("x-api-key") != "" {
		t.Errorf("x-api-key must be absent on OAuth requests, got %q", got.Get("x-api-key"))
	}
	beta := got.Get("anthropic-beta")
	if !strings.Contains(beta, AnthropicOAuthBeta) {
		t.Errorf("anthropic-beta %q missing %s", beta, AnthropicOAuthBeta)
	}
	if !strings.Contains(beta, "interleaved-thinking-2025-05-14") {
		t.Errorf("anthropic-beta %q missing thinking beta", beta)
	}
	if _, has := gotBody["service_tier"]; has {
		t.Errorf("service_tier must not be sent on OAuth requests")
	}
	if _, has := gotBody["thinking"]; !has {
		t.Errorf("expected thinking block in request body (drives the thinking beta)")
	}
}

// TestNormalizeClaudeAPIBase covers the restart path: the DB row stores the
// bare host (ClaudeAPIBase without /v1) while doRequest appends "/messages".
func TestNormalizeClaudeAPIBase(t *testing.T) {
	if got := normalizeClaudeAPIBase("https://api.anthropic.com"); got != "https://api.anthropic.com/v1" {
		t.Fatalf("normalized base = %q", got)
	}
	if got := normalizeClaudeAPIBase("https://api.anthropic.com/"); got != "https://api.anthropic.com/v1" {
		t.Fatalf("normalized trailing-slash base = %q", got)
	}
	if got := normalizeClaudeAPIBase("http://127.0.0.1:9999"); got != "http://127.0.0.1:9999" {
		t.Fatalf("custom base must be untouched, got %q", got)
	}
}
