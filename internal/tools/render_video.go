package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	videopkg "github.com/nextlevelbuilder/goclaw/internal/video"
)

// RenderVideoTool renders a storyboard JSON into an MP4 video via the
// videoworker sidecar. It creates a job, submits to the worker, and
// returns immediately with the job ID (async completion via callback).
type RenderVideoTool struct {
	videoJobs store.VideoRenderJobStore
	dispatcher *videopkg.Dispatcher
	msgBus     *bus.MessageBus
}

// NewRenderVideoTool creates a RenderVideoTool.
func NewRenderVideoTool(
	videoJobs store.VideoRenderJobStore,
	dispatcher *videopkg.Dispatcher,
	msgBus *bus.MessageBus,
) *RenderVideoTool {
	return &RenderVideoTool{
		videoJobs:  videoJobs,
		dispatcher: dispatcher,
		msgBus:     msgBus,
	}
}

func (t *RenderVideoTool) Name() string { return "render_video" }

func (t *RenderVideoTool) Description() string {
	return "Render a storyboard JSON into an MP4 video using the local video pipeline. " +
		"Accepts a storyboard object with scenes (image/video/color), canvas settings, and audio mix. " +
		"Returns a job ID immediately; the video will be delivered asynchronously when rendering completes."
}

func (t *RenderVideoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"storyboard": map[string]any{
				"type":        "object",
				"description": "Storyboard JSON object with version, canvas, scenes, and optional audio/output config. See the storyboard contract for fields.",
			},
			"storyboard_json": map[string]any{
				"type":        "string",
				"description": "Alternative: storyboard as a raw JSON string (if the object form is inconvenient).",
			},
		},
		"required": []string{"storyboard"},
	}
}

// Execute validates the storyboard, creates a render job in the store,
// notifies the dispatcher, and returns the job ID immediately.
func (t *RenderVideoTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.videoJobs == nil {
		return ErrorResult("video rendering is not available (store not configured)")
	}
	if t.dispatcher == nil {
		return ErrorResult("video rendering is not available (dispatcher not running)")
	}

	// Parse storyboard from object or JSON string.
	sbJSON, ok := t.extractStoryboard(args)
	if !ok {
		return ErrorResult("provide storyboard as an object or storyboard_json as a string")
	}

	// Validate the storyboard.
	var sb videopkg.Storyboard
	if err := json.Unmarshal([]byte(sbJSON), &sb); err != nil {
		return ErrorResult(fmt.Sprintf("invalid storyboard JSON: %v", err))
	}
	if err := sb.Validate(); err != nil {
		return ErrorResult(fmt.Sprintf("storyboard validation failed: %v", err))
	}

	// Check path traversal on scene sources.
	workspace := ToolWorkspaceFromCtx(ctx)
	for i, scene := range sb.Scenes {
		if scene.Source != "" && !strings.HasPrefix(scene.Source, "http://") && !strings.HasPrefix(scene.Source, "https://") {
			if err := t.validateAssetPath(workspace, scene.Source); err != nil {
				return ErrorResult(fmt.Sprintf("scene %d: invalid source path: %v", i, err))
			}
		}
	}

	// Build tenant context for store operations.
	tenantID := store.TenantIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)
	agentKey := ToolAgentKeyFromCtx(ctx)
	sessionKey := ToolSessionKeyFromCtx(ctx)

	jobID := uuid.New().String()
	now := time.Now()
	job := &store.VideoRenderJob{
		ID:             jobID,
		TenantID:       tenantID.String(),
		UserID:         userID,
		AgentID:        agentKey,
		SessionKey:     sessionKey,
		Status:         string(videopkg.JobQueued),
		Engine:         "ffmpeg",
		StoryboardJSON: sbJSON,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := t.videoJobs.Create(ctx, job); err != nil {
		slog.Error("render_video: failed to create job", "error", err)
		return ErrorResult(fmt.Sprintf("failed to create render job: %v", err))
	}

	// Notify the dispatcher to pick up the new job immediately.
	t.dispatcher.Notify()

	slog.Info("render_video: job created", "job_id", jobID, "scenes", len(sb.Scenes))

	result := NewResult(fmt.Sprintf(
		"Video render job created successfully.\nJob ID: %s\nStatus: queued\nScenes: %d\n"+
			"The video will be delivered asynchronously when rendering completes.",
		jobID, len(sb.Scenes),
	))
	result.Async = true
	return result
}

// extractStoryboard extracts the storyboard JSON from either the "storyboard"
// object or "storyboard_json" string argument.
func (t *RenderVideoTool) extractStoryboard(args map[string]any) (string, bool) {
	// Try object form first.
	if sb, ok := args["storyboard"]; ok {
		if sbMap, ok := sb.(map[string]any); ok {
			data, err := json.Marshal(sbMap)
			if err != nil {
				return "", false
			}
			return string(data), true
		}
		// Could already be a JSON string.
		if sbStr, ok := sb.(string); ok {
			return sbStr, true
		}
	}

	// Try string form.
	if sbStr, _ := args["storyboard_json"].(string); sbStr != "" {
		return sbStr, true
	}

	return "", false
}

// validateAssetPath checks that a scene source path doesn't escape the workspace.
func (t *RenderVideoTool) validateAssetPath(workspace, source string) error {
	if workspace == "" {
		return nil // no workspace restriction
	}
	// Resolve against workspace and ensure it stays within.
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return fmt.Errorf("cannot resolve workspace: %w", err)
	}
	absSource, err := filepath.Abs(filepath.Join(workspace, source))
	if err != nil {
		return fmt.Errorf("cannot resolve source path: %w", err)
	}
	rel, err := filepath.Rel(absWorkspace, absSource)
	if err != nil {
		return fmt.Errorf("path escapes workspace")
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path traversal detected: %s", source)
	}
	return nil
}

// Ensure compile-time interface compliance.
var _ Tool = (*RenderVideoTool)(nil)
