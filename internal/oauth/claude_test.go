package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseClaudeRedirectURL(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantCode string
		wantSate string
		wantErr  bool
	}{
		{name: "query form", raw: "https://console.anthropic.com/oauth/code/callback?code=abc123&state=xyz", wantCode: "abc123", wantSate: "xyz"},
		{name: "fragment form", raw: "https://console.anthropic.com/oauth/code/callback#code=abc123&state=xyz", wantCode: "abc123", wantSate: "xyz"},
		{name: "bare code#state", raw: "abc123#xyz", wantCode: "abc123", wantSate: "xyz"},
		{name: "encoded fragment", raw: "https://console.anthropic.com/oauth/code/callback%23code=abc123&state=xyz", wantCode: "abc123", wantSate: "xyz"},
		{name: "whitespace trimmed", raw: "  abc123#xyz\n", wantCode: "abc123", wantSate: "xyz"},
		{name: "empty", raw: "   ", wantErr: true},
		{name: "no code", raw: "https://console.anthropic.com/oauth/code/callback?state=xyz", wantErr: true},
		{name: "error param", raw: "https://console.anthropic.com/oauth/code/callback?error=access_denied", wantErr: true},
		{name: "code only no state", raw: "https://console.anthropic.com/oauth/code/callback?code=abc123", wantCode: "abc123", wantSate: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, state, err := ParseClaudeRedirectURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got code=%q state=%q", code, state)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != tt.wantCode || state != tt.wantSate {
				t.Fatalf("got code=%q state=%q, want code=%q state=%q", code, state, tt.wantCode, tt.wantSate)
			}
		})
	}
}

func TestStartLoginClaude(t *testing.T) {
	pending, err := StartLoginClaude()
	if err != nil {
		t.Fatalf("StartLoginClaude: %v", err)
	}
	u, err := url.Parse(pending.AuthURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	q := u.Query()
	if got := q.Get("client_id"); got != ClaudeClientID {
		t.Errorf("client_id = %q", got)
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q", got)
	}
	if got := q.Get("code"); got != "true" {
		t.Errorf("code = %q, want true", got)
	}
	if got := q.Get("redirect_uri"); got != ClaudeRedirectURI {
		t.Errorf("redirect_uri = %q", got)
	}
	if q.Get("code_challenge") == "" || q.Get("state") == "" {
		t.Errorf("missing PKCE challenge or state")
	}
	if !strings.HasPrefix(u.String(), ClaudeAuthorizeURL+"?") {
		t.Errorf("authorize host = %q", u.String())
	}
}

func TestClaudeExchangeAndStateCheck(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_ = json.NewEncoder(w).Encode(ClaudeTokenResponse{
			AccessToken: "at-1", RefreshToken: "rt-1", ExpiresIn: 3600, Scope: "user:inference",
		})
	}))
	defer srv.Close()

	old := claudeOAuthCfg
	SetClaudeOAuthConfig(ClaudeOAuthConfig{TokenURL: srv.URL})
	t.Cleanup(func() { claudeOAuthCfg = old })

	pending, err := StartLoginClaude()
	if err != nil {
		t.Fatalf("StartLoginClaude: %v", err)
	}

	// Wrong state must be rejected before any network call.
	if _, err := pending.ExchangeRedirectURL("realcode#wrongstate"); err == nil {
		t.Fatalf("expected state mismatch error")
	}

	resp, err := pending.ExchangeRedirectURL("realcode#" + pending.State())
	if err != nil {
		t.Fatalf("ExchangeRedirectURL: %v", err)
	}
	if resp.AccessToken != "at-1" || resp.RefreshToken != "rt-1" {
		t.Fatalf("unexpected token response: %+v", resp)
	}
	if got := gotForm.Get("code"); got != "realcode" {
		t.Errorf("exchanged code = %q", got)
	}
	if got := gotForm.Get("code_verifier"); got != pending.verifier {
		t.Errorf("code_verifier mismatch")
	}
	if got := gotForm.Get("grant_type"); got != "authorization_code" {
		t.Errorf("grant_type = %q", got)
	}
}

func TestRefreshClaudeToken(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_ = json.NewEncoder(w).Encode(ClaudeTokenResponse{AccessToken: "at-2", RefreshToken: "rt-2", ExpiresIn: 7200})
	}))
	defer srv.Close()

	old := claudeOAuthCfg
	SetClaudeOAuthConfig(ClaudeOAuthConfig{TokenURL: srv.URL})
	t.Cleanup(func() { claudeOAuthCfg = old })

	resp, err := RefreshClaudeToken(context.Background(), "rt-old")
	if err != nil {
		t.Fatalf("RefreshClaudeToken: %v", err)
	}
	if resp.AccessToken != "at-2" {
		t.Fatalf("access token = %q", resp.AccessToken)
	}
	if got := gotForm.Get("grant_type"); got != "refresh_token" {
		t.Errorf("grant_type = %q", got)
	}
	if got := gotForm.Get("refresh_token"); got != "rt-old" {
		t.Errorf("refresh_token = %q", got)
	}
}

func TestClaudeRefreshTokenSecretKeyNamespaced(t *testing.T) {
	if got := ClaudeRefreshTokenSecretKey("claude-pro"); got != "oauth.claude.claude-pro.refresh_token" {
		t.Fatalf("key = %q", got)
	}
	// Must NOT collide with the OpenAI source's legacy literal key.
	if got := ClaudeRefreshTokenSecretKey("openai-codex"); got == RefreshTokenSecretKey("openai-codex") {
		t.Fatalf("claude key collides with openai key: %q", got)
	}
}
