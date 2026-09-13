package http

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	cloudmgr "github.com/nextlevelbuilder/goclaw/internal/cloud"
	"github.com/nextlevelbuilder/goclaw/internal/cloud/mail"
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
//
// Plus the file-operation surface (Phase 4) under /v1/cloud/accounts/{id}/files,
// the cross-account transfer endpoints and the tenant-admin sync-pair config
// (Phase 6, see RegisterRoutes) — writes are gated on owner-or-tenant-admin
// AND the account's write OAuth scopes.
type CloudHandler struct {
	manager      *cloudmgr.Manager
	accounts     store.CloudAccountStore
	bindings     store.CloudBindingStore // optional (same DB handle as accounts)
	syncPairs    store.CloudSyncPairStore // optional (same DB handle as accounts)
	sync         *cloudmgr.SyncService    // optional; backs POST .../sync-pairs/{id}/run
	tenants      store.TenantStore        // for requireTenantAdmin on shared/bindings writes
	mail         *cloudmgr.MailService    // optional; backs the per-account mailbox view
	enabled      bool                     // edition gate AND config kill-switch (cloud.enabled); credentials are dynamic
	redirectBase string                   // cloud.redirect_base_url (empty = derive from request)
	fetchCapMB   int64                    // cloud.fetch_size_cap_mb — upload/download size cap
}

// NewCloudHandler creates a CloudHandler.
func NewCloudHandler(manager *cloudmgr.Manager, accounts store.CloudAccountStore, tenants store.TenantStore, mail *cloudmgr.MailService, enabled bool, redirectBase string, fetchCapMB int64) *CloudHandler {
	var bindings store.CloudBindingStore
	if bs, ok := any(accounts).(store.CloudBindingStore); ok {
		bindings = bs
	}
	if fetchCapMB <= 0 {
		fetchCapMB = 100
	}
	return &CloudHandler{
		manager:      manager,
		accounts:     accounts,
		bindings:     bindings,
		tenants:      tenants,
		mail:         mail,
		enabled:      enabled,
		redirectBase: redirectBase,
		fetchCapMB:   fetchCapMB,
	}
}

