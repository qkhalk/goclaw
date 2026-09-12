package oauth

// ClaudeDBTokenSource mirrors DBTokenSource (token.go) for claude_oauth
// providers: access token in llm_providers.api_key, refresh token in
// config_secrets, expiry in settings JSONB, on-demand refresh with the same
// 5-minute margin and stale-token fallback. No quota RouteEligibility — the
// ChatGPT-specific usage endpoint does not exist on Anthropic.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// DefaultClaudeProviderName is the suggested alias for the first Claude OAuth account.
const DefaultClaudeProviderName = "claude-pro"

// claudeRefreshSecretPrefix namespaces refresh tokens away from the generic
// oauth.<name>.refresh_token keys: a Claude provider aliased "openai-codex"
// must never collide with the OpenAI refresh secret.
const claudeRefreshSecretPrefix = "oauth.claude."

// ClaudeRefreshTokenSecretKey returns the namespaced secret key.
func ClaudeRefreshTokenSecretKey(providerName string) string {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		providerName = DefaultClaudeProviderName
	}
	return claudeRefreshSecretPrefix + providerName + ".refresh_token"
}

var refreshClaudeTokenFunc = RefreshClaudeToken

// ClaudeDBTokenSource provides valid Claude OAuth access tokens from the DB.
// Implements providers.TokenSource.
type ClaudeDBTokenSource struct {
	providerStore store.ProviderStore
	secretsStore  store.ConfigSecretsStore
	providerName  string
	tenantID      uuid.UUID

	providerDisplayName string
	providerAPIBase     string

	mu          sync.Mutex
	cachedToken string
	expiresAt   time.Time
}

// NewClaudeDBTokenSource creates a DB-backed Claude OAuth token source.
func NewClaudeDBTokenSource(provStore store.ProviderStore, secretsStore store.ConfigSecretsStore, providerName string) *ClaudeDBTokenSource {
	if strings.TrimSpace(providerName) == "" {
		providerName = DefaultClaudeProviderName
	}
	return &ClaudeDBTokenSource{
		providerStore: provStore,
		secretsStore:  secretsStore,
		providerName:  providerName,
		tenantID:      store.MasterTenantID,
	}
}

// WithTenantID sets the tenant context for DB queries. Call at init time.
func (ts *ClaudeDBTokenSource) WithTenantID(tenantID uuid.UUID) *ClaudeDBTokenSource {
	ts.tenantID = tenantID
	return ts
}

// WithProviderMeta sets defaults used when creating or updating the provider row.
func (ts *ClaudeDBTokenSource) WithProviderMeta(displayName, apiBase string) *ClaudeDBTokenSource {
	if strings.TrimSpace(displayName) != "" {
		ts.providerDisplayName = strings.TrimSpace(displayName)
	}
	if strings.TrimSpace(apiBase) != "" {
		ts.providerAPIBase = strings.TrimSpace(apiBase)
	}
	return ts
}

func (ts *ClaudeDBTokenSource) resolvedAPIBase() string {
	if ts.providerAPIBase != "" {
		return ts.providerAPIBase
	}
	return claudeOAuthConfig().APIBase
}

func (ts *ClaudeDBTokenSource) loadClaudeOAuthProvider(ctx context.Context) (*store.LLMProviderData, error) {
	p, err := ts.providerStore.GetProviderByName(ctx, ts.providerName)
	if err != nil {
		return nil, err
	}
	if p.ProviderType != store.ProviderClaudeOAuth {
		return nil, &ProviderTypeConflictError{
			ProviderName: ts.providerName,
			ProviderType: p.ProviderType,
		}
	}
	return p, nil
}

// Token returns a valid access token, refreshing if expired or about to expire.
func (ts *ClaudeDBTokenSource) Token() (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.cachedToken != "" && time.Until(ts.expiresAt) > refreshMargin {
		return ts.cachedToken, nil
	}

	ctx := store.WithTenantID(context.Background(), ts.tenantID)

	if ts.cachedToken == "" {
		p, err := ts.loadClaudeOAuthProvider(ctx)
		if err != nil {
			return "", fmt.Errorf("load claude oauth provider %q: %w", ts.providerName, err)
		}
		ts.cachedToken = p.APIKey
		settings := parseOAuthSettings(p.Settings)
		if settings.ExpiresAt > 0 {
			ts.expiresAt = time.Unix(settings.ExpiresAt, 0)
		}
	}

	if time.Until(ts.expiresAt) < refreshMargin {
		if err := ts.refresh(ctx); err != nil {
			// Refresh failed but the cached token may still work.
			if ts.cachedToken != "" {
				slog.Warn("claude oauth token refresh failed, using existing token", "error", err)
				return ts.cachedToken, nil
			}
			return "", fmt.Errorf("refresh claude oauth token: %w", err)
		}
	}

	return ts.cachedToken, nil
}

