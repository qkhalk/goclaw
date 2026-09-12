package http

// ClaudeOAuthHandler serves /v1/auth/claude/{provider}/... for Claude Pro/Max
// subscription OAuth. Mirrors OAuthHandler (oauth.go) but paste-only: the
// console.anthropic.com redirect page displays the code, so there is no
// loopback server and therefore no single-flow 409 limitation.

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/oauth"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const claudePendingFlowTTL = 10 * time.Minute

type ClaudeOAuthHandler struct {
	provStore   store.ProviderStore
	secretStore store.ConfigSecretsStore
	providerReg *providers.Registry
	msgBus      *bus.MessageBus

	mu      sync.Mutex
	pending map[string]*pendingClaudeFlow
}

type pendingClaudeFlow struct {
	login        *oauth.PendingClaudeLogin
	flowKey      string
	tenantID     uuid.UUID
	userID       string
	providerName string
	displayName  string
	createdAt    time.Time
}

func NewClaudeOAuthHandler(provStore store.ProviderStore, secretStore store.ConfigSecretsStore, providerReg *providers.Registry, msgBus *bus.MessageBus) *ClaudeOAuthHandler {
	return &ClaudeOAuthHandler{
		provStore:   provStore,
		secretStore: secretStore,
		providerReg: providerReg,
		msgBus:      msgBus,
		pending:     make(map[string]*pendingClaudeFlow),
	}
}

func (h *ClaudeOAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/auth/claude/{provider}/status", h.readAuth(h.handleClaudeStatus))
	mux.HandleFunc("POST /v1/auth/claude/{provider}/start", h.auth(h.handleClaudeStart))
	mux.HandleFunc("POST /v1/auth/claude/{provider}/callback", h.auth(h.handleClaudeCallback))
	mux.HandleFunc("POST /v1/auth/claude/{provider}/logout", h.auth(h.handleClaudeLogout))
}

func (h *ClaudeOAuthHandler) auth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(permissions.RoleAdmin, next)
}

func (h *ClaudeOAuthHandler) readAuth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth("", next)
}

func (h *ClaudeOAuthHandler) newTokenSource(ctx context.Context, providerName, displayName string) *oauth.ClaudeDBTokenSource {
	return oauth.NewClaudeDBTokenSource(h.provStore, h.secretStore, providerName).
		WithTenantID(oauthTenantID(ctx)).
		WithProviderMeta(displayName, "")
}

func (h *ClaudeOAuthHandler) ensureProviderName(ctx context.Context, providerName string) error {
	if h.provStore == nil {
		return nil
	}
	p, err := h.provStore.GetProviderByName(ctx, providerName)
	if err != nil {
		return nil
	}
	if p.ProviderType != store.ProviderClaudeOAuth {
		return &oauth.ProviderTypeConflictError{
			ProviderName: providerName,
			ProviderType: p.ProviderType,
		}
	}
	return nil
}

// takePendingFlow returns the live pending flow for the key, lazily evicting
// expired entries (paste-only flows have no background waiter to clean up).
func (h *ClaudeOAuthHandler) takePendingFlow(flowKey string) *pendingClaudeFlow {
	h.mu.Lock()
	defer h.mu.Unlock()
	flow := h.pending[flowKey]
	if flow == nil {
		return nil
	}
	if time.Since(flow.createdAt) > claudePendingFlowTTL {
		delete(h.pending, flowKey)
		return nil
	}
	return flow
}

func (h *ClaudeOAuthHandler) dropPendingFlow(flowKey string) {
	h.mu.Lock()
	delete(h.pending, flowKey)
	h.mu.Unlock()
}

func (h *ClaudeOAuthHandler) handleClaudeStatus(w http.ResponseWriter, r *http.Request) {
	providerName := oauthProviderName(r)
	if !isValidSlug(providerName) {
		locale := extractLocale(r)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidSlug, "provider")})
		return
	}
	if err := h.ensureProviderName(r.Context(), providerName); err != nil {
		writeOAuthProviderConflict(w, err)
		return
	}

	ts := h.newTokenSource(r.Context(), providerName, "")
	if !ts.Exists(r.Context()) {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	if _, err := ts.Token(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
			"error":         "token invalid or expired",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"provider_name": providerName,
	})
}

