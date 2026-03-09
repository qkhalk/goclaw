package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PendingMessagesHandler handles pending message HTTP endpoints.
type PendingMessagesHandler struct {
	store       store.PendingMessageStore
	token       string
	providerReg *providers.Registry
}

func NewPendingMessagesHandler(s store.PendingMessageStore, token string, providerReg *providers.Registry) *PendingMessagesHandler {
	return &PendingMessagesHandler{store: s, token: token, providerReg: providerReg}
}

func (h *PendingMessagesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/pending-messages", h.authMiddleware(h.handleListGroups))
	mux.HandleFunc("GET /v1/pending-messages/messages", h.authMiddleware(h.handleListMessages))
	mux.HandleFunc("DELETE /v1/pending-messages", h.authMiddleware(h.handleDelete))
	mux.HandleFunc("POST /v1/pending-messages/compact", h.authMiddleware(h.handleCompact))
}

func (h *PendingMessagesHandler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.token != "" {
			if extractBearerToken(r) != h.token {
				locale := extractLocale(r)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": i18n.T(locale, i18n.MsgUnauthorized)})
				return
			}
		}
		locale := extractLocale(r)
		ctx := store.WithLocale(r.Context(), locale)
		r = r.WithContext(ctx)
		next(w, r)
	}
}

// GET /v1/pending-messages — list all groups with resolved titles
func (h *PendingMessagesHandler) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.store.ListGroups(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Resolve group titles from session metadata (best-effort, non-blocking)
	if titles, err := h.store.ResolveGroupTitles(r.Context(), groups); err == nil {
		for i := range groups {
			if t, ok := titles[groups[i].ChannelName+":"+groups[i].HistoryKey]; ok {
				groups[i].GroupTitle = t
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"groups": groups})
}

// GET /v1/pending-messages/messages?channel=X&key=Y — list messages for a group
func (h *PendingMessagesHandler) handleListMessages(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	channel := r.URL.Query().Get("channel")
	key := r.URL.Query().Get("key")
	if channel == "" || key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgChannelKeyReq)})
		return
	}

	msgs, err := h.store.ListByKey(r.Context(), channel, key)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"messages": msgs})
}

// DELETE /v1/pending-messages?channel=X&key=Y — clear a group
func (h *PendingMessagesHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	channel := r.URL.Query().Get("channel")
	key := r.URL.Query().Get("key")
	if channel == "" || key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgChannelKeyReq)})
		return
	}

	if err := h.store.DeleteByKey(r.Context(), channel, key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type compactRequest struct {
	ChannelName string `json:"channel_name"`
	HistoryKey  string `json:"history_key"`
}

// POST /v1/pending-messages/compact — LLM-based summarization of old messages, keeping recent ones.
// Falls back to hard delete if no LLM provider is available.
func (h *PendingMessagesHandler) handleCompact(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	var req compactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidJSON)})
		return
	}
	if req.ChannelName == "" || req.HistoryKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgRequired, "channel_name and history_key")})
		return
	}

	// Resolve an LLM provider for summarization
	provider := h.resolveProvider()
	if provider == nil {
		// Fallback: hard delete if no provider available
		slog.Warn("compact.no_provider", "channel", req.ChannelName, "key", req.HistoryKey)
		if err := h.store.DeleteByKey(r.Context(), req.ChannelName, req.HistoryKey); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "method": "deleted"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	remaining, err := channels.CompactGroup(ctx, h.store, req.ChannelName, req.HistoryKey, provider, provider.DefaultModel(), 15)
	if err != nil {
		slog.Warn("compact.failed", "channel", req.ChannelName, "key", req.HistoryKey, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "method": "summarized", "remaining": remaining})
}

// resolveProvider returns the first available LLM provider, or nil.
func (h *PendingMessagesHandler) resolveProvider() providers.Provider {
	if h.providerReg == nil {
		return nil
	}
	names := h.providerReg.List()
	if len(names) == 0 {
		return nil
	}
	p, err := h.providerReg.Get(names[0])
	if err != nil {
		return nil
	}
	return p
}