// SetSync wires the sync-pair surface: the tenant-scoped pair store (same DB
// handle as the account store) and the worker backing "run now". Optional —
// without it the sync-pair endpoints answer 503.
func (h *CloudHandler) SetSync(pairs store.CloudSyncPairStore, sync *cloudmgr.SyncService) {
	h.syncPairs = pairs
	h.sync = sync
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
	mux.HandleFunc("GET /v1/cloud/accounts/{id}/about", requireAuth("", h.handleAccountAbout))
	mux.HandleFunc("GET /v1/cloud/accounts/{id}/files", requireAuth("", h.handleAccountFiles))
	mux.HandleFunc("GET /v1/cloud/accounts/{id}/mail", requireAuth("", h.handleAccountMail))
	mux.HandleFunc("GET /v1/cloud/bindings", requireAuth("", h.handleListBindings))
	mux.HandleFunc("PUT /v1/cloud/bindings", requireAuth("", h.handleUpsertBinding))
	mux.HandleFunc("DELETE /v1/cloud/bindings/{id}", requireAuth("", h.handleDeleteBinding))
	mux.HandleFunc("POST /v1/cloud/oauth/{provider}/start", requireAuth("", h.handleStart))
	mux.HandleFunc("POST /v1/cloud/oauth/{provider}/complete", requireAuth("", h.handleComplete))
	mux.HandleFunc("GET /v1/cloud/oauth/callback", h.handleCallback)

	// File operations (Phase 4). All writes: owner-or-tenant-admin + the
	// account's write OAuth scopes (403 code=cloud_write_scope_required);
	// download is read-only (any caller who can see the account).
	mux.HandleFunc("POST /v1/cloud/accounts/{id}/files", requireAuth("", h.handleAccountUpload))
	mux.HandleFunc("POST /v1/cloud/accounts/{id}/folders", requireAuth("", h.handleAccountMkdir))
	mux.HandleFunc("PATCH /v1/cloud/accounts/{id}/files", requireAuth("", h.handleAccountMove))
	mux.HandleFunc("POST /v1/cloud/accounts/{id}/files/copy", requireAuth("", h.handleAccountCopy))
	mux.HandleFunc("POST /v1/cloud/accounts/{id}/files/copyurl", requireAuth("", h.handleAccountCopyURL))
	mux.HandleFunc("DELETE /v1/cloud/accounts/{id}/files", requireAuth("", h.handleAccountDelete))
	mux.HandleFunc("GET /v1/cloud/accounts/{id}/files/download", requireAuth("", h.handleAccountDownload))
	mux.HandleFunc("POST /v1/cloud/accounts/{id}/files/publiclink", requireAuth("", h.handleAccountPublicLink))
	mux.HandleFunc("POST /v1/cloud/transfer", requireAuth("", h.handleTransfer))
	mux.HandleFunc("GET /v1/cloud/transfers/{id}", requireAuth("", h.handleTransferStatus))

	// Sync pairs (Phase 6): tenant-level config — every endpoint is
	// tenant-admin gated (members get 403), like the binding rules above.
	mux.HandleFunc("GET /v1/cloud/sync-pairs", requireAuth("", h.handleListSyncPairs))
	mux.HandleFunc("POST /v1/cloud/sync-pairs", requireAuth("", h.handleCreateSyncPair))
	mux.HandleFunc("PUT /v1/cloud/sync-pairs/{id}", requireAuth("", h.handleUpdateSyncPair))
	mux.HandleFunc("DELETE /v1/cloud/sync-pairs/{id}", requireAuth("", h.handleDeleteSyncPair))
	mux.HandleFunc("POST /v1/cloud/sync-pairs/{id}/run", requireAuth("", h.handleRunSyncPair))
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

// cloudAccountView is one account in the list response: the store row plus
// the derived can_write flag (true when the stored OAuth grant includes the
// provider's write scope — false for accounts connected before the write
// upgrade, which stay read-only until re-granted).
type cloudAccountView struct {
	store.CloudAccount
	CanWrite bool `json:"can_write"`
}

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
	out := make([]cloudAccountView, 0, len(accounts))
	for i := range accounts {
		out = append(out, cloudAccountView{
			CloudAccount: accounts[i],
			CanWrite:     cloudmgr.AccountCanWrite(&accounts[i]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out})
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
	// Optional rule controls: enabled defaults to true, priority to 100
	// (valid range 0–1000, lower wins on ties within a scope tier).
	Enabled  *bool `json:"enabled,omitempty"`
	Priority *int  `json:"priority,omitempty"`
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
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	priority := 100
	if in.Priority != nil {
		if *in.Priority < 0 || *in.Priority > 1000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "priority must be between 0 and 1000"})
			return
		}
		priority = *in.Priority
	}
	b := &store.CloudBinding{
		ScopeType: in.ScopeType,
		ScopeKey:  in.ScopeKey,
		Provider:  in.Provider,
		AccountID: in.AccountID,
		CreatedBy: store.UserIDFromContext(r.Context()),
		Enabled:   enabled,
		Priority:  priority,
	}
	if err := h.bindings.UpsertBinding(r.Context(), b); err != nil {
		slog.Error("cloud: upsert binding failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save binding"})
		return
	}
	slog.Info("cloud: binding saved", "scope_type", b.ScopeType, "scope_key", b.ScopeKey, "provider", b.Provider, "account_id", b.AccountID, "enabled", b.Enabled, "priority", b.Priority)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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

// --- GET /v1/cloud/accounts/{id}/about | /files | /mail (account detail) ---

// accessibleAccount resolves the account for the detail views, verifying the
// caller may see it (own or tenant-shared).
func (h *CloudHandler) accessibleAccount(w http.ResponseWriter, r *http.Request) *store.CloudAccount {
	return h.accountByID(w, r, r.PathValue("id"))
}

// accountByID resolves one accessible account (own or tenant-shared) by ID,
// writing the 400/404 response and returning nil when unresolvable.
func (h *CloudHandler) accountByID(w http.ResponseWriter, r *http.Request, id string) *store.CloudAccount {
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid account id"})
		return nil
	}
	acct, err := h.manager.AccountByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
		return nil
	}
	return acct
}

