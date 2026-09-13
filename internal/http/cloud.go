package http

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	cloudmgr "github.com/nextlevelbuilder/goclaw/internal/cloud"
	"github.com/nextlevelbuilder/goclaw/internal/edition"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// CloudHandler exposes the per-user OAuth cloud-connection surface:
//
//	GET    /v1/cloud/status                 — surface + provider config status
//	GET    /v1/cloud/settings               — admin: OAuth client config view (no secret)
//	PUT    /v1/cloud/settings               — admin: save OAuth client (first-run setup from the UI)
//	GET    /v1/cloud/accounts               — caller's connected accounts (no tokens)
//	DELETE /v1/cloud/accounts/{id}          — disconnect (owner-scoped)
//	POST   /v1/cloud/oauth/{provider}/start — returns {auth_url, redirect_uri}
//	GET    /v1/cloud/oauth/callback         — provider redirect (unauthenticated; signed state)
type CloudHandler struct {
	manager      *cloudmgr.Manager
	accounts     store.CloudAccountStore
	bindings     store.CloudBindingStore // optional (same DB handle as accounts)
	tenants      store.TenantStore       // for requireTenantAdmin on shared/bindings writes
	enabled      bool                    // edition gate AND config kill-switch (cloud.enabled); credentials are dynamic
	redirectBase string                  // cloud.redirect_base_url (empty = derive from request)
}

// NewCloudHandler creates a CloudHandler.
func NewCloudHandler(manager *cloudmgr.Manager, accounts store.CloudAccountStore, tenants store.TenantStore, enabled bool, redirectBase string) *CloudHandler {
	var bindings store.CloudBindingStore
	if bs, ok := any(accounts).(store.CloudBindingStore); ok {
		bindings = bs
	}
	return &CloudHandler{
		manager:      manager,
		accounts:     accounts,
		bindings:     bindings,
		tenants:      tenants,
		enabled:      enabled,
		redirectBase: redirectBase,
	}
}

// RegisterRoutes registers all cloud routes on the given mux.
func (h *CloudHandler) RegisterRoutes(mux *http.ServeMux) {
	// The callback is an unauthenticated browser redirect: the signed,
	// expiring state param is the CSRF/auth primitive (mcp_oauth pattern).
	// Everything else requires an authenticated session.
	mux.HandleFunc("GET /v1/cloud/status", requireAuth("", h.handleStatus))
	mux.HandleFunc("GET /v1/cloud/settings", requireAuth(permissions.RoleAdmin, h.handleGetSettings))
	mux.HandleFunc("PUT /v1/cloud/settings", requireAuth(permissions.RoleAdmin, h.handlePutSettings))
	mux.HandleFunc("GET /v1/cloud/accounts", requireAuth("", h.handleList))
	mux.HandleFunc("DELETE /v1/cloud/accounts/{id}", requireAuth("", h.handleDelete))
	mux.HandleFunc("PUT /v1/cloud/accounts/{id}/shared", requireAuth("", h.handleSetShared))
	mux.HandleFunc("GET /v1/cloud/bindings", requireAuth("", h.handleListBindings))
	mux.HandleFunc("PUT /v1/cloud/bindings", requireAuth("", h.handleUpsertBinding))
	mux.HandleFunc("DELETE /v1/cloud/bindings/{id}", requireAuth("", h.handleDeleteBinding))
	mux.HandleFunc("POST /v1/cloud/oauth/{provider}/start", requireAuth("", h.handleStart))
	mux.HandleFunc("POST /v1/cloud/oauth/{provider}/complete", requireAuth("", h.handleComplete))
	mux.HandleFunc("GET /v1/cloud/oauth/callback", h.handleCallback)
}

// --- GET /v1/cloud/status ---

func (h *CloudHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	googleConfigured := h.manager != nil && h.manager.GoogleConfigured(r.Context())
	microsoftConfigured := h.manager != nil && h.manager.MicrosoftConfigured(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": h.enabled && (googleConfigured || microsoftConfigured),
		"edition": h.editionName(),
		"providers": map[string]any{
			"google": map[string]bool{
				"configured": googleConfigured,
			},
			"onedrive": map[string]bool{
				"configured": microsoftConfigured,
			},
		},
	})
}

// --- GET/PUT /v1/cloud/settings (admin, master scope — instance-wide config) ---

type cloudSettingsView struct {
	ClientID    string `json:"client_id"`
	SecretSet   bool   `json:"secret_set"`
	RedirectURI string `json:"redirect_uri"`
}