func (h *ClaudeOAuthHandler) handleClaudeStart(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	providerName := oauthProviderName(r)
	if !isValidSlug(providerName) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidSlug, "provider")})
		return
	}

	var body struct {
		DisplayName string `json:"display_name"`
	}
	if r.ContentLength > 0 {
		if !bindJSON(w, r, locale, &body) {
			return
		}
	}
	if err := h.ensureProviderName(r.Context(), providerName); err != nil {
		writeOAuthProviderConflict(w, err)
		return
	}

	// Already authenticated?
	ts := h.newTokenSource(r.Context(), providerName, body.DisplayName)
	if ts.Exists(r.Context()) {
		if _, err := ts.Token(); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":        "already_authenticated",
				"provider_name": providerName,
			})
			return
		}
	}

	flowKey := oauthFlowKey(r.Context(), providerName)
	if pending := h.takePendingFlow(flowKey); pending != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"auth_url":      pending.login.AuthURL,
			"provider_name": providerName,
		})
		return
	}

	pending, err := oauth.StartLoginClaude()
	if err != nil {
		slog.Error("claude_oauth.start", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": i18n.T(locale, i18n.MsgInternalError, "failed to start Claude OAuth flow"),
		})
		return
	}

	h.mu.Lock()
	h.pending[flowKey] = &pendingClaudeFlow{
		login:        pending,
		flowKey:      flowKey,
		tenantID:     oauthTenantID(r.Context()),
		userID:       store.UserIDFromContext(r.Context()),
		providerName: providerName,
		displayName:  body.DisplayName,
		createdAt:    time.Now(),
	}
	h.mu.Unlock()

	emitAudit(h.msgBus, r, "oauth.login_started", "oauth", "claude")
	writeJSON(w, http.StatusOK, map[string]any{
		"auth_url":      pending.AuthURL,
		"provider_name": providerName,
	})
}

func (h *ClaudeOAuthHandler) handleClaudeCallback(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	providerName := oauthProviderName(r)
	if !isValidSlug(providerName) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidSlug, "provider")})
		return
	}

	var body struct {
		RedirectURL string `json:"redirect_url"`
	}
	if !bindJSON(w, r, locale, &body) {
		return
	}
	if body.RedirectURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgRequired, "redirect_url")})
		return
	}
	if err := h.ensureProviderName(r.Context(), providerName); err != nil {
		writeOAuthProviderConflict(w, err)
		return
	}

	flowKey := oauthFlowKey(r.Context(), providerName)
	pending := h.takePendingFlow(flowKey)
	if pending == nil || pending.providerName != providerName || pending.tenantID != oauthTenantID(r.Context()) || pending.userID != store.UserIDFromContext(r.Context()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgNoPendingOAuth)})
		return
	}

	tokenResp, err := pending.login.ExchangeRedirectURL(body.RedirectURL)
	if err != nil {
		slog.Warn("claude_oauth.manual_callback", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.dropPendingFlow(flowKey)

	providerID, err := h.saveAndRegister(r.Context(), providerName, pending.displayName, tokenResp)
	if err != nil {
		if writeOAuthProviderConflict(w, err) {
			return
		}
		slog.Error("claude_oauth.save_token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": i18n.T(locale, i18n.MsgFailedToSaveToken)})
		return
	}

	slog.Info("claude_oauth: token saved via manual callback", "provider", providerName)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"provider_name": providerName,
		"provider_id":   providerID.String(),
	})
}

func (h *ClaudeOAuthHandler) handleClaudeLogout(w http.ResponseWriter, r *http.Request) {
	providerName := oauthProviderName(r)
	if !isValidSlug(providerName) {
		locale := extractLocale(r)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidSlug, "provider")})
		return
	}
	if err := h.ensureProviderName(r.Context(), providerName); err != nil {
		writeOAuthProviderConflict(w, err)
		return
	}

	ts := h.newTokenSource(r.Context(), providerName, "")
	if err := ts.Delete(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if h.providerReg != nil {
		tid := store.TenantIDFromContext(r.Context())
		if tid == uuid.Nil {
			tid = store.MasterTenantID
		}
		h.providerReg.UnregisterForTenant(tid, providerName)
	}

	emitAudit(h.msgBus, r, "oauth.logout", "oauth", "claude")
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (h *ClaudeOAuthHandler) saveAndRegister(ctx context.Context, providerName, displayName string, tokenResp *oauth.ClaudeTokenResponse) (uuid.UUID, error) {
	saveCtx := store.WithTenantID(context.Background(), oauthTenantID(ctx))
	accountEmail := oauth.FetchClaudeAccountEmail(saveCtx, tokenResp.AccessToken)

	ts := h.newTokenSource(saveCtx, providerName, displayName)
	providerID, err := ts.SaveOAuthResult(saveCtx, tokenResp, accountEmail)
	if err != nil {
		return uuid.Nil, err
	}

	if h.providerReg != nil {
		h.providerReg.RegisterForTenant(oauthTenantID(saveCtx), providers.NewClaudeOAuthProvider(providerName, ts, "", "", nil))
	}
	return providerID, nil
}
