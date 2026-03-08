package http

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// handleListInstances returns all user instances for a predefined agent.
func (h *AgentsHandler) handleListInstances(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if userID != "" && ag.OwnerID != userID && !h.isOwnerUser(userID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can view instances"})
		return
	}

	instances, err := h.agents.ListUserInstances(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"instances": instances})
}

// handleGetInstanceFiles returns user context files for a specific instance.
func (h *AgentsHandler) handleGetInstanceFiles(w http.ResponseWriter, r *http.Request) {
	callerID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}
	instanceUserID := r.PathValue("userID")
	if instanceUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing userID"})
		return
	}

	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if callerID != "" && ag.OwnerID != callerID && !h.isOwnerUser(callerID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can view instance files"})
		return
	}

	files, err := h.agents.GetUserContextFiles(r.Context(), id, instanceUserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})
}

// handleSetInstanceFile updates a user context file for a specific instance.
func (h *AgentsHandler) handleSetInstanceFile(w http.ResponseWriter, r *http.Request) {
	callerID := store.UserIDFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}
	instanceUserID := r.PathValue("userID")
	fileName := r.PathValue("fileName")
	if instanceUserID == "" || fileName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing userID or fileName"})
		return
	}

	// Only USER.md can be edited via this endpoint — other files are managed by the agent itself.
	if fileName != "USER.md" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only USER.md can be edited via this endpoint"})
		return
	}

	ag, err := h.agents.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if callerID != "" && ag.OwnerID != callerID && !h.isOwnerUser(callerID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owner can edit instance files"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if err := h.agents.SetUserContextFile(r.Context(), id, instanceUserID, fileName, payload.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Invalidate caches so the agent picks up the change immediately
	h.emitCacheInvalidate(bus.CacheKindBootstrap, id.String())

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