func (h *CloudHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !requireMasterScope(w, r) {
		return
	}
	if h.manager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cloud manager unavailable"})
		return
	}
	if !h.validProvider(w, r) {
		return
	}
	provider := h.requestProvider(r)
	var clientID string
	var secretSet bool
	if provider == cloudmgr.MicrosoftProvider {
		clientID, secretSet = h.manager.MicrosoftCredentialsStatus(r.Context())
	} else {
		clientID, secretSet = h.manager.GoogleCredentialsStatus(r.Context())
	}
	writeJSON(w, http.StatusOK, cloudSettingsView{
		ClientID:    clientID,
		SecretSet:   secretSet,
		RedirectURI: h.redirectURI(r),
	})
}

type cloudSettingsInput struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"` // write-only; empty = keep saved secret
}

func (h *CloudHandler) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !requireMasterScope(w, r) {
		return
	}
	if h.manager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cloud manager unavailable"})
		return
	}
	if !h.validProvider(w, r) {
		return
	}
	provider := h.requestProvider(r)
	var in cloudSettingsInput
	locale := store.LocaleFromContext(r.Context())
	if !bindJSON(w, r, locale, &in) {
		return
	}
	in.ClientID = strings.TrimSpace(in.ClientID)
	if in.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id is required"})
		return
	}
	// First-time save requires a secret; updates may omit it (keep existing).
	alreadySet := false
	if provider == cloudmgr.MicrosoftProvider {
		_, alreadySet = h.manager.MicrosoftCredentialsStatus(r.Context())
	} else {
		_, alreadySet = h.manager.GoogleCredentialsStatus(r.Context())
	}
	if !alreadySet && in.ClientSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_secret is required on first save"})
		return
	}
	var err error
	if provider == cloudmgr.MicrosoftProvider {
		err = h.manager.SaveMicrosoftCredentials(r.Context(), in.ClientID, strings.TrimSpace(in.ClientSecret))
	} else {
		err = h.manager.SaveGoogleCredentials(r.Context(), in.ClientID, strings.TrimSpace(in.ClientSecret))
	}
	if err != nil {
		slog.Error("cloud: save settings failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save cloud settings"})
		return
	}
	slog.Info("cloud: oauth client saved from web UI", "provider", provider)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// requestProvider resolves the per-provider settings/connect target from the
// query string ("google" default for backward compatibility).
func (h *CloudHandler) requestProvider(r *http.Request) string {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		return cloudmgr.GoogleProvider
	}
	return provider
}

// validProvider writes a 400 unless the ?provider= value is connectable.
func (h *CloudHandler) validProvider(w http.ResponseWriter, r *http.Request) bool {
	if cloudmgr.IsSupportedProvider(h.requestProvider(r)) {
		return true
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported provider"})
	return false
}

// redirectURI computes the exact redirect URI admins must register in GCP.
func (h *CloudHandler) redirectURI(r *http.Request) string {
	base := h.redirectBase
	if base == "" {
		base = requestBaseURL(r)
	}
	return cloudmgr.RedirectURI(base)
}

// --- GET /v1/cloud/accounts ---

func (h *CloudHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	accounts, err := h.accounts.List(r.Context())
	if err != nil {
		slog.Error("cloud: list accounts failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list accounts"})
		return
	}
	if accounts == nil {
		accounts = []store.CloudAccount{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

// --- DELETE /v1/cloud/accounts/{id} ---

func (h *CloudHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid account id"})
		return
	}
	if err := h.accounts.Delete(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrCloudAccountNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
			return
		}
		slog.Error("cloud: delete account failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete account"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- PUT /v1/cloud/accounts/{id}/shared ---

type cloudSharedInput struct {
	Shared *bool `json:"shared"`
}

// handleSetShared toggles the tenant-wide shared flag on an account.
// Tenant-admin gated: sharing exposes the account to every agent in the
// tenant, so a regular member cannot opt their own account in (an admin
// consents on their behalf).
func (h *CloudHandler) handleSetShared(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !h.tenantAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid account id"})
		return
	}
	var in cloudSharedInput
	locale := store.LocaleFromContext(r.Context())
	if !bindJSON(w, r, locale, &in) || in.Shared == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "shared (bool) is required"})
		return
	}
	if err := h.accounts.SetShared(r.Context(), id, *in.Shared); err != nil {
		if errors.Is(err, store.ErrCloudAccountNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
			return
		}
		slog.Error("cloud: set shared failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update account"})
		return
	}
	slog.Info("cloud: account shared flag updated", "account_id", id, "shared", *in.Shared)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- /v1/cloud/bindings ---