// refresh rotates the access token via config_secrets + the token endpoint.
func (ts *ClaudeDBTokenSource) refresh(ctx context.Context) error {
	refreshToken, err := ts.secretsStore.Get(ctx, ClaudeRefreshTokenSecretKey(ts.providerName))
	if err != nil {
		return fmt.Errorf("get refresh token: %w", err)
	}

	slog.Info("refreshing Claude OAuth token")
	newToken, err := refreshClaudeTokenFunc(ctx, refreshToken)
	if err != nil {
		return err
	}

	ts.cachedToken = newToken.AccessToken
	if newToken.ExpiresIn > 0 {
		ts.expiresAt = time.Now().Add(time.Duration(newToken.ExpiresIn) * time.Second)
	} else {
		// Anthropic has returned omit-expiry responses; assume 8h and rely on
		// the stale-token fallback if refresh later fails.
		ts.expiresAt = time.Now().Add(8 * time.Hour)
	}

	p, err := ts.loadClaudeOAuthProvider(ctx)
	if err != nil {
		return fmt.Errorf("get provider for update: %w", err)
	}

	settings := parseOAuthSettings(p.Settings)
	settings.ExpiresAt = ts.expiresAt.Unix()
	if newToken.Scope != "" {
		settings.Scopes = newToken.Scope
	}

	if err := ts.providerStore.UpdateProvider(ctx, p.ID, map[string]any{
		"api_key":  newToken.AccessToken,
		"settings": marshalOAuthSettingsInto(p.Settings, settings),
	}); err != nil {
		slog.Warn("failed to persist refreshed claude access token", "error", err)
	}

	if newToken.RefreshToken != "" {
		if err := ts.secretsStore.Set(ctx, ClaudeRefreshTokenSecretKey(ts.providerName), newToken.RefreshToken); err != nil {
			slog.Warn("failed to persist new claude refresh token", "error", err)
		}
	}

	return nil
}

// SaveOAuthResult persists the Claude OAuth tokens after a successful exchange:
// upserts the llm_providers row and stores the refresh token in config_secrets.
func (ts *ClaudeDBTokenSource) SaveOAuthResult(ctx context.Context, tokenResp *ClaudeTokenResponse, accountEmail string) (uuid.UUID, error) {
	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int((8 * time.Hour).Seconds())
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	ts.mu.Lock()
	ts.cachedToken = tokenResp.AccessToken
	ts.expiresAt = expiresAt
	ts.mu.Unlock()

	settings := OAuthSettings{
		ExpiresAt:    expiresAt.Unix(),
		Scopes:       tokenResp.Scope,
		AccountEmail: strings.TrimSpace(accountEmail),
	}

	existing, err := ts.loadClaudeOAuthProvider(ctx)
	if err == nil {
		updates := map[string]any{
			"api_key":  tokenResp.AccessToken,
			"settings": marshalOAuthSettingsInto(existing.Settings, settings),
			"enabled":  true,
		}
		if ts.providerDisplayName != "" {
			updates["display_name"] = ts.providerDisplayName
		}
		if ts.providerAPIBase != "" {
			updates["api_base"] = ts.providerAPIBase
		}
		if err := ts.providerStore.UpdateProvider(ctx, existing.ID, updates); err != nil {
			return uuid.Nil, fmt.Errorf("update provider: %w", err)
		}
		if tokenResp.RefreshToken != "" {
			if err := ts.secretsStore.Set(ctx, ClaudeRefreshTokenSecretKey(ts.providerName), tokenResp.RefreshToken); err != nil {
				return uuid.Nil, fmt.Errorf("save refresh token: %w", err)
			}
		}
		return existing.ID, nil
	}
	if _, ok := err.(*ProviderTypeConflictError); ok {
		return uuid.Nil, err
	}

	p := &store.LLMProviderData{
		Name:         ts.providerName,
		DisplayName:  ts.providerDisplayName,
		ProviderType: store.ProviderClaudeOAuth,
		APIBase:      ts.resolvedAPIBase(),
		APIKey:       tokenResp.AccessToken,
		Enabled:      true,
		Settings:     marshalOAuthSettings(settings),
	}
	if err := ts.providerStore.CreateProvider(ctx, p); err != nil {
		return uuid.Nil, fmt.Errorf("create provider: %w", err)
	}
	if tokenResp.RefreshToken != "" {
		if err := ts.secretsStore.Set(ctx, ClaudeRefreshTokenSecretKey(ts.providerName), tokenResp.RefreshToken); err != nil {
			return uuid.Nil, fmt.Errorf("save refresh token: %w", err)
		}
	}
	return p.ID, nil
}

// Delete removes the provider row and its refresh token from config_secrets.
func (ts *ClaudeDBTokenSource) Delete(ctx context.Context) error {
	ts.mu.Lock()
	ts.cachedToken = ""
	ts.expiresAt = time.Time{}
	ts.mu.Unlock()

	_ = ts.secretsStore.Delete(ctx, ClaudeRefreshTokenSecretKey(ts.providerName))

	p, err := ts.loadClaudeOAuthProvider(ctx)
	if err != nil {
		if _, ok := err.(*ProviderTypeConflictError); ok {
			return err
		}
		return nil // already gone
	}
	return ts.providerStore.DeleteProvider(ctx, p.ID)
}

// Exists checks if a Claude OAuth provider exists and has a token.
func (ts *ClaudeDBTokenSource) Exists(ctx context.Context) bool {
	p, err := ts.loadClaudeOAuthProvider(ctx)
	return err == nil && p.APIKey != ""
}
