package vworker

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// Server is the HTTP mini-server for the videoworker.
type Server struct {
	runner  *Runner
	token   string
	mux     *http.ServeMux
}

// NewServer creates a new HTTP server backed by the given runner.
func NewServer(runner *Runner, token string) *Server {
	s := &Server{
		runner: runner,
		token:  token,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the root HTTP handler (for use with http.Server).
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /v1/jobs", s.handleSubmit)
	s.mux.HandleFunc("GET /v1/jobs/{id}", s.handleGetStatus)
	s.mux.HandleFunc("POST /v1/jobs/{id}/cancel", s.handleCancel)
	s.mux.HandleFunc("GET /health", s.handleHealth)
}

// ServeHTTP implements http.Handler, delegating to the internal mux.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleSubmit processes POST /v1/jobs
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB limit
	if err != nil {
		http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
		return
	}

	var job contract.SubmitJob
	if err := json.Unmarshal(body, &job); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if job.JobID == "" {
		http.Error(w, `{"error":"jobId required"}`, http.StatusBadRequest)
		return
	}
	if job.Storyboard == nil {
		http.Error(w, `{"error":"storyboard required"}`, http.StatusBadRequest)
		return
	}

	// Check queue capacity
	if s.runner.ActiveJobs() >= s.runner.cfg.MaxQueue {
		writeJSON(w, http.StatusConflict, contract.SubmitJobResponse{
			JobID:  job.JobID,
			Status: contract.JobFailed,
		})
		return
	}

	resp := s.runner.Submit(r.Context(), job)
	writeJSON(w, http.StatusAccepted, resp)
}

// handleGetStatus processes GET /v1/jobs/{id}
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"job id required"}`, http.StatusBadRequest)
		return
	}

	state, ok := s.runner.GetStatus(id)
	if !ok {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, state)
}

// handleCancel processes POST /v1/jobs/{id}/cancel
func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"job id required"}`, http.StatusBadRequest)
		return
	}

	status, ok := s.runner.Cancel(id)
	if !ok {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, contract.CancelResponse{
		JobID:  id,
		Status: status,
	})
}

// handleHealth processes GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"active": s.runner.ActiveJobs(),
	})
}

func (s *Server) authenticate(r *http.Request) bool {
	if s.token == "" {
		return true // no token configured = open (dev mode)
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}
	// Support "Bearer <token>" format
	token := strings.TrimPrefix(auth, "Bearer ")
	token = strings.TrimSpace(token)
	return token == s.token
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write JSON response", "err", err)
	}
}

// Start starts the HTTP server. Blocks until the server stops.
func (s *Server) Start(addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s,
	}
	slog.Info("videoworker HTTP server starting", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
