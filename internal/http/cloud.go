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
	enabled      bool   // edition gate AND config kill-switch (cloud.enabled); credentials are dynamic
	redirectBase string // cloud.redirect_base_url (empty = derive from request)
}

// NewCloudHandler creates a CloudHandler.
func NewCloudHandler(manager *cloudmgr.Manager, accounts store.CloudAccountStore, enabled bool, redirectBase string) *CloudHandler {
	return &CloudHandler{
		manager:      manager,
		accounts:     accounts,
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
	mux.HandleFunc("POST /v1/cloud/oauth/{provider}/start", requireAuth("", h.handleStart))
	mux.HandleFunc("GET /v1/cloud/oauth/callback", h.handleCallback)
}

// --- GET /v1/cloud/status ---

func (h *CloudHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	configured := h.manager != nil && h.manager.GoogleConfigured(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": h.enabled && configured,
		"edition": h.editionName(),
		"providers": map[string]any{
			"google": map[string]bool{
				"configured": configured,
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
	clientID, secretSet := h.manager.GoogleCredentialsStatus(r.Context())
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
	if _, alreadySet := h.manager.GoogleCredentialsStatus(r.Context()); !alreadySet && in.ClientSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_secret is required on first save"})
		return
	}
	if err := h.manager.SaveGoogleCredentials(r.Context(), in.ClientID, strings.TrimSpace(in.ClientSecret)); err != nil {
		slog.Error("cloud: save settings failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save cloud settings"})
		return
	}
	slog.Info("cloud: oauth client saved from web UI")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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

// --- POST /v1/cloud/oauth/{provider}/start ---

type cloudStartResponse struct {
	AuthURL    string `json:"auth_url"`
	RedirectURI string `json:"redirect_uri"`
}

func (h *CloudHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	provider := r.PathValue("provider")
	if provider != cloudmgr.GoogleProvider {
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
	authURL, redirectURI, err := h.manager.BuildAuthURL(r.Context(), base, tenantID.String(), userID)
	if err != nil {
		slog.Warn("cloud: build auth url failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build authorization URL"})
		return
	}
	writeJSON(w, http.StatusOK, cloudStartResponse{AuthURL: authURL, RedirectURI: redirectURI})
}

// --- GET /v1/cloud/oauth/callback ---

func (h *CloudHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Surface must still be enabled: the callback is unauthenticated so the
	// edition/config gate is checked here, not just at /start.
	if !h.enabled || h.manager == nil || !h.manager.GoogleConfigured(r.Context()) {
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
