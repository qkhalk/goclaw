package http

// CopilotOAuthHandler serves /v1/auth/copilot/{provider}/... using the GitHub
// device flow: start returns a user_code the UI displays while a background
// poller completes the grant. One in-flight flow per tenant:user:provider.

import (
	"context"
	"errors"
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

// startLoginCopilotFunc is a seam so handler tests can stub the device flow
// without touching github.com.
var startLoginCopilotFunc = oauth.StartLoginCopilot

type CopilotOAuthHandler struct {
	provStore   store.ProviderStore
	providerReg *providers.Registry
	msgBus      *bus.MessageBus

	mu      sync.Mutex
	pending map[string]*pendingCopilotFlow
}

type pendingCopilotFlow struct {
	login        *oauth.PendingCopilotLogin
	flowKey      string
	tenantID     uuid.UUID
	userID       string
	providerName string
	displayName  string
	cancel       context.CancelFunc
	createdAt    time.Time
	// canceled is set by logout; the background waiter checks it before and
	// after persisting so a logout that races the device grant wins.
	canceled bool
	saving   bool
	// lastError surfaces a failed grant via /status (login without a Copilot
	// subscription, network failure, …) instead of an endless "waiting" UI.
	lastError string
}

func NewCopilotOAuthHandler(provStore store.ProviderStore, providerReg *providers.Registry, msgBus *bus.MessageBus) *CopilotOAuthHandler {
	return &CopilotOAuthHandler{
		provStore:   provStore,
		providerReg: providerReg,
		msgBus:      msgBus,
		pending:     make(map[string]*pendingCopilotFlow),
	}
}

func (h *CopilotOAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/auth/copilot/{provider}/status", h.readAuth(h.handleCopilotStatus))
	mux.HandleFunc("POST /v1/auth/copilot/{provider}/start", h.auth(h.handleCopilotStart))
	mux.HandleFunc("POST /v1/auth/copilot/{provider}/logout", h.auth(h.handleCopilotLogout))
}

func (h *CopilotOAuthHandler) auth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(permissions.RoleAdmin, next)
}

func (h *CopilotOAuthHandler) readAuth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth("", next)
}

func (h *CopilotOAuthHandler) newTokenSource(ctx context.Context, providerName, displayName string) *oauth.CopilotDBTokenSource {
	return oauth.NewCopilotDBTokenSource(h.provStore, providerName).
		WithTenantID(oauthTenantID(ctx)).
		WithProviderMeta(displayName, "")
}

func (h *CopilotOAuthHandler) ensureProviderName(ctx context.Context, providerName string) error {
	if h.provStore == nil {
		return nil
	}
	p, err := h.provStore.GetProviderByName(ctx, providerName)
	if err != nil {
		return nil
	}
	if p.ProviderType != store.ProviderCopilotOAuth {
		return &oauth.ProviderTypeConflictError{
			ProviderName: providerName,
			ProviderType: p.ProviderType,
		}
	}
	return nil
}

func (h *CopilotOAuthHandler) handleCopilotStatus(w http.ResponseWriter, r *http.Request) {
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

func (h *CopilotOAuthHandler) handleCopilotStart(w http.ResponseWriter, r *http.Request) {
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
	h.mu.Lock()
	if pending := h.pending[flowKey]; pending != nil && time.Since(pending.createdAt) < oauth.CopilotFlowTimeout {
		errMsg := pending.lastError
		h.mu.Unlock()
		if errMsg != "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": errMsg})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user_code":        pending.login.UserCode,
			"verification_uri": pending.login.VerificationURI,
			"provider_name":    providerName,
		})
		return
	}

	pending, err := startLoginCopilotFunc()
	if err != nil {
		h.mu.Unlock()
		slog.Error("copilot_oauth.start", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": i18n.T(locale, i18n.MsgInternalError, "failed to start GitHub device flow"),
		})
		return
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), oauth.CopilotFlowTimeout)
	flow := &pendingCopilotFlow{
		login:        pending,
		flowKey:      flowKey,
		tenantID:     oauthTenantID(r.Context()),
		userID:       store.UserIDFromContext(r.Context()),
		providerName: providerName,
		displayName:  body.DisplayName,
		cancel:       cancel,
		createdAt:    time.Now(),
	}
	h.pending[flowKey] = flow
	h.mu.Unlock()

	go h.waitForDeviceGrant(waitCtx, flow)

	emitAudit(h.msgBus, r, "oauth.login_started", "oauth", "copilot")
	writeJSON(w, http.StatusOK, map[string]any{
		"user_code":        pending.UserCode,
		"verification_uri": pending.VerificationURI,
		"provider_name":    providerName,
	})
}

