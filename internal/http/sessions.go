package http

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// SessionsHandler exposes read-only HTTP session discovery for automation.
type SessionsHandler struct {
	sessions store.SessionStore
	ownerIDs []string
}

func NewSessionsHandler(s store.SessionStore, ownerIDs []string) *SessionsHandler {
	return &SessionsHandler{sessions: s, ownerIDs: ownerIDs}
}

func (h *SessionsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/sessions", requireAuth("", h.handleList))
}

func (h *SessionsHandler) handleList(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	userID := store.UserIDFromContext(r.Context())
	role := permissions.Role(store.RoleFromContext(r.Context()))

	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	offset := parseNonNegativeInt(r.URL.Query().Get("offset"), 0)
	opts := store.SessionListOpts{
		AgentID:  firstSessionQueryValue(r.URL.Query().Get("agent_id"), r.URL.Query().Get("agentId")),
		Channel:  r.URL.Query().Get("channel"),
		Limit:    limit,
		Offset:   offset,
		TenantID: store.TenantIDFromContext(r.Context()),
	}

	if !canSeeAllHTTP(role, h.ownerIDs, userID) {
		if userID == "" {
			writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgUserIDHeader))
			return
		}
		opts.UserID = userID
	}

	result := h.sessions.ListPagedRich(r.Context(), opts)
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": result.Sessions,
		"total":    result.Total,
		"limit":    limit,
		"offset":   offset,
	})
}

func canSeeAllHTTP(role permissions.Role, ownerIDs []string, userID string) bool {
	if permissions.HasMinRole(role, permissions.RoleAdmin) {
		return true
	}
	return userID != "" && slices.Contains(ownerIDs, userID)
}

func parsePositiveInt(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func parseNonNegativeInt(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func firstSessionQueryValue(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