// handleAccountAbout returns the rclone quota (total/used/free bytes) for a
// storage-capable account.
func (h *CloudHandler) handleAccountAbout(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	storage := h.manager.StorageService()
	if storage == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage layer unavailable"})
		return
	}
	acct := h.accessibleAccount(w, r)
	if acct == nil {
		return
	}
	about, err := storage.AboutAccount(r.Context(), acct)
	if err != nil {
		h.storageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": about.Total, "used": about.Used, "free": about.Free,
	})
}

// handleAccountFiles lists one remote path of the account (read-only browse).
func (h *CloudHandler) handleAccountFiles(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	storage := h.manager.StorageService()
	if storage == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage layer unavailable"})
		return
	}
	acct := h.accessibleAccount(w, r)
	if acct == nil {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	entries, err := storage.ListAccount(r.Context(), acct, path, limit)
	if err != nil {
		h.storageError(w, err)
		return
	}
	type fileEntry struct {
		Name    string `json:"name"`
		IsDir   bool   `json:"is_dir"`
		Size    int64  `json:"size"`
		ModTime string `json:"mod_time"`
	}
	out := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, fileEntry{Name: e.Name, IsDir: e.IsDir, Size: e.Size, ModTime: e.ModTime})
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "entries": out})
}

// handleAccountMail returns the recent inbox for a Gmail-capable account —
// the web "hộp thư" preview (read-only; full reads stay with the agent).
func (h *CloudHandler) handleAccountMail(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	if h.mail == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "mail layer unavailable"})
		return
	}
	acct := h.accessibleAccount(w, r)
	if acct == nil {
		return
	}
	client, err := h.mail.MailClient(r.Context(), acct.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "this account has no mailbox (Gmail tools need a BYO OAuth client)"})
		return
	}
	email, _ := client.GetProfile(r.Context())
	max := 10
	if v := r.URL.Query().Get("max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			max = n
		}
	}
	messages, _, err := client.Search(r.Context(), "in:inbox", max, "")
	if err != nil {
		slog.Warn("cloud: mailbox preview failed", "account", acct.ID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read mailbox"})
		return
	}
	if messages == nil {
		messages = []mail.MessageSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "messages": messages})
}

// --- Cloud file operations (Phase 4) ---

// requireAccountWrite allows only the account owner or a tenant admin to
// mutate an account's files: members with access to a shared account are
// read-only. Writes the 403 response when denied.
func (h *CloudHandler) requireAccountWrite(w http.ResponseWriter, r *http.Request, acct *store.CloudAccount) bool {
	if userID := store.UserIDFromContext(r.Context()); userID != "" && acct.UserID == userID {
		return true
	}
	return h.tenantAdmin(w, r)
}

// requireWriteScope rejects accounts whose stored OAuth grant lacks the
// provider's write scope. The machine-readable code lets the UI offer the
// re-grant flow instead of a dead end.
func (h *CloudHandler) requireWriteScope(w http.ResponseWriter, acct *store.CloudAccount) bool {
	if cloudmgr.AccountCanWrite(acct) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": cloudmgr.ErrCloudWriteScopeRequired.Error(),
		"code":  "cloud_write_scope_required",
	})
	return false
}

// storageLayer resolves the shared rclone storage service (503 when nil).
// The caller must already have passed h.available().
func (h *CloudHandler) storageLayer(w http.ResponseWriter) *cloudmgr.StorageService {
	st := h.manager.StorageService()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage layer unavailable"})
		return nil
	}
	return st
}

// writeAccess bundles the write-endpoint preflight, in order: surface
// enabled → account accessible → owner-or-admin → write OAuth scopes →
// storage layer up. It writes the error response itself and returns nils
// when any check fails.
func (h *CloudHandler) writeAccess(w http.ResponseWriter, r *http.Request) (*cloudmgr.StorageService, *store.CloudAccount) {
	if !h.available(w, r) {
		return nil, nil
	}
	acct := h.accessibleAccount(w, r)
	if acct == nil {
		return nil, nil
	}
	if !h.requireAccountWrite(w, r, acct) || !h.requireWriteScope(w, acct) {
		return nil, nil
	}
	st := h.storageLayer(w)
	if st == nil {
		return nil, nil
	}
	return st, acct
}