// waitForDeviceGrant completes the device flow in the background: validates
// Copilot access, saves the provider row, and registers it for the tenant.
// The flow entry stays in the map until the grant resolves so logout can
// cancel it and /status can surface a failure.
func (h *CopilotOAuthHandler) waitForDeviceGrant(ctx context.Context, flow *pendingCopilotFlow) {
	defer flow.login.Shutdown() // stop the github poller no matter how we exit

	tokenResp, err := flow.login.Wait(ctx)
	if err != nil {
		h.mu.Lock()
		if h.pending[flow.flowKey] == flow {
			if errors.Is(err, context.Canceled) {
				delete(h.pending, flow.flowKey)
				h.mu.Unlock()
				slog.Info("copilot_oauth flow canceled", "provider", flow.providerName)
				return
			}
			flow.lastError = err.Error()
		}
		h.mu.Unlock()
		if !errors.Is(err, context.Canceled) {
			slog.Warn("copilot_oauth.device_grant failed", "error", err)
		}
		return
	}

	// Bail out if logout raced the grant.
	h.mu.Lock()
	if flow.canceled {
		delete(h.pending, flow.flowKey)
		h.mu.Unlock()
		slog.Info("copilot_oauth grant discarded (logged out during flow)", "provider", flow.providerName)
		return
	}
	flow.saving = true
	h.mu.Unlock()

	saveCtx := store.WithTenantID(context.Background(), flow.tenantID)
	validation, verr := oauth.ValidateCopilotToken(saveCtx, tokenResp.AccessToken)
	var providerID uuid.UUID
	if verr == nil {
		ts := h.newTokenSource(saveCtx, flow.providerName, flow.displayName)
		providerID, verr = ts.SaveOAuthResult(saveCtx, tokenResp.AccessToken, validation.APIBase, validation.Login)
		if verr == nil && h.providerReg != nil {
			h.providerReg.RegisterForTenant(flow.tenantID, providers.NewCopilotProvider(flow.providerName, ts, validation.APIBase, ""))
		}
	}

	h.mu.Lock()
	canceled := flow.canceled
	delete(h.pending, flow.flowKey)
	h.mu.Unlock()

	if verr != nil {
		slog.Error("copilot_oauth grant failed", "error", verr)
		return
	}
	if canceled {
		// Logout won the race — revert the just-saved provider.
		if h.provStore != nil {
			ts := h.newTokenSource(saveCtx, flow.providerName, flow.displayName)
			_ = ts.Delete(saveCtx)
		}
		if h.providerReg != nil {
			h.providerReg.UnregisterForTenant(flow.tenantID, flow.providerName)
		}
		slog.Info("copilot_oauth grant reverted (logged out during save)", "provider", flow.providerName)
		return
	}
	slog.Info("copilot_oauth: token saved via device flow", "provider", flow.providerName, "provider_id", providerID.String(), "api_base", validation.APIBase)
}

func (h *CopilotOAuthHandler) handleCopilotLogout(w http.ResponseWriter, r *http.Request) {
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

	// Cancel any in-flight device flow for this tenant:user:provider. The
	// entry stays if a save is mid-flight — the waiter reverts it (canceled).
	flowKey := oauthFlowKey(r.Context(), providerName)
	h.mu.Lock()
	if flow := h.pending[flowKey]; flow != nil {
		flow.canceled = true
		flow.cancel()
		flow.login.Shutdown()
		if !flow.saving {
			delete(h.pending, flowKey)
		}
	}
	h.mu.Unlock()

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

	emitAudit(h.msgBus, r, "oauth.logout", "oauth", "copilot")
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}
