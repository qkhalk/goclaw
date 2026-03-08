package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// --- Context helpers for media audio ---

const ctxMediaAudioRefs toolContextKey = "tool_media_audio_refs"

// WithMediaAudioRefs stores audio MediaRefs in context for read_audio tool access.
func WithMediaAudioRefs(ctx context.Context, refs []providers.MediaRef) context.Context {
	return context.WithValue(ctx, ctxMediaAudioRefs, refs)
}

// MediaAudioRefsFromCtx retrieves stored audio MediaRefs from context.
func MediaAudioRefsFromCtx(ctx context.Context) []providers.MediaRef {
	v, _ := ctx.Value(ctxMediaAudioRefs).([]providers.MediaRef)
	return v
}

// --- ReadAudioTool ---

// audioMaxBytes is the max file size for audio analysis (50MB).
const audioMaxBytes = 50 * 1024 * 1024

// ReadAudioTool uses an audio-capable provider to analyze audio files
// attached to the current conversation. Follows same pattern as ReadDocumentTool.
type ReadAudioTool struct {
	registry    *providers.Registry
	mediaLoader MediaPathLoader
}

func NewReadAudioTool(registry *providers.Registry, mediaLoader MediaPathLoader) *ReadAudioTool {
	return &ReadAudioTool{registry: registry, mediaLoader: mediaLoader}
}

func (t *ReadAudioTool) Name() string { return "read_audio" }

func (t *ReadAudioTool) Description() string {
	return "Analyze audio files (speech, music, sounds) attached to the conversation. " +
		"Use when you see <media:audio> tags and need to transcribe, summarize, or analyze audio content. " +
		"Specify what you want to extract or analyze."
}

func (t *ReadAudioTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "What to analyze. E.g. 'Transcribe this audio', 'Summarize the conversation', 'What language is spoken?'",
			},
			"media_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional: specific media_id from <media:audio> tag. If omitted, uses most recent audio.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *ReadAudioTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		prompt = "Analyze this audio and describe its contents."
	}
	mediaID, _ := args["media_id"].(string)

	// Resolve audio file path from MediaRefs in context.
	audioPath, audioMime, err := t.resolveAudioFile(ctx, mediaID)
	if err != nil {
		return ErrorResult(err.Error())
	}

	slog.Info("read_audio: resolved file", "path", audioPath, "mime", audioMime, "media_id", mediaID)

	// Read audio file.
	data, err := os.ReadFile(audioPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to read audio file: %v", err))
	}
	slog.Info("read_audio: file loaded", "size_bytes", len(data))
	if len(data) > audioMaxBytes {
		return ErrorResult(fmt.Sprintf("Audio too large: %d bytes (max %d)", len(data), audioMaxBytes))
	}

	// Find an audio-capable provider.
	provider, model, err := t.resolveAudioProviderWithConfig(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Try primary provider, fallback to next available on error.
	resp, usedProvider, usedModel := t.callAudioProvider(ctx, provider, model, prompt, data, audioMime)
	if resp == nil {
		// Primary failed — try fallback providers from priority list.
		slog.Warn("read_audio: primary provider failed, trying fallback", "primary", provider.Name())
		for _, fbName := range audioProviderPriority {
			if fbName == provider.Name() {
				continue
			}
			fbProvider, fbModel, fbErr := t.resolveAudioProviderByName(fbName)
			if fbErr != nil {
				continue
			}
			resp, usedProvider, usedModel = t.callAudioProvider(ctx, fbProvider, fbModel, prompt, data, audioMime)
			if resp != nil {
				slog.Info("read_audio: fallback succeeded", "provider", usedProvider)
				break
			}
		}
	}
	if resp == nil {
		return ErrorResult("Audio analysis failed: all providers returned errors")
	}

	result := NewResult(resp.Content)
	result.Usage = resp.Usage
	result.Provider = usedProvider
	result.Model = usedModel
	return result
}

// mimeFromAudioExt returns MIME type for audio file extensions.
func mimeFromAudioExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".aiff", ".aif":
		return "audio/aiff"
	case ".wma":
		return "audio/x-ms-wma"
	case ".opus":
		return "audio/opus"
	default:
		return "audio/mpeg"
	}
}
