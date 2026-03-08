package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// --- Context helpers for media video ---

const ctxMediaVideoRefs toolContextKey = "tool_media_video_refs"

// WithMediaVideoRefs stores video MediaRefs in context for read_video tool access.
func WithMediaVideoRefs(ctx context.Context, refs []providers.MediaRef) context.Context {
	return context.WithValue(ctx, ctxMediaVideoRefs, refs)
}

// MediaVideoRefsFromCtx retrieves stored video MediaRefs from context.
func MediaVideoRefsFromCtx(ctx context.Context) []providers.MediaRef {
	v, _ := ctx.Value(ctxMediaVideoRefs).([]providers.MediaRef)
	return v
}

// --- ReadVideoTool ---

// videoMaxBytes is the max file size for video analysis (100MB).
const videoMaxBytes = 100 * 1024 * 1024

// ReadVideoTool uses a video-capable provider to analyze video files
// attached to the current conversation. Follows same pattern as ReadAudioTool.
type ReadVideoTool struct {
	registry    *providers.Registry
	mediaLoader MediaPathLoader
}

func NewReadVideoTool(registry *providers.Registry, mediaLoader MediaPathLoader) *ReadVideoTool {
	return &ReadVideoTool{registry: registry, mediaLoader: mediaLoader}
}

func (t *ReadVideoTool) Name() string { return "read_video" }

func (t *ReadVideoTool) Description() string {
	return "Analyze video files attached to the conversation. " +
		"Use when you see <media:video> tags and need to describe, summarize, or analyze video content. " +
		"Specify what you want to extract or analyze."
}

func (t *ReadVideoTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "What to analyze. E.g. 'Describe what happens in this video', 'Summarize the key scenes', 'What text appears on screen?'",
			},
			"media_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional: specific media_id from <media:video> tag. If omitted, uses most recent video.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *ReadVideoTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		prompt = "Analyze this video and describe its contents."
	}
	mediaID, _ := args["media_id"].(string)

	videoPath, videoMime, err := t.resolveVideoFile(ctx, mediaID)
	if err != nil {
		return ErrorResult(err.Error())
	}

	slog.Info("read_video: resolved file", "path", videoPath, "mime", videoMime, "media_id", mediaID)

	data, err := os.ReadFile(videoPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to read video file: %v", err))
	}
	slog.Info("read_video: file loaded", "size_bytes", len(data))
	if len(data) > videoMaxBytes {
		return ErrorResult(fmt.Sprintf("Video too large: %d bytes (max %d)", len(data), videoMaxBytes))
	}

	provider, model, err := t.resolveVideoProviderWithConfig(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	resp, usedProvider, usedModel := t.callVideoProvider(ctx, provider, model, prompt, data, videoMime)
	if resp == nil {
		slog.Warn("read_video: primary provider failed, trying fallback", "primary", provider.Name())
		for _, fbName := range videoProviderPriority {
			if fbName == provider.Name() {
				continue
			}
			fbProvider, fbModel, fbErr := t.resolveVideoProviderByName(fbName)
			if fbErr != nil {
				continue
			}
			resp, usedProvider, usedModel = t.callVideoProvider(ctx, fbProvider, fbModel, prompt, data, videoMime)
			if resp != nil {
				slog.Info("read_video: fallback succeeded", "provider", usedProvider)
				break
			}
		}
	}
	if resp == nil {
		return ErrorResult("Video analysis failed: all providers returned errors")
	}

	result := NewResult(resp.Content)
	result.Usage = resp.Usage
	result.Provider = usedProvider
	result.Model = usedModel
	return result
}

// mimeFromVideoExt returns MIME type for video file extensions.
func mimeFromVideoExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	case ".3gp":
		return "video/3gpp"
	case ".mpeg", ".mpg":
		return "video/mpeg"
	default:
		return "video/mp4"
	}
}
