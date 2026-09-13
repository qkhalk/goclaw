package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	videopkg "github.com/nextlevelbuilder/goclaw/internal/video"
)

// VideoHandler exposes the HTTP API for video render jobs.
//
//	GET    /v1/video/jobs       — list jobs for caller's tenant
//	GET    /v1/video/jobs/{id}  — get one job by ID
//	DELETE /v1/video/jobs/{id}  — cancel a queued/rendering job
type VideoHandler struct {
	videoJobs store.VideoRenderJobStore
	worker    *videopkg.WorkerClient
	enabled   bool
}

// NewVideoHandler creates a VideoHandler.
func NewVideoHandler(videoJobs store.VideoRenderJobStore, worker *videopkg.WorkerClient, enabled bool) *VideoHandler {
	return &VideoHandler{
		videoJobs: videoJobs,
		worker:    worker,
		enabled:   enabled,
	}
}

// RegisterRoutes registers all video routes on the given mux.
func (h *VideoHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/video/jobs", requireAuth("", h.handleListJobs))
	mux.HandleFunc("GET /v1/video/jobs/{id}", requireAuth("", h.handleGetJob))
	mux.HandleFunc("DELETE /v1/video/jobs/{id}", requireAuth("", h.handleCancelJob))
}

// --- GET /v1/video/jobs ---

func (h *VideoHandler) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	jobs, err := h.videoJobs.ListByTenant(r.Context(), limit)
	if err != nil {
		slog.Error("video: list jobs failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list video jobs",
		})
		return
	}
	if jobs == nil {
		jobs = []store.VideoRenderJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

// --- GET /v1/video/jobs/{id} ---

func (h *VideoHandler) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid job id"})
		return
	}

	job, err := h.videoJobs.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrVideoJobNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}
		slog.Error("video: get job failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get job"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// --- DELETE /v1/video/jobs/{id} ---

func (h *VideoHandler) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid job id"})
		return
	}

	job, err := h.videoJobs.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrVideoJobNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}
		slog.Error("video: get job for cancel failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get job"})
		return
	}

	// Only queued or rendering jobs can be cancelled.
	if job.Status != string(videopkg.JobQueued) && job.Status != string(videopkg.JobRendering) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "job cannot be cancelled (status: " + job.Status + ")",
		})
		return
	}

	// Try to cancel on the worker first (best-effort for rendering jobs).
	if job.Status == string(videopkg.JobRendering) && h.worker != nil {
		if _, werr := h.worker.CancelJob(r.Context(), id); werr != nil {
			slog.Warn("video: worker cancel failed (proceeding with local cancel)", "job_id", id, "error", werr)
		}
	}

	// Update status locally.
	if err := h.videoJobs.UpdateStatus(r.Context(), id, store.VideoJobUpdate{
		Status: string(videopkg.JobCancelled),
	}); err != nil {
		slog.Error("video: cancel job failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to cancel job"})
		return
	}

	slog.Info("video: job cancelled", "job_id", id)
	writeJSON(w, http.StatusOK, map[string]string{
		"jobId":  id,
		"status": string(videopkg.JobCancelled),
	})
}

// available writes the gate response (403) when the Video surface is off,
// returning true when it is on.
func (h *VideoHandler) available(w http.ResponseWriter) bool {
	if !h.enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "video surface is not enabled"})
		return false
	}
	if h.videoJobs == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "video job store unavailable"})
		return false
	}
	return true
}
