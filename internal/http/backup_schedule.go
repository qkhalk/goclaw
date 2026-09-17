package http

import (
	"encoding/json"
	"net/http"

	"github.com/nextlevelbuilder/goclaw/internal/backup"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// BackupScheduleHandler exposes the scheduled-backup config + status + manual
// run. Owner-only like the rest of the system backup surface (the schedule
// writes to the master secrets store).
type BackupScheduleHandler struct {
	sched   *backup.Scheduler
	isOwner func(string) bool
}

// NewBackupScheduleHandler builds the handler. sched may be nil (scheduler
// not wired, e.g. desktop edition) — routes then report unavailable.
func NewBackupScheduleHandler(sched *backup.Scheduler, isOwner func(string) bool) *BackupScheduleHandler {
	return &BackupScheduleHandler{sched: sched, isOwner: isOwner}
}

// RegisterRoutes registers schedule routes on the given mux.
func (h *BackupScheduleHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/system/backup/schedule", h.authMiddleware(h.handleGet))
	mux.HandleFunc("PUT /v1/system/backup/schedule", h.authMiddleware(h.handlePut))
	mux.HandleFunc("POST /v1/system/backup/schedule/run", h.authMiddleware(h.handleRun))
}

func (h *BackupScheduleHandler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := store.UserIDFromContext(r.Context())
		if !h.isOwner(userID) {
			writeError(w, http.StatusForbidden, protocol.ErrUnauthorized, "owner only")
			return
		}
		if !requireMasterScope(w, r) {
			return
		}
		next(w, r)
	}
}

func (h *BackupScheduleHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if h.sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "scheduler not available"})
		return
	}
	ctx := store.WithTenantID(r.Context(), store.MasterTenantID)
	cfg, last, nextDue, err := h.sched.Status(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"config":   cfg,
		"last_run": last,
		"next_due": nextDue,
	})
}

func (h *BackupScheduleHandler) handlePut(w http.ResponseWriter, r *http.Request) {
	if h.sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "scheduler not available"})
		return
	}
	locale := store.LocaleFromContext(r.Context())
	var cfg backup.ScheduleConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON))
		return
	}
	ctx := store.WithTenantID(r.Context(), store.MasterTenantID)
	if err := h.sched.Save(ctx, cfg); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, err.Error())
		return
	}
	saved, last, nextDue, err := h.sched.Status(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": saved, "last_run": last, "next_due": nextDue})
}

func (h *BackupScheduleHandler) handleRun(w http.ResponseWriter, r *http.Request) {
	if h.sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "scheduler not available"})
		return
	}
	ctx := store.WithTenantID(r.Context(), store.MasterTenantID)
	run, err := h.sched.RunNow(ctx)
	if err != nil {
		writeError(w, http.StatusConflict, protocol.ErrInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}
