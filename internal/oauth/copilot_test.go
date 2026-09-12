package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsTrustedCopilotAPIHost(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"https://api.individual.githubcopilot.com", true},
		{"https://api.business.githubcopilot.com/", true},
		{"https://copilot-proxy.githubusercontent.com", true},
		{"http://api.individual.githubcopilot.com", false}, // plain http
		{"https://github.com", false},                      // not a copilot host
		{"https://evil.com/githubcopilot.com", false},      // suffix game
		{"https://api.githubcopilot.com.evil.com", false},  // suffix game
		{"https://user:pass@api.githubcopilot.com", false}, // userinfo
		{"https://api.githubcopilot.com/x?query=1", false}, // query
		{"https://169.254.169.254", false},                 // metadata
		{"localhost", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isTrustedCopilotAPIHost(tt.raw); got != tt.want {
			t.Errorf("isTrustedCopilotAPIHost(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestValidateCopilotToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gho_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"login":     "mona",
			"endpoints": map[string]string{"api": "https://api.individual.githubcopilot.com"},
		})
	}))
	defer srv.Close()

	old := copilotUserURL
	copilotUserURL = srv.URL
	t.Cleanup(func() { copilotUserURL = old })

	res, err := ValidateCopilotToken(context.Background(), "gho_test")
	if err != nil {
		t.Fatalf("ValidateCopilotToken: %v", err)
	}
	if res.Login != "mona" {
		t.Errorf("login = %q", res.Login)
	}
	if res.APIBase != "https://api.individual.githubcopilot.com" {
		t.Errorf("api base = %q", res.APIBase)
	}

	// Untrusted endpoint falls back to the default base, never followed.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"login":     "mona",
			"endpoints": map[string]string{"api": "https://evil.example.com/api"},
		})
	}))
	defer srv2.Close()
	copilotUserURL = srv2.URL
	defer func() { copilotUserURL = srv.URL }()

	res, err = ValidateCopilotToken(context.Background(), "gho_test")
	if err != nil {
		t.Fatalf("ValidateCopilotToken (untrusted): %v", err)
	}
	if res.APIBase != CopilotAPIBaseDefault {
		t.Errorf("untrusted endpoint must fall back to default, got %q", res.APIBase)
	}
}

func TestValidateCopilotTokenNoAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	old := copilotUserURL
	copilotUserURL = srv.URL
	t.Cleanup(func() { copilotUserURL = old })

	if _, err := ValidateCopilotToken(context.Background(), "gho_none"); err == nil {
		t.Fatalf("expected error for token without Copilot access")
	}
}

func TestStartLoginCopilotDeviceFlow(t *testing.T) {
	pollCount := 0
	deviceSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("client_id") != CopilotClientID || r.PostForm.Get("scope") != CopilotScope {
			t.Errorf("unexpected device code form: %v", r.PostForm)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dc-1",
			"user_code":        "ABCD-1234",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900,
			"interval":         1,
		})
	}))
	defer deviceSrv.Close()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		pollCount++
		switch {
		case pollCount <= 2:
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
		case pollCount == 3:
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_ok", "token_type": "bearer"})
		}
	}))
	defer tokenSrv.Close()

	oldDev, oldTok := copilotDeviceCodeURL, copilotAccessTokenURL
	copilotDeviceCodeURL, copilotAccessTokenURL = deviceSrv.URL, tokenSrv.URL
	t.Cleanup(func() { copilotDeviceCodeURL, copilotAccessTokenURL = oldDev, oldTok })

	pending, err := StartLoginCopilot()
	if err != nil {
		t.Fatalf("StartLoginCopilot: %v", err)
	}
	defer pending.Shutdown()
	if pending.UserCode != "ABCD-1234" || pending.VerificationURI != "https://github.com/login/device" {
		t.Fatalf("unexpected pending: %+v", pending)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := pending.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if resp.AccessToken != "gho_ok" {
		t.Fatalf("access token = %q", resp.AccessToken)
	}
	if !strings.Contains(pending.deviceCode, "dc-") {
		t.Errorf("device code not retained")
	}
}

func TestCopilotTokenSourceSaveTokenStatus(t *testing.T) {
	provStore := newMockProviderStore()
	ts := NewCopilotDBTokenSource(provStore, "github-copilot").WithProviderMeta("My Copilot", "")

	id, err := ts.SaveOAuthResult(context.Background(), "gho_x", "https://api.individual.githubcopilot.com", "mona")
	if err != nil {
		t.Fatalf("SaveOAuthResult: %v", err)
	}
	if id.String() == "" {
		t.Fatalf("empty provider id")
	}

	p, _ := provStore.GetProviderByName(context.Background(), "github-copilot")
	if p.ProviderType != "copilot_oauth" || p.APIKey != "gho_x" {
		t.Fatalf("row = %+v", p)
	}
	settings := parseOAuthSettings(p.Settings)
	if settings.AccountID != "mona" {
		t.Errorf("account login = %q", settings.AccountID)
	}

	got, err := ts.Token()
	if err != nil || got != "gho_x" {
		t.Fatalf("Token = %q err = %v", got, err)
	}
	if !ts.Exists(context.Background()) {
		t.Errorf("Exists = false")
	}
	if err := ts.Delete(context.Background()); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if ts.Exists(context.Background()) {
		t.Errorf("Exists after delete = true")
	}
}
