package http

import (
	"net/http"
	"regexp"

	"github.com/nextlevelbuilder/goclaw/internal/browse"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
)

// validBrowseID matches relay ids minted by browse.Store (hex or fallback).
var validBrowseID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// BrowseRelayHandler serves sanitized documents for the client-side browsing
// panel. The iframe loading this path cannot send an Authorization header,
// so auth mirrors the media serve pattern: a short-lived signed ?ft= token
// minted when the browse invoke was sent, with Bearer as the API-client
// fallback.
type BrowseRelayHandler struct {
	store *browse.Store
}

// NewBrowseRelayHandler creates the relay handler.
func NewBrowseRelayHandler(store *browse.Store) *BrowseRelayHandler {
	return &BrowseRelayHandler{store: store}
}

// RegisterRoutes registers the browse relay endpoint.
func (h *BrowseRelayHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/browse/{id}", h.auth(h.handleServe))
}

func (h *BrowseRelayHandler) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Priority 1: short-lived signed file token (?ft=) — decoupled from
		// gateway token, minted per browse invoke (5-minute TTL).
		if ft := r.URL.Query().Get("ft"); ft != "" {
			browseID := r.PathValue("id")
			if VerifyFileToken(ft, "/v1/browse/"+browseID, FileSigningKey()) {
				next(w, r)
				return
			}
			http.Error(w, "invalid or expired file token", http.StatusUnauthorized)
			return
		}
		// Priority 2: Bearer header (API clients only).
		provided := extractBearerToken(r)
		authedReq, ok := requireAuthBearer("", provided, w, r)
		if !ok {
			return
		}
		next(w, authedReq)
	}
}

func (h *BrowseRelayHandler) handleServe(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	id := r.PathValue("id")
	if !validBrowseID.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidRequest, "invalid browse id")})
		return
	}

	entry, ok := h.store.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	contentType := entry.ContentType
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	// Defense in depth: the document is already sanitized script-free; this
	// CSP stops any sanitization regression from executing on our origin.
	w.Header().Set("Content-Security-Policy", "script-src 'none'; object-src 'none'; frame-ancestors 'self'")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(entry.HTML))
}