// rejectBadPath writes the 400 for a path that failed validation, with the
// same security.* log the local-storage surface emits on traversal attempts.
func rejectBadPath(w http.ResponseWriter, kind, raw string) {
	slog.Warn("security.cloud_path_traversal", "kind", kind, "path", raw)
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
}

type cloudFolderPath struct {
	Path string `json:"path"`
}

// handleAccountMkdir creates a folder in the account's remote.
func (h *CloudHandler) handleAccountMkdir(w http.ResponseWriter, r *http.Request) {
	st, acct := h.writeAccess(w, r)
	if st == nil {
		return
	}
	var in cloudFolderPath
	if !bindJSON(w, r, store.LocaleFromContext(r.Context()), &in) {
		return
	}
	dir, err := cloudmgr.CleanRemotePath(in.Path)
	if err != nil {
		rejectBadPath(w, "mkdir", in.Path)
		return
	}
	if err := st.MkdirAccount(r.Context(), acct, dir); err != nil {
		h.storageError(w, err)
		return
	}
	slog.Info("cloud: folder created", "account", acct.ID, "path", dir)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type cloudFilePaths struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// handleAccountMove renames/moves one file within the account's remote
// (PATCH — the resource is the same file under a new name/location).
func (h *CloudHandler) handleAccountMove(w http.ResponseWriter, r *http.Request) {
	st, acct := h.writeAccess(w, r)
	if st == nil {
		return
	}
	var in cloudFilePaths
	if !bindJSON(w, r, store.LocaleFromContext(r.Context()), &in) {
		return
	}
	from, err := cloudmgr.CleanRemotePath(in.From)
	if err != nil {
		rejectBadPath(w, "move.from", in.From)
		return
	}
	to, err := cloudmgr.CleanRemotePath(in.To)
	if err != nil {
		rejectBadPath(w, "move.to", in.To)
		return
	}
	if err := st.MoveAccount(r.Context(), acct, from, to); err != nil {
		h.storageError(w, err)
		return
	}
	slog.Info("cloud: file moved", "account", acct.ID, "from", from, "to", to)
	writeJSON(w, http.StatusOK, map[string]string{"from": from, "to": to})
}

// handleAccountCopy copies one file to another path within the account.
func (h *CloudHandler) handleAccountCopy(w http.ResponseWriter, r *http.Request) {
	st, acct := h.writeAccess(w, r)
	if st == nil {
		return
	}
	var in cloudFilePaths
	if !bindJSON(w, r, store.LocaleFromContext(r.Context()), &in) {
		return
	}
	from, err := cloudmgr.CleanRemotePath(in.From)
	if err != nil {
		rejectBadPath(w, "copy.from", in.From)
		return
	}
	to, err := cloudmgr.CleanRemotePath(in.To)
	if err != nil {
		rejectBadPath(w, "copy.to", in.To)
		return
	}
	if err := st.CopyAccount(r.Context(), acct, from, to); err != nil {
		h.storageError(w, err)
		return
	}
	slog.Info("cloud: file copied", "account", acct.ID, "from", from, "to", to)
	writeJSON(w, http.StatusOK, map[string]string{"from": from, "to": to})
}

type cloudCopyURL struct {
	URL  string `json:"url"`
	Path string `json:"path"` // full destination file path incl. name
}

// handleAccountCopyURL uploads-by-URL: rclone fetches the URL server side
// into the account. Only absolute http(s) URLs — no file:// or other schemes.
func (h *CloudHandler) handleAccountCopyURL(w http.ResponseWriter, r *http.Request) {
	st, acct := h.writeAccess(w, r)
	if st == nil {
		return
	}
	var in cloudCopyURL
	if !bindJSON(w, r, store.LocaleFromContext(r.Context()), &in) {
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	u, perr := url.Parse(in.URL)
	if perr != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url must be an absolute http(s) URL"})
		return
	}
	path, err := cloudmgr.CleanRemotePath(in.Path)
	if err != nil {
		rejectBadPath(w, "copyurl", in.Path)
		return
	}
	if err := st.CopyURLAccount(r.Context(), acct, in.URL, path); err != nil {
		h.storageError(w, err)
		return
	}
	slog.Info("cloud: url copied into remote", "account", acct.ID, "path", path)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAccountDelete removes one file (isDir=false) or one EMPTY directory
// (isDir=true). Deletion is PERMANENT on Drive — no trash via rc; the UI
// must confirm. Query: ?path=&isDir=.
func (h *CloudHandler) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	st, acct := h.writeAccess(w, r)
	if st == nil {
		return
	}
	raw := r.URL.Query().Get("path")
	path, err := cloudmgr.CleanRemotePath(raw)
	if err != nil {
		rejectBadPath(w, "delete", raw)
		return
	}
	isDir := r.URL.Query().Get("isDir") == "true"
	if err := st.DeleteAccount(r.Context(), acct, path, isDir); err != nil {
		h.storageError(w, err)
		return
	}
	slog.Info("cloud: path deleted", "account", acct.ID, "path", path, "is_dir", isDir)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// sanitizeRemoteName reduces a multipart filename to a bare, safe remote file
// name: no separators, no dot segments.
func sanitizeRemoteName(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	switch name {
	case "", ".", "..":
		return ""
	}
	return name
}

// handleAccountUpload receives one multipart file (fields: `file`, `path` =
// destination folder, default root) into the account's remote. Transport is
// temp file + operations/copyfile — works with every rclone version (the rc
// upload endpoints are multipart and binary-gated). The size cap mirrors the
// download path (cloud.fetch_size_cap_mb); the temp file is removed on every
// path, success or failure.
func (h *CloudHandler) handleAccountUpload(w http.ResponseWriter, r *http.Request) {
	st, acct := h.writeAccess(w, r)
	if st == nil {
		return
	}
	capBytes := h.fetchCapMB << 20
	r.Body = http.MaxBytesReader(w, r.Body, capBytes)
	// maxMemory stays small (32 MB) — file parts beyond it spool to Go's own
	// temp files; the total body size is enforced by MaxBytesReader above.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
				"error": fmt.Sprintf("upload exceeds the %d MB size cap", h.fetchCapMB),
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed multipart body"})
		return
	}
	dir, err := cloudmgr.CleanRemoteDir(r.FormValue("path"))
	if err != nil {
		rejectBadPath(w, "upload", r.FormValue("path"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file field"})
		return
	}
	defer file.Close()
	name := sanitizeRemoteName(header.Filename)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
		return
	}
	tmp, err := os.CreateTemp("", "goclaw-cloud-upload-")
	if err != nil {
		slog.Error("cloud: upload temp file failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to stage upload"})
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	written, err := io.Copy(tmp, file)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		slog.Error("cloud: upload staging failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to stage upload"})
		return
	}
	remotePath := cloudmgr.RemoteJoin(dir, name)
	if err := st.UploadAccount(r.Context(), acct, filepath.Dir(tmpPath), filepath.Base(tmpPath), remotePath); err != nil {
		h.storageError(w, err)
		return
	}
	slog.Info("cloud: file uploaded", "account", acct.ID, "path", remotePath, "size", written)
	writeJSON(w, http.StatusOK, map[string]any{"path": remotePath, "filename": name, "size": written})
}

