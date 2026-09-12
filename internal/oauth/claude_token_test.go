package oauth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestClaudeTokenSourceSaveAndToken(t *testing.T) {
	provStore := newMockProviderStore()
	secretStore := newMockSecretsStore()

	resp := &ClaudeTokenResponse{AccessToken: "at-1", RefreshToken: "rt-1", ExpiresIn: 3600, Scope: "user:inference"}
	ts := NewClaudeDBTokenSource(provStore, secretStore, "claude-pro").WithProviderMeta("My Claude", "")

	id, err := ts.SaveOAuthResult(context.Background(), resp, "me@example.com")
	if err != nil {
		t.Fatalf("SaveOAuthResult: %v", err)
	}
	if id == uuid.Nil {
		t.Fatalf("bad provider id %v", id)
	}

	p, err := provStore.GetProviderByName(context.Background(), "claude-pro")
	if err != nil {
		t.Fatalf("provider not saved: %v", err)
	}
	if p.ProviderType != store.ProviderClaudeOAuth {
		t.Errorf("provider_type = %q", p.ProviderType)
	}
	if p.APIKey != "at-1" {
		t.Errorf("api_key = %q", p.APIKey)
	}
	settings := parseOAuthSettings(p.Settings)
	if settings.AccountEmail != "me@example.com" {
		t.Errorf("account_email = %q", settings.AccountEmail)
	}
	if got := secretStore.data[ClaudeRefreshTokenSecretKey("claude-pro")]; got != "rt-1" {
		t.Errorf("refresh token secret = %q", got)
	}

	// Token() served from the saved row without refresh.
	got, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "at-1" {
		t.Errorf("Token = %q", got)
	}
}

func TestClaudeTokenSourceRefreshOnDemand(t *testing.T) {
	provStore := newMockProviderStore()
	secretStore := newMockSecretsStore()

	// Saved with an already-expired token → Token() must refresh.
	oldResp := &ClaudeTokenResponse{AccessToken: "stale", RefreshToken: "rt-old", ExpiresIn: 60}
	ts := NewClaudeDBTokenSource(provStore, secretStore, "claude-pro")
	if _, err := ts.SaveOAuthResult(context.Background(), oldResp, ""); err != nil {
		t.Fatalf("SaveOAuthResult: %v", err)
	}

	refreshes := 0
	oldFunc := refreshClaudeTokenFunc
	refreshClaudeTokenFunc = func(_ context.Context, refreshToken string) (*ClaudeTokenResponse, error) {
		refreshes++
		if refreshToken != "rt-old" {
			t.Errorf("refresh called with %q", refreshToken)
		}
		return &ClaudeTokenResponse{AccessToken: "fresh", RefreshToken: "rt-new", ExpiresIn: 3600}, nil
	}
	t.Cleanup(func() { refreshClaudeTokenFunc = oldFunc })

	got, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "fresh" || refreshes != 1 {
		t.Fatalf("Token = %q refreshes = %d", got, refreshes)
	}
	// Persisted.
	p, _ := provStore.GetProviderByName(context.Background(), "claude-pro")
	if p.APIKey != "fresh" {
		t.Errorf("persisted api_key = %q", p.APIKey)
	}
	if got := secretStore.data[ClaudeRefreshTokenSecretKey("claude-pro")]; got != "rt-new" {
		t.Errorf("rotated refresh token = %q", got)
	}
	// Second call served from cache (no new refresh).
	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token 2: %v", err)
	}
	if refreshes != 1 {
		t.Errorf("unexpected extra refreshes: %d", refreshes)
	}
}

func TestClaudeTokenSourceStaleFallbackOnRefreshFailure(t *testing.T) {
	provStore := newMockProviderStore()
	secretStore := newMockSecretsStore()

	resp := &ClaudeTokenResponse{AccessToken: "stale-but-present", RefreshToken: "rt", ExpiresIn: 30}
	ts := NewClaudeDBTokenSource(provStore, secretStore, "claude-pro")
	if _, err := ts.SaveOAuthResult(context.Background(), resp, ""); err != nil {
		t.Fatalf("SaveOAuthResult: %v", err)
	}

	oldFunc := refreshClaudeTokenFunc
	refreshClaudeTokenFunc = func(_ context.Context, _ string) (*ClaudeTokenResponse, error) {
		return nil, errors.New("endpoint down")
	}
	t.Cleanup(func() { refreshClaudeTokenFunc = oldFunc })

	got, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "stale-but-present" {
		t.Errorf("expected stale fallback, got %q", got)
	}
}

func TestClaudeTokenSourceTypeConflict(t *testing.T) {
	provStore := newMockProviderStore()
	secretStore := newMockSecretsStore()
	_ = provStore.CreateProvider(context.Background(), &store.LLMProviderData{
		Name: "dup", ProviderType: store.ProviderAnthropicNative, APIKey: "sk",
	})
	ts := NewClaudeDBTokenSource(provStore, secretStore, "dup")
	if ts.Exists(context.Background()) {
		t.Fatalf("Exists true for a non-claude provider")
	}
	var conflict *ProviderTypeConflictError
	if _, err := ts.Token(); err == nil || !errors.As(err, &conflict) {
		t.Fatalf("expected ProviderTypeConflictError, got %v", err)
	}
}
