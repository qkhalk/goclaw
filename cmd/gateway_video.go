package cmd

import (
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	httpapi "github.com/nextlevelbuilder/goclaw/internal/http"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	videopkg "github.com/nextlevelbuilder/goclaw/internal/video"
)

// newVideoStack builds the shared video pipeline components. Returns nil when
// the video surface is disabled by config or when required stores are missing.
// Pattern: newCloudStack in cmd/gateway_cloud.go.
func newVideoStack(cfg *config.Config, stores *store.Stores, workspace string, eventPub bus.EventPublisher) *videopkg.VideoStack {
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
		slog.Warn("video: worker_url not configured, video pipeline disabled")
		return nil
	}

	worker := videopkg.NewWorkerClient(workerURL, cfg.Video.WorkerToken, 10, 5)
	disp := videopkg.NewDispatcher(&cfg.Video, stores.VideoJobs, worker, eventPub, workspace)

	return &videopkg.VideoStack{
		VideoJobs:  stores.VideoJobs,
		Worker:     worker,
		Dispatcher: disp,
	}
}

// wireVideo registers the video HTTP handler on the gateway server and
// registers the render_video agent tool. Returns a cleanup func that
// stops the dispatcher (safe to defer).
// Pattern: wireCloudTools in cmd/gateway_cloud.go + wireCloud.
func wireVideo(
	stack *videopkg.VideoStack,
	server *gateway.Server,
	toolsReg *tools.Registry,
) func() {
	if stack == nil {
		return func() {}
	}

	// Register the render_video agent tool.
	renderTool := tools.NewRenderVideoTool(stack.VideoJobs, stack.Dispatcher, nil)
	toolsReg.Register(renderTool)
	slog.Info("video: render_video tool registered")

	// Wire the HTTP handler for /v1/video/* endpoints.
	// The handler is registered unconditionally so the API surface is
	// discoverable; it returns 403 when disabled.
	videoHandler := httpapi.NewVideoHandler(stack.VideoJobs, stack.Worker, true)
	server.SetVideoHandler(videoHandler)

	// Start the dispatcher goroutine.
	stack.Dispatcher.Start()

	return func() {
		stack.Dispatcher.Stop()
	}
}
