package video

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// Dispatcher polls the video worker for status updates, moves deliverables
// into the workspace, and broadcasts WS events on status changes.
// Lifecycle: Start launches the goroutine; Stop signals shutdown and waits.
type Dispatcher struct {
	cfg         *config.VideoConfig
	store       store.VideoRenderJobStore
	worker      *WorkerClient
	eventPub    bus.EventPublisher
	workspace   string
	signal      chan struct{}
	done        chan struct{}
}

// NewDispatcher creates a Dispatcher. The workspace is the root directory
// where completed MP4 files are placed.
func NewDispatcher(
	cfg *config.VideoConfig,
	videoJobs store.VideoRenderJobStore,
	worker *WorkerClient,
	eventPub bus.EventPublisher,
	workspace string,
) *Dispatcher {
	return &Dispatcher{
		cfg:       cfg,
		store:     videoJobs,
		worker:    worker,
		eventPub:  eventPub,
		workspace: workspace,
		signal:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

// Start launches the background polling loop. Safe to call once.
func (d *Dispatcher) Start() {
	go d.loop()
	slog.Info("video dispatcher started", "poll_interval", d.cfg.EffectivePollInterval())
}

// Stop signals shutdown and blocks until the loop exits.
func (d *Dispatcher) Stop() {
	close(d.signal)
	<-d.done
}

// Notify signals the dispatcher to run an immediate poll cycle (called after
// a new job is created so it gets picked up without waiting for the next tick).
func (d *Dispatcher) Notify() {
	select {
	case d.signal <- struct{}{}:
	default:
		// already signaled, skip
	}
}

func (d *Dispatcher) loop() {
	defer close(d.done)

	ticker := time.NewTicker(d.cfg.EffectivePollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-d.done:
			return
		case <-d.signal:
			d.pollOnce()
		case <-ticker.C:
			d.pollOnce()
		}
	}
}

// pollOnce processes all rendering jobs: submits queued ones, polls active
// ones, and handles timeouts.
func (d *Dispatcher) pollOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Claim and submit any queued jobs.
	for {
		job, err := d.store.ClaimNextQueued(ctx)
		if err != nil {
			break // ErrNoQueuedJobs or other error
		}
		d.submitJob(ctx, job)
	}

	// 2. Poll all rendering jobs for status updates.
	//    ListByTenant is tenant-scoped, but the dispatcher operates in a
	//    system context. We use a special "system" tenant scope by listing
	//    without explicit tenant (the store implementation handles this for
	//    unscoped queries). For now we poll rendering jobs via the store.
	//    We iterate via a simple approach: get the job by scanning active.
	d.pollRenderingJobs(ctx)

	// 3. Cleanup expired jobs.
	d.cleanupExpired(ctx)
}

// submitJob pushes a queued job to the worker.
func (d *Dispatcher) submitJob(ctx context.Context, job *store.VideoRenderJob) {
	var sb Storyboard
	if err := json.Unmarshal([]byte(job.StoryboardJSON), &sb); err != nil {
		slog.Error("video.dispatcher: invalid storyboard in job",
			"job_id", job.ID, "error", err)
		d.failJob(ctx, job.ID, "invalid storyboard: "+err.Error())
		return
	}

	submit := &SubmitJob{
		JobID:      job.ID,
		Storyboard: &sb,
		AssetsDir:  d.assetsDir(job.ID),
	}

	resp, err := d.worker.SubmitJob(ctx, submit)
	if err != nil {
		slog.Error("video.dispatcher: submit to worker failed",
			"job_id", job.ID, "error", err)
		d.failJob(ctx, job.ID, "worker submit failed: "+err.Error())
		return
	}

	slog.Info("video.dispatcher: job submitted to worker",
		"job_id", job.ID, "worker_status", resp.Status)

	if err := d.store.UpdateStatus(ctx, job.ID, store.VideoJobUpdate{
		Status: string(JobRendering),
	}); err != nil {
		slog.Error("video.dispatcher: update to rendering failed",
			"job_id", job.ID, "error", err)
	}

	d.broadcastEvent(job.ID, string(resp.Status), 0, "", "")
}