func validBindingScope(scopeType, scopeKey string) bool {
	switch scopeType {
	case store.CloudBindingScopeTenant:
		return scopeKey == ""
	case store.CloudBindingScopeUser, store.CloudBindingScopeGroup:
		return scopeKey != ""
	default:
		return false
	}
}

// handleListBindings returns the tenant's provider-account bindings. Members
// see the list read-only (needed for the UI picker); writes are admin-gated.
func (h *CloudHandler) handleListBindings(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !requireTenantAdmin(w, r, h.tenants) {
		return
	}
	if h.bindings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "bindings store unavailable"})
		return
	}
	bindings, err := h.bindings.ListBindings(r.Context())
	if err != nil {
		slog.Error("cloud: list bindings failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list bindings"})
		return
	}
	if bindings == nil {
		bindings = []store.CloudBinding{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"bindings": bindings})
}

type cloudBindingInput struct {
	ScopeType string `json:"scope_type"`
	ScopeKey  string `json:"scope_key"`
	Provider  string `json:"provider"`
	AccountID string `json:"account_id"`
}

// handleUpsertBinding assigns a provider account to a scope (tenant default,
// one user, or one group chat). Tenant-admin gated.
func (h *CloudHandler) handleUpsertBinding(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !requireTenantAdmin(w, r, h.tenants) {
		return
	}
	if h.bindings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "bindings store unavailable"})
		return
	}
	var in cloudBindingInput
	locale := store.LocaleFromContext(r.Context())
	if !bindJSON(w, r, locale, &in) {
		return
	}
	in.ScopeType = strings.TrimSpace(in.ScopeType)
	in.ScopeKey = strings.TrimSpace(in.ScopeKey)
	in.Provider = strings.TrimSpace(in.Provider)
	in.AccountID = strings.TrimSpace(in.AccountID)
	if !validBindingScope(in.ScopeType, in.ScopeKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid scope_type/scope_key"})
		return
	}
	if !cloudmgr.IsSupportedProvider(in.Provider) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported provider"})
		return
	}
	if _, err := uuid.Parse(in.AccountID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid account_id"})
		return
	}
	// The bound account must be visible to the tenant (own or shared).
	acct, err := h.manager.AccountByID(r.Context(), in.AccountID)
	if err != nil || acct.Provider != in.Provider {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found for this provider"})
		return
	}
	b := &store.CloudBinding{
		ScopeType: in.ScopeType,
		ScopeKey:  in.ScopeKey,
		Provider:  in.Provider,
		AccountID: in.AccountID,
		CreatedBy: store.UserIDFromContext(r.Context()),
	}
	if err := h.bindings.UpsertBinding(r.Context(), b); err != nil {
		slog.Error("cloud: upsert binding failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save binding"})
		return
	}
	slog.Info("cloud: binding saved", "scope_type", b.ScopeType, "scope_key", b.ScopeKey, "provider", b.Provider, "account_id", b.AccountID)
	writeJSON(w, http.StatusOK, map[string]any{"binding": b})
}

// handleDeleteBinding removes one binding (tenant-admin gated).
func (h *CloudHandler) handleDeleteBinding(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !requireTenantAdmin(w, r, h.tenants) {
		return
	}
	if h.bindings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "bindings store unavailable"})
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid binding id"})
		return
	}
	if err := h.bindings.DeleteBinding(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrCloudAccountNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "binding not found"})
			return
		}
		slog.Error("cloud: delete binding failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete binding"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// tenantAdmin is the cloud-surface alias of requireTenantAdmin.
func (h *CloudHandler) tenantAdmin(w http.ResponseWriter, r *http.Request) bool {
	return requireTenantAdmin(w, r, h.tenants)
}

// --- POST /v1/cloud/oauth/{provider}/start ---

type cloudStartResponse struct {
	AuthURL     string `json:"auth_url"`
	RedirectURI string `json:"redirect_uri"`
	// Mode tells the UI how the flow finishes: "callback" (BYO client — the
	// browser lands back on the server callback) or "paste" (embedded shared
	// client — the browser lands on a loopback URL the user pastes back).
	Mode string `json:"mode"`
}

func (h *CloudHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	provider := r.PathValue("provider")
	if !cloudmgr.IsSupportedProvider(provider) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported provider"})
		return
	}

	// Tenant + user were injected by requireAuth → enrichContext.
	tenantID := store.TenantIDFromContext(r.Context())
	userID := store.UserIDFromContext(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user identity"})
		return
	}

	base := h.redirectBase
	if base == "" {
		base = requestBaseURL(r)
	}
	authURL, redirectURI, mode, err := h.manager.BuildAuthURL(r.Context(), provider, base, tenantID.String(), userID)
	if err != nil {
		slog.Warn("cloud: build auth url failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build authorization URL"})
		return
	}
	writeJSON(w, http.StatusOK, cloudStartResponse{AuthURL: authURL, RedirectURI: redirectURI, Mode: mode})
}