// handleAccountDownload streams one remote file to the browser. Read-only —
// any caller who can see the account; no write guard. Over-cap requests get
// a clear 413 before any transfer starts.
func (h *CloudHandler) handleAccountDownload(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	acct := h.accessibleAccount(w, r)
	if acct == nil {
		return
	}
	st := h.storageLayer(w)
	if st == nil {
		return
	}
	raw := r.URL.Query().Get("path")
	path, err := cloudmgr.CleanRemotePath(raw)
	if err != nil {
		rejectBadPath(w, "download", raw)
		return
	}
	df, err := st.DownloadAccount(r.Context(), acct, path, h.fetchCapMB)
	if err != nil {
		if errors.Is(err, cloudmgr.ErrFileTooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": err.Error()})
			return
		}
		h.storageError(w, err)
		return
	}
	// The temp copy lives exactly as long as this request.
	defer os.RemoveAll(df.Dir)
	f, err := os.Open(filepath.Join(df.Dir, df.Name))
	if err != nil {
		h.storageError(w, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", df.Name))
	if df.Stat != nil && df.Stat.MimeType != "" {
		w.Header().Set("Content-Type", df.Stat.MimeType)
	}
	http.ServeContent(w, r, df.Name, time.Time{}, f)
}

// handleAccountPublicLink creates/retrieves a public share link. Guarded like
// a write: the link is public and does not expire on its own.
func (h *CloudHandler) handleAccountPublicLink(w http.ResponseWriter, r *http.Request) {
	st, acct := h.writeAccess(w, r)
	if st == nil {
		return
	}
	var in cloudFolderPath
	if !bindJSON(w, r, store.LocaleFromContext(r.Context()), &in) {
		return
	}
	path, err := cloudmgr.CleanRemotePath(in.Path)
	if err != nil {
		rejectBadPath(w, "publiclink", in.Path)
		return
	}
	link, err := st.PublicLinkAccount(r.Context(), acct, path)
	if err != nil {
		h.storageError(w, err)
		return
	}
	slog.Info("cloud: public link created", "account", acct.ID, "path", path)
	writeJSON(w, http.StatusOK, map[string]string{"url": link.URL})
}

type cloudTransferInput struct {
	SourceAccountID string `json:"source_account_id"`
	SourcePath      string `json:"source_path"`
	TargetAccountID string `json:"target_account_id"`
	TargetPath      string `json:"target_path"`
	Mode            string `json:"mode"` // "copy" (default) | "move"
}

// handleTransfer copies/moves between two accessible accounts. The source
// needs read access only; the target requires the write guard + write scopes.
// Files transfer synchronously ({ok:true}); folders queue an async rclone job
// (202 {job_id}) polled at GET /v1/cloud/transfers/{job_id}.
func (h *CloudHandler) handleTransfer(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	var in cloudTransferInput
	if !bindJSON(w, r, store.LocaleFromContext(r.Context()), &in) {
		return
	}
	src := h.accountByID(w, r, strings.TrimSpace(in.SourceAccountID))
	if src == nil {
		return
	}
	dst := h.accountByID(w, r, strings.TrimSpace(in.TargetAccountID))
	if dst == nil {
		return
	}
	if !h.requireAccountWrite(w, r, dst) || !h.requireWriteScope(w, dst) {
		return
	}
	st := h.storageLayer(w)
	if st == nil {
		return
	}
	srcPath, err := cloudmgr.CleanRemotePath(in.SourcePath)
	if err != nil {
		rejectBadPath(w, "transfer.source", in.SourcePath)
		return
	}
	dstPath, err := cloudmgr.CleanRemotePath(in.TargetPath)
	if err != nil {
		rejectBadPath(w, "transfer.target", in.TargetPath)
		return
	}
	if in.Mode != "" && in.Mode != "copy" && in.Mode != "move" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be \"copy\" or \"move\""})
		return
	}
	jobID, err := st.TransferAccount(r.Context(), src, dst, srcPath, dstPath, in.Mode)
	if err != nil {
		h.storageError(w, err)
		return
	}
	if jobID == 0 {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	slog.Info("cloud: transfer queued", "source", src.ID, "target", dst.ID, "job_id", jobID)
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": strconv.FormatInt(jobID, 10)})
}

