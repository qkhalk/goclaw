package http

import (
	"net/http"

	"github.com/nextlevelbuilder/goclaw/internal/sysstats"
)

// SystemStatsHandler exposes host/process metrics for the dashboard System
// card.
//
//	GET /v1/system/stats — CPU / memory / disk / host / proc snapshot
type SystemStatsHandler struct {
	sampler  *sysstats.Sampler
	diskPath string
}

// NewSystemStatsHandler creates a SystemStatsHandler. diskPath is the
// directory whose filesystem usage is reported (gateway data dir); empty
// omits the disk section.
func NewSystemStatsHandler(diskPath string) *SystemStatsHandler {
	return &SystemStatsHandler{
		sampler:  sysstats.NewSampler(),
		diskPath: diskPath,
	}
}

// RegisterRoutes registers the system stats routes on the given mux.
func (h *SystemStatsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/system/stats", requireAuth("", h.handleStats))
}

func (h *SystemStatsHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.sampler.Snapshot(h.diskPath))
}
