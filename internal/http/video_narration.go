package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	videopkg "github.com/nextlevelbuilder/goclaw/internal/video"
)

// Client-uploaded narration for video render jobs. The browser studio
// synthesizes scene audio locally (plain edge voices, or cloned voices via the
// in-browser tone converter) and uploads the rendered WAV here; the video
// dispatcher then hands the file to the worker via SubmitJob.Narration instead
// of re-synthesizing text server-side.
//
// Storage goes through VideoNarrationStore (implemented by
// *video.Dispatcher, which owns the per-job assets dir). The setter below is
// the wiring point; until cmd wiring calls it the route answers 503, mirroring
// how /v1/tts/clone/voices answers without a clone worker.
//
// Uploads are validated against the JOB: the id must exist in the caller's
// tenant (the store Get is tenant-scoped — cross-tenant ids 404, no existence
// oracle) and still be queued. NOTE the gateway dispatches queued jobs
// immediately after creation, so in practice the queued window is tiny — this
// endpoint stays unwired until a client-declared "narration ready" attach
// flow exists; the checks keep it safe to wire piecemeal.
//
// Validation mirrors tts_clone.go (ext whitelist, hard size cap) with a 20MB
// cap — 60s of 22.05kHz mono WAV is ~5MB, so 20MB covers any valid clip.

const (
	// maxVideoNarrationUploadBytes caps one scene's narration clip. A minute
	// of 22.05kHz mono 16-bit WAV is ~5MB; 20MB leaves headroom for 3-minute
	// clips without letting uploads become a memory DoS.
	maxVideoNarrationUploadBytes = 20 << 20
	// maxVideoNarrationScene is the highest valid 0-based scene index
	// (storyboards validate 1..60 scenes).
	maxVideoNarrationScene = 59
)

// videoNarrationExts lists accepted narration audio extensions — same
// containers the clone reference upload accepts.
var videoNarrationExts = map[string]bool{
	".wav": true, ".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".webm": true, ".flac": true,
}

// VideoNarrationStore persists a narration clip for one scene of a render job
// and reports job existence/status for upload validation.
type VideoNarrationStore interface {
	SaveNarration(jobID string, sceneIndex int, ext string, data []byte) (string, error)
	// NarrationJob reports whether the job exists in the caller's tenant
	// (context-scoped) and its lifecycle status. ok=false covers unknown ids
	// and cross-tenant ids alike.
	NarrationJob(ctx context.Context, jobID string) (status string, ok bool)
}

// SetVideoNarrationStore wires the narration store (the video dispatcher).
// Nil-able: the endpoint answers 503 until configured.
func (h *TTSHandler) SetVideoNarrationStore(s VideoNarrationStore) { h.videoNarration = s }

// handleVideoNarrationUpload serves POST /v1/video/narration — multipart
// upload (fields: job_id, scene_index, file) stored into the job's assets dir.
func (h *TTSHandler) handleVideoNarrationUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := store.LocaleFromContext(ctx)

	// Rate limit (same per-IP/token limiter as /v1/tts/synthesize).
	if h.rateLimiter != nil {
		key := r.RemoteAddr
		if tok := extractBearerToken(r); tok != "" {
			key = "token:" + tok
		}
		if !h.rateLimiter(key) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, fmt.Sprintf(`{"error":%q}`, i18n.T(locale, i18n.MsgRateLimitExceeded)), http.StatusTooManyRequests)
			return
		}
	}

	h.mu.RLock()
	st := h.videoNarration
	h.mu.RUnlock()
	if st == nil {
		http.Error(w, `{"error":"video narration upload is not configured on this install"}`, http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxVideoNarrationUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, `{"error":"narration audio exceeds 20MB"}`, http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, `{"error":"invalid multipart form (20MB upload cap)"}`, http.StatusBadRequest)
		return
	}

	jobID := strings.TrimSpace(r.FormValue("job_id"))
	if _, err := uuid.Parse(jobID); err != nil {
		http.Error(w, `{"error":"job_id must be a UUID"}`, http.StatusBadRequest)
		return
	}

	// Existence + ownership (tenant-scoped lookup) + lifecycle: only queued
	// jobs accept narration, so uploads can never poison a rendering job or
	// create orphan asset dirs for ids that were never jobs.
	status, ok := st.NarrationJob(ctx, jobID)
	if !ok {
		http.Error(w, `{"error":"render job not found"}`, http.StatusNotFound)
		return
	}
	if status != string(videopkg.JobQueued) {
		http.Error(w, `{"error":"job is no longer queued — attach narration before rendering starts"}`, http.StatusConflict)
		return
	}

	sceneIdx, err := strconv.Atoi(strings.TrimSpace(r.FormValue("scene_index")))
	if err != nil || sceneIdx < 0 || sceneIdx > maxVideoNarrationScene {
		http.Error(w, fmt.Sprintf(`{"error":"scene_index must be 0..%d"}`, maxVideoNarrationScene), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file field is required (narration audio)"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	dot := strings.LastIndex(header.Filename, ".")
	if dot < 0 || !videoNarrationExts[strings.ToLower(header.Filename[dot:])] {
		http.Error(w, `{"error":"unsupported audio format (use wav, mp3, m4a, aac, ogg, webm or flac)"}`, http.StatusUnsupportedMediaType)
		return
	}
	ext := strings.ToLower(header.Filename[dot+1:])

	data, err := io.ReadAll(io.LimitReader(file, maxVideoNarrationUploadBytes+1))
	if err != nil {
		http.Error(w, `{"error":"failed reading upload"}`, http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, `{"error":"empty audio file"}`, http.StatusBadRequest)
		return
	}
	if len(data) > maxVideoNarrationUploadBytes {
		http.Error(w, `{"error":"narration audio exceeds 20MB"}`, http.StatusRequestEntityTooLarge)
		return
	}

	path, err := st.SaveNarration(jobID, sceneIdx, ext, data)
	if err != nil {
		slog.Warn("video.narration.upload.failed", "job_id", jobID, "scene", sceneIdx, "bytes", len(data), "error", err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	slog.Info("video.narration.upload.ok", "job_id", jobID, "scene", sceneIdx, "bytes", len(data))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "job_id": jobID, "scene_index": sceneIdx, "audio_path": path})
}