// handleTransferStatus polls one async transfer job. Ownership (same tenant +
// same user) is enforced inside the registry — cross-tenant polling 404s.
func (h *CloudHandler) handleTransferStatus(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) {
		return
	}
	st := h.storageLayer(w)
	if st == nil {
		return
	}
	jobID, perr := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if perr != nil || jobID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid job id"})
		return
	}
	job, err := st.TransferStatus(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, cloudmgr.ErrTransferNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		h.storageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":   strconv.FormatInt(jobID, 10),
		"finished": job.Finished,
		"success":  job.Success,
		"error":    job.Error,
	})
}

// --- Sync pairs (Phase 6) ---

// syncPairStore resolves the pair store or writes the 503 when the sync
// surface is not wired. Caller must already have passed h.available().
func (h *CloudHandler) syncPairStore(w http.ResponseWriter) store.CloudSyncPairStore {
	if h.syncPairs == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sync pairs store unavailable"})
		return nil
	}
	return h.syncPairs
}

// handleListSyncPairs returns the tenant's sync pairs (tenant-admin gated).
func (h *CloudHandler) handleListSyncPairs(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !h.tenantAdmin(w, r) {
		return
	}
	pairs := h.syncPairStore(w)
	if pairs == nil {
		return
	}
	list, err := pairs.List(r.Context())
	if err != nil {
		slog.Error("cloud: list sync pairs failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sync pairs"})
		return
	}
	if list == nil {
		list = []store.CloudSyncPair{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pairs": list})
}

type cloudSyncPairInput struct {
	SourceAccountID string `json:"source_account_id"`
	SourcePath      string `json:"source_path"`
	TargetAccountID string `json:"target_account_id"`
	TargetPath      string `json:"target_path"`
	// IntervalMinutes: 0 = manual only; nil = keep/create with 0.
	IntervalMinutes *int `json:"interval_minutes,omitempty"`
	Enabled         *bool `json:"enabled,omitempty"`
}

// maxSyncIntervalMinutes caps the custom schedule at 30 days.
const maxSyncIntervalMinutes = 60 * 24 * 30

// validateSyncPairInput normalizes + validates the create/update payload and
// resolves both endpoint accounts (accessible to the tenant, storage-capable).
// Writes the error response and returns ok=false on any problem.
func (h *CloudHandler) validateSyncPairInput(w http.ResponseWriter, r *http.Request, in *cloudSyncPairInput) (srcPath, dstPath string, src, dst *store.CloudAccount, interval int, ok bool) {
	in.SourceAccountID = strings.TrimSpace(in.SourceAccountID)
	in.TargetAccountID = strings.TrimSpace(in.TargetAccountID)
	if _, err := uuid.Parse(in.SourceAccountID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source_account_id"})
		return "", "", nil, nil, 0, false
	}
	if _, err := uuid.Parse(in.TargetAccountID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target_account_id"})
		return "", "", nil, nil, 0, false
	}
	// CleanRemoteDir (not Path): "/" (drive root) is a valid sync endpoint.
	srcPath, err := cloudmgr.CleanRemoteDir(in.SourcePath)
	if err != nil {
		rejectBadPath(w, "sync-pair.source", in.SourcePath)
		return "", "", nil, nil, 0, false
	}
	dstPath, err = cloudmgr.CleanRemoteDir(in.TargetPath)
	if err != nil {
		rejectBadPath(w, "sync-pair.target", in.TargetPath)
		return "", "", nil, nil, 0, false
	}
	if srcPath == "" {
		srcPath = "/"
	}
	if dstPath == "" {
		dstPath = "/"
	}
	interval = 0
	if in.IntervalMinutes != nil {
		interval = *in.IntervalMinutes
		if interval < 0 || interval > maxSyncIntervalMinutes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("interval_minutes must be between 0 and %d", maxSyncIntervalMinutes)})
			return "", "", nil, nil, 0, false
		}
	}
	src = h.accountByID(w, r, in.SourceAccountID)
	if src == nil {
		return "", "", nil, nil, 0, false
	}
	dst = h.accountByID(w, r, in.TargetAccountID)
	if dst == nil {
		return "", "", nil, nil, 0, false
	}
	if !cloudmgr.IsStorageProvider(src.Provider) || !cloudmgr.IsStorageProvider(dst.Provider) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "both endpoints must be storage-capable accounts"})
		return "", "", nil, nil, 0, false
	}
	// A pair syncing a path onto itself would be a no-op at best.
	if src.ID == dst.ID && srcPath == dstPath {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source and target account+path must differ"})
		return "", "", nil, nil, 0, false
	}
	return srcPath, dstPath, src, dst, interval, true
}