// pollRenderingJobs checks all active jobs for status changes.
// Since we don't have a ListByStatus store method, we track via the
// submit flow. For a production implementation this would query
// rendering-status jobs. We use a simple heuristic: the dispatcher
// only knows about jobs it has submitted, so we maintain an in-memory
// set. For simplicity in Phase 3, we poll jobs that are in rendering
// state via the store's ListByTenant with a generous limit.
func (d *Dispatcher) pollRenderingJobs(ctx context.Context) {
	// Use a system-scoped context (empty tenant = unscoped query for dispatcher).
	systemCtx := ctx

	jobs, err := d.store.ListByTenant(systemCtx, 50)
	if err != nil {
		slog.Warn("video.dispatcher: failed to list jobs for polling", "error", err)
		return
	}

	now := time.Now()
	timeout := d.cfg.EffectiveJobTimeout()

	for i := range jobs {
		job := &jobs[i]
		switch job.Status {
		case string(JobRendering):
			d.pollSingleJob(ctx, job, now, timeout)
		case string(JobDone), string(JobFailed), string(JobCancelled):
			// Terminal — nothing to do here.
		}
	}
}

// pollSingleJob checks one rendering job against the worker.
func (d *Dispatcher) pollSingleJob(ctx context.Context, job *store.VideoRenderJob, now time.Time, timeout time.Duration) {
	// Check timeout.
	if job.StartedAt != nil && now.Sub(*job.StartedAt) > timeout {
		slog.Warn("video.dispatcher: job timed out",
			"job_id", job.ID, "elapsed", now.Sub(*job.StartedAt))
		d.failJob(ctx, job.ID, "render timed out")
		return
	}

	state, err := d.worker.GetJobStatus(ctx, job.ID)
	if err != nil {
		slog.Warn("video.dispatcher: poll failed",
			"job_id", job.ID, "error", err)
		// Don't fail immediately — worker may be temporarily unreachable.
		return
	}

	switch state.Status {
	case JobDone:
		d.completeJob(ctx, job, state)
	case JobFailed:
		d.failJob(ctx, job.ID, state.Error)
	case JobCancelled:
		d.store.UpdateStatus(ctx, job.ID, store.VideoJobUpdate{
			Status: string(JobCancelled),
		})
		d.broadcastEvent(job.ID, string(JobCancelled), 0, "", "")
	case JobRendering, JobQueued:
		// Still in progress — broadcast progress if changed.
		d.broadcastEvent(job.ID, string(state.Status), state.Progress, "", "")
	}
}

// completeJob moves the output file into the workspace and marks done.
func (d *Dispatcher) completeJob(ctx context.Context, job *store.VideoRenderJob, state *JobState) {
	// Move output from worker assets dir to workspace.
	workspacePath := filepath.Join(d.workspace, "videos", job.ID+".mp4")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0755); err != nil {
		slog.Error("video.dispatcher: create workspace dir failed",
			"job_id", job.ID, "error", err)
		d.failJob(ctx, job.ID, "failed to create output directory")
		return
	}

	// If worker provided an output path, copy it to workspace.
	if state.OutputPath != "" {
		if err := copyFile(state.OutputPath, workspacePath); err != nil {
			slog.Error("video.dispatcher: copy output failed",
				"job_id", job.ID, "src", state.OutputPath, "error", err)
			d.failJob(ctx, job.ID, "failed to copy output file")
			return
		}
	}

	// Get file size.
	var sizeBytes int64
	if fi, err := os.Stat(workspacePath); err == nil {
		sizeBytes = fi.Size()
	}

	if err := d.store.UpdateStatus(ctx, job.ID, store.VideoJobUpdate{
		Status:          string(JobDone),
		OutputPath:      &workspacePath,
		OutputSizeBytes: &sizeBytes,
	}); err != nil {
		slog.Error("video.dispatcher: update to done failed",
			"job_id", job.ID, "error", err)
		return
	}

	slog.Info("video.dispatcher: job completed",
		"job_id", job.ID, "output", workspacePath, "size", sizeBytes)

	d.broadcastEvent(job.ID, string(JobDone), 100, workspacePath, "")
}

