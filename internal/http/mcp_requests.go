package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func (h *MCPHandler) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	var req store.MCPAccessRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.ServerID == uuid.Nil || req.Scope == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "server_id and scope are required"})
		return
	}
	if req.Scope != "agent" && req.Scope != "user" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be 'agent' or 'user'"})
		return
	}

	req.RequestedBy = store.UserIDFromContext(r.Context())
	req.Status = "pending"

	if err := h.store.CreateRequest(r.Context(), &req); err != nil {
		slog.Error("mcp.create_request", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, req)
}

func (h *MCPHandler) handleListPendingRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := h.store.ListPendingRequests(r.Context())
	if err != nil {
		slog.Error("mcp.list_pending_requests", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"requests": requests})
}

func (h *MCPHandler) handleReviewRequest(w http.ResponseWriter, r *http.Request) {
	requestID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request ID"})
		return
	}

	var req struct {
		Approved bool   `json:"approved"`
		Note     string `json:"note,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	reviewedBy := store.UserIDFromContext(r.Context())

	if err := h.store.ReviewRequest(r.Context(), requestID, req.Approved, reviewedBy, req.Note); err != nil {
		slog.Error("mcp.review_request", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if req.Approved {
		h.emitCacheInvalidate()
	}

	status := "rejected"
	if req.Approved {
		status = "approved"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}