// handleCreateSyncPair inserts one sync pair (tenant-admin gated).
func (h *CloudHandler) handleCreateSyncPair(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !h.tenantAdmin(w, r) {
		return
	}
	pairs := h.syncPairStore(w)
	if pairs == nil {
		return
	}
	var in cloudSyncPairInput
	if !bindJSON(w, r, store.LocaleFromContext(r.Context()), &in) {
		return
	}
	srcPath, dstPath, _, _, interval, ok := h.validateSyncPairInput(w, r, &in)
	if !ok {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	p := &store.CloudSyncPair{
		SourceAccountID: strings.TrimSpace(in.SourceAccountID),
		SourcePath:      srcPath,
		TargetAccountID: strings.TrimSpace(in.TargetAccountID),
		TargetPath:      dstPath,
		IntervalMinutes: interval,
		Enabled:         enabled,
		CreatedBy:       store.UserIDFromContext(r.Context()),
	}
	if err := pairs.Create(r.Context(), p); err != nil {
		slog.Error("cloud: create sync pair failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create sync pair"})
		return
	}
	p.TenantID = store.TenantIDFromContext(r.Context()).String()
	slog.Info("cloud: sync pair created", "pair_id", p.ID, "source", p.SourceAccountID, "target", p.TargetAccountID, "interval", p.IntervalMinutes)
	writeJSON(w, http.StatusCreated, p)
}