// failJob marks a job as failed and broadcasts the event.
func (d *Dispatcher) failJob(ctx context.Context, jobID, errMsg string) {
	if err := d.store.UpdateStatus(ctx, jobID, store.VideoJobUpdate{
		Status: string(JobFailed),
		Error:  &errMsg,
	}); err != nil {
		slog.Error("video.dispatcher: update to failed failed",
			"job_id", jobID, "error", err)
	}
	d.broadcastEvent(jobID, string(JobFailed), 0, "", errMsg)
}

// cleanupExpired removes terminal jobs past their TTL.
func (d *Dispatcher) cleanupExpired(ctx context.Context) {
	ttl := 7 * 24 * time.Hour // 7 days default
	before := time.Now().Add(-ttl)
	deleted, err := d.store.DeleteExpired(ctx, before)
	if err != nil {
		slog.Warn("video.dispatcher: cleanup failed", "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("video.dispatcher: cleaned up expired jobs", "count", deleted)
	}
}

// broadcastEvent publishes a video.job.updated WS event.
func (d *Dispatcher) broadcastEvent(jobID, status string, progress int, outputPath, errMsg string) {
	if d.eventPub == nil {
		return
	}
	payload := map[string]any{
		"jobId":    jobID,
		"status":   status,
		"progress": progress,
	}
	if outputPath != "" {
		payload["outputPath"] = outputPath
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}

	// Extract tenant ID from the job context. Since the dispatcher operates
	// system-wide, we broadcast to all connected clients via nil tenant
	// (the event filter handles routing).
	d.eventPub.Broadcast(bus.Event{
		Name:    protocol.EventVideoJobUpdated,
		Payload: payload,
	})
}

// assetsDir returns the local directory for a job's mirrored assets.
func (d *Dispatcher) assetsDir(jobID string) string {
	return filepath.Join(d.workspace, ".video-assets", jobID)
}

// copyFile copies a file from src to dst, creating parent dirs as needed.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// --- VideoStack bundles everything the gateway needs for the video pipeline.

// VideoStack holds the components of the video render pipeline.
type VideoStack struct {
	VideoJobs  store.VideoRenderJobStore
	Worker     *WorkerClient
	Dispatcher *Dispatcher
}

// newVideoStack builds the shared video pipeline components. Returns nil when
// the video surface is disabled by config.
func newVideoStack(cfg *config.Config, stores *store.Stores, workspace string, eventPub bus.EventPublisher) *VideoStack {
	if !cfg.Video.VideoEnabled() {
		slog.Debug("video: disabled by config")
		return nil
	}
	if stores == nil || stores.VideoJobs == nil {
		slog.Warn("video: job store unavailable")
		return nil
	}
	workerURL := cfg.Video.WorkerURL
	if workerURL == "" {
		slog.Warn("video: worker_url not configured")
		return nil
	}

	worker := NewWorkerClient(workerURL, cfg.Video.WorkerToken, 10*time.Second, 5*time.Second)
	disp := NewDispatcher(&cfg.Video, stores.VideoJobs, worker, eventPub, workspace)

	return &VideoStack{
		VideoJobs:  stores.VideoJobs,
		Worker:     worker,
		Dispatcher: disp,
	}
}

// wireVideoTools starts the dispatcher goroutine. Tool registration happens
// in cmd/gateway_video.go where the tool has access to all dependencies.
// Returns a cleanup func (safe to defer).
func wireVideoTools(stack *VideoStack) func() {
	if stack == nil {
		return func() {}
	}

	// Start the dispatcher goroutine.
	stack.Dispatcher.Start()

	return func() {
		stack.Dispatcher.Stop()
	}
}

// tenantCtx builds a context with the given tenant ID for store queries.
func tenantCtx(tenantID string) context.Context {
	ctx := context.Background()
	if tenantID != "" {
		if tid, err := uuid.Parse(tenantID); err == nil {
			ctx = store.WithTenantID(ctx, tid)
		}
	}
	return ctx
}
