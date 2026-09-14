package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	videopkg "github.com/nextlevelbuilder/goclaw/internal/video"
)

// maxStoryboardBytes caps the inline storyboard JSON body size.
const maxStoryboardBytes = 1 << 20 // 1 MB

// VideoHandler exposes the HTTP API for video render jobs.
//
//	POST   /v1/video/jobs       — create a job from a storyboard JSON
//	GET    /v1/video/jobs       — list jobs for caller's tenant
//	GET    /v1/video/jobs/{id}  — get one job by ID
//	DELETE /v1/video/jobs/{id}  — cancel a queued/rendering job
type VideoHandler struct {
	videoJobs  store.VideoRenderJobStore
	worker     *videopkg.WorkerClient
	dispatcher *videopkg.Dispatcher
	enabled    bool
}

// NewVideoHandler creates a VideoHandler. dispatcher may be nil (the job is
// then picked up on the dispatcher's next poll tick).
func NewVideoHandler(videoJobs store.VideoRenderJobStore, worker *videopkg.WorkerClient, dispatcher *videopkg.Dispatcher, enabled bool) *VideoHandler {
	return &VideoHandler{
		videoJobs:  videoJobs,
		worker:     worker,
		dispatcher: dispatcher,
		enabled:    enabled,
	}
}

// RegisterRoutes registers all video routes on the given mux.
func (h *VideoHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/video/jobs", requireAuth("", h.handleCreateJob))
	mux.HandleFunc("GET /v1/video/jobs", requireAuth("", h.handleListJobs))
	mux.HandleFunc("GET /v1/video/jobs/{id}", requireAuth("", h.handleGetJob))
	mux.HandleFunc("DELETE /v1/video/jobs/{id}", requireAuth("", h.handleCancelJob))
}

// --- POST /v1/video/jobs ---
// Body: {"storyboard": {...}} or {"storyboard_json": "<raw json>"} — the same
// shapes the render_video agent tool accepts.

func (h *VideoHandler) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxStoryboardBytes)
	var req struct {
		Storyboard     *json.RawMessage `json:"storyboard"`
		StoryboardJSON string           `json:"storyboard_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	var raw []byte
	switch {
	case req.Storyboard != nil:
		raw = []byte(*req.Storyboard)
	case req.StoryboardJSON != "":
		raw = []byte(req.StoryboardJSON)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provide storyboard or storyboard_json"})
		return
	}

	// Validate against the shared contract (same rules as the agent tool).
	var sb videopkg.Storyboard
	if err := json.Unmarshal(raw, &sb); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid storyboard JSON: " + err.Error()})
		return
	}
	if err := sb.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "storyboard validation failed: " + err.Error()})
		return
	}

	tenantID := store.TenantIDFromContext(r.Context())
	userID := store.UserIDFromContext(r.Context())

	jobID := uuid.New().String()
	now := time.Now()
	job := &store.VideoRenderJob{
		ID:             jobID,
		TenantID:       tenantID.String(),
		UserID:         userID,
		Status:         string(videopkg.JobQueued),
		Engine:         "ffmpeg",
		StoryboardJSON: string(raw),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := h.videoJobs.Create(r.Context(), job); err != nil {
		slog.Error("video: create job failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create render job"})
		return
	}
	if h.dispatcher != nil {
		h.dispatcher.Notify()
	}

	slog.Info("video: job created via http", "job_id", jobID, "scenes", len(sb.Scenes), "user_id", userID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"jobId":  jobID,
		"status": string(videopkg.JobQueued),
		"scenes": len(sb.Scenes),
	})
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