// handleUpdateSyncPair replaces the mutable fields of one sync pair.
func (h *CloudHandler) handleUpdateSyncPair(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !h.tenantAdmin(w, r) {
		return
	}
	pairs := h.syncPairStore(w)
	if pairs == nil {
		return
	}
	id := r.PathValue("id")
	p, err := pairs.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrCloudSyncPairNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sync pair not found"})
			return
		}
		slog.Error("cloud: get sync pair failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sync pair"})
		return
	}
	var in cloudSyncPairInput
	if !bindJSON(w, r, store.LocaleFromContext(r.Context()), &in) {
		return
	}
	srcPath, dstPath, _, _, interval, ok := h.validateSyncPairInput(w, r, &in)
	if !ok {
		return
	}
	p.SourceAccountID = strings.TrimSpace(in.SourceAccountID)
	p.SourcePath = srcPath
	p.TargetAccountID = strings.TrimSpace(in.TargetAccountID)
	p.TargetPath = dstPath
	p.IntervalMinutes = interval
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	if err := pairs.Update(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrCloudSyncPairNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sync pair not found"})
			return
		}
		slog.Error("cloud: update sync pair failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update sync pair"})
		return
	}
	slog.Info("cloud: sync pair updated", "pair_id", p.ID, "interval", p.IntervalMinutes, "enabled", p.Enabled)
	writeJSON(w, http.StatusOK, p)
}

// handleDeleteSyncPair removes one sync pair.
func (h *CloudHandler) handleDeleteSyncPair(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !h.tenantAdmin(w, r) {
		return
	}
	pairs := h.syncPairStore(w)
	if pairs == nil {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sync pair id"})
		return
	}
	if err := pairs.Delete(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrCloudSyncPairNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sync pair not found"})
			return
		}
		slog.Error("cloud: delete sync pair failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete sync pair"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRunSyncPair queues one manual run on the worker loop (never executed
// inside the request — folder syncs are async and far exceed HTTP timeouts).
func (h *CloudHandler) handleRunSyncPair(w http.ResponseWriter, r *http.Request) {
	if !h.available(w, r) || !h.tenantAdmin(w, r) {
		return
	}
	pairs := h.syncPairStore(w)
	if pairs == nil {
		return
	}
	if h.sync == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sync worker unavailable"})
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sync pair id"})
		return
	}
	if err := h.sync.RunNow(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrCloudSyncPairNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sync pair not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	slog.Info("cloud: sync pair queued for manual run", "pair_id", id)
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// storageError maps rclone-layer failures to status codes.
func (h *CloudHandler) storageError(w http.ResponseWriter, err error) {	if errors.Is(err, cloudmgr.ErrRCloneMissing) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	slog.Warn("cloud: storage detail failed", "error", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "storage backend error — is this account a storage provider?"})
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