// --- POST /v1/cloud/oauth/{provider}/complete ---

type cloudCompleteInput struct {
	URL string `json:"url"`
}

// handleComplete finishes the paste-back flow: the user's browser landed on
// a loopback redirect target (nothing listening), they copied the URL from
// the address bar and the UI posts it here. The embedded state param is
// HMAC-verified exactly like the server callback.
func (h *CloudHandler) handleComplete(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	provider := r.PathValue("provider")
	if !cloudmgr.IsSupportedProvider(provider) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported provider"})
		return
	}
	userID := store.UserIDFromContext(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user identity"})
		return
	}
	var in cloudCompleteInput
	locale := store.LocaleFromContext(r.Context())
	if !bindJSON(w, r, locale, &in) {
		return
	}
	code, state, err := cloudmgr.ParseRedirectedURL(in.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	acct, err := h.manager.HandleCallback(r.Context(), code, state)
	if err != nil {
		slog.Warn("cloud: paste-back complete failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection failed — the link may have expired, try again"})
		return
	}
	slog.Info("cloud: account connected (paste-back)", "provider", acct.Provider, "email", acct.Email)
	writeJSON(w, http.StatusOK, map[string]string{"email": acct.Email})
}

// --- GET /v1/cloud/oauth/callback ---

func (h *CloudHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Surface must still be enabled: the callback is unauthenticated so the
	// edition/config gate is checked here, not just at /start. At least one
	// provider must have an OAuth client; the state names the provider and
	// HandleCallback dispatches (rejecting unconfigured ones).
	if !h.enabled || h.manager == nil || !h.manager.AnyProviderConfigured(r.Context()) {
		h.redirectCloud(w, r, "error=disabled")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing state"})
		return
	}
	if code == "" {
		h.redirectCloud(w, r, "error=missing_code")
		return
	}

	acct, err := h.manager.HandleCallback(r.Context(), code, state)
	if err != nil {
		slog.Warn("cloud: callback failed", "error", err)
		h.redirectCloud(w, r, "error=exchange_failed")
		return
	}
	slog.Info("cloud: account connected", "provider", acct.Provider, "email", acct.Email)
	h.redirectCloud(w, r, "connected="+url.QueryEscape(acct.Email))
}

// redirectCloud sends the browser back to the Cloud page with a short query
// result. Only fixed key=value fragments are emitted — never a user-supplied
// redirect target (no open redirect).
func (h *CloudHandler) redirectCloud(w http.ResponseWriter, r *http.Request, result string) {
	base := h.redirectBase
	if base == "" {
		base = requestBaseURL(r)
	}
	http.Redirect(w, r, trimRightS(base, "/")+"/cloud?"+result, http.StatusFound)
}

// available writes the gate response (403) when the Cloud surface is off,
// returning true when it is on.
func (h *CloudHandler) available(w http.ResponseWriter, r *http.Request) bool {
	if !h.enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cloud surface is not enabled"})
		return false
	}
	return true
}

func (h *CloudHandler) editionName() string { return edition.Current().Name }

// requestBaseURL derives the public origin for redirects: explicit proxy
// headers first, then r.Host (mcp_oauth callbackURL precedence).
func requestBaseURL(r *http.Request) string {
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		proto := r.Header.Get("X-Forwarded-Proto")
		if proto == "" {
			proto = "https"
		}
		return proto + "://" + strings.TrimRight(fwdHost, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func trimRightS(s, cut string) string {
	for len(s) >= len(cut) && s[len(s)-len(cut):] == cut {
		s = s[:len(s)-len(cut)]
	}
	return s
}
