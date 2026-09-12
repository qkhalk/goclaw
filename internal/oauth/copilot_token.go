package oauth

// CopilotDBTokenSource serves the static GitHub OAuth token stored on the
// copilot_oauth provider row. GitHub device-flow tokens are long-lived, so
// unlike the Claude/OpenAI sources there is no refresh path — revalidation
// against copilot_internal/user happens at login and on auth failures.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// DefaultCopilotProviderName is the suggested alias for the first Copilot account.
const DefaultCopilotProviderName = "github-copilot"

type CopilotDBTokenSource struct {
	providerStore store.ProviderStore
	providerName  string
	tenantID      uuid.UUID

	providerDisplayName string
	providerAPIBase     string

	mu          sync.Mutex
	cachedToken string
}

func NewCopilotDBTokenSource(provStore store.ProviderStore, providerName string) *CopilotDBTokenSource {
	if strings.TrimSpace(providerName) == "" {
		providerName = DefaultCopilotProviderName
	}
	return &CopilotDBTokenSource{
		providerStore: provStore,
		providerName:  providerName,
		tenantID:      store.MasterTenantID,
	}
}

func (ts *CopilotDBTokenSource) WithTenantID(tenantID uuid.UUID) *CopilotDBTokenSource {
	ts.tenantID = tenantID
	return ts
}

func (ts *CopilotDBTokenSource) WithProviderMeta(displayName, apiBase string) *CopilotDBTokenSource {
	if strings.TrimSpace(displayName) != "" {
		ts.providerDisplayName = strings.TrimSpace(displayName)
	}
	if strings.TrimSpace(apiBase) != "" {
		ts.providerAPIBase = strings.TrimSpace(apiBase)
	}
	return ts
}

func (ts *CopilotDBTokenSource) loadCopilotOAuthProvider(ctx context.Context) (*store.LLMProviderData, error) {
	p, err := ts.providerStore.GetProviderByName(ctx, ts.providerName)
	if err != nil {
		return nil, err
	}
	if p.ProviderType != store.ProviderCopilotOAuth {
		return nil, &ProviderTypeConflictError{
			ProviderName: ts.providerName,
			ProviderType: p.ProviderType,
		}
	}
	return p, nil
}

// Token returns the stored GitHub token (no TTL — no refresh path).
func (ts *CopilotDBTokenSource) Token() (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.cachedToken != "" {
		return ts.cachedToken, nil
	}
	ctx := store.WithTenantID(context.Background(), ts.tenantID)
	p, err := ts.loadCopilotOAuthProvider(ctx)
	if err != nil {
		return "", fmt.Errorf("load copilot oauth provider %q: %w", ts.providerName, err)
	}
	if p.APIKey == "" {
		return "", fmt.Errorf("copilot provider %q has no token", ts.providerName)
	}
	ts.cachedToken = p.APIKey
	return ts.cachedToken, nil
}

// SaveOAuthResult persists the validated GitHub token + Copilot API base.
func (ts *CopilotDBTokenSource) SaveOAuthResult(ctx context.Context, ghToken string, apiBase, login string) (uuid.UUID, error) {
	ts.mu.Lock()
	ts.cachedToken = ghToken
	ts.mu.Unlock()

	settings := OAuthSettings{AccountID: login}

	existing, err := ts.loadCopilotOAuthProvider(ctx)
	if err == nil {
		updates := map[string]any{
			"api_key":  ghToken,
			"api_base": apiBase,
			"settings": marshalOAuthSettingsInto(existing.Settings, settings),
			"enabled":  true,
		}
		if ts.providerDisplayName != "" {
			updates["display_name"] = ts.providerDisplayName
		}
		if err := ts.providerStore.UpdateProvider(ctx, existing.ID, updates); err != nil {
			return uuid.Nil, fmt.Errorf("update provider: %w", err)
		}
		return existing.ID, nil
	}
	if _, ok := err.(*ProviderTypeConflictError); ok {
		return uuid.Nil, err
	}

	p := &store.LLMProviderData{
		Name:         ts.providerName,
		DisplayName:  ts.providerDisplayName,
		ProviderType: store.ProviderCopilotOAuth,
		APIBase:      apiBase,
		APIKey:       ghToken,
		Enabled:      true,
		Settings:     marshalOAuthSettings(settings),
	}
	if err := ts.providerStore.CreateProvider(ctx, p); err != nil {
		return uuid.Nil, fmt.Errorf("create provider: %w", err)
	}
	return p.ID, nil
}

// Delete removes the provider row.
func (ts *CopilotDBTokenSource) Delete(ctx context.Context) error {
	ts.mu.Lock()
	ts.cachedToken = ""
	ts.mu.Unlock()

	p, err := ts.loadCopilotOAuthProvider(ctx)
	if err != nil {
		if _, ok := err.(*ProviderTypeConflictError); ok {
			return err
		}
		return nil // already gone
	}
	return ts.providerStore.DeleteProvider(ctx, p.ID)
}

// Exists checks if a Copilot OAuth provider exists and has a token.
func (ts *CopilotDBTokenSource) Exists(ctx context.Context) bool {
	p, err := ts.loadCopilotOAuthProvider(ctx)
	return err == nil && p.APIKey != ""
}
