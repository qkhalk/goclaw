package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// videoProviderPriority is the order in which providers are tried for video analysis.
// OpenAI excluded — no native video upload in chat completions.
var videoProviderPriority = []string{"gemini", "openrouter"}

// videoModelOverrides maps provider names to preferred video-capable models.
var videoModelOverrides = map[string]string{
	"gemini":     "gemini-2.5-flash",
	"openrouter": "google/gemini-2.5-flash",
}

// resolveVideoFile finds the video file path from context MediaRefs.
func (t *ReadVideoTool) resolveVideoFile(ctx context.Context, mediaID string) (path, mime string, err error) {
	if t.mediaLoader == nil {
		return "", "", fmt.Errorf("no media storage configured — cannot access video files")
	}

	refs := MediaVideoRefsFromCtx(ctx)
	if len(refs) == 0 {
		return "", "", fmt.Errorf("no video files available in this conversation. The user may not have sent a video file.")
	}

	var ref *providers.MediaRef
	if mediaID != "" {
		for i := range refs {
			if refs[i].ID == mediaID {
				ref = &refs[i]
				break
			}
		}
		if ref == nil {
			return "", "", fmt.Errorf("video with media_id %q not found in conversation", mediaID)
		}
	} else {
		ref = &refs[len(refs)-1]
	}

	p, err := t.mediaLoader.LoadPath(ref.ID)
	if err != nil {
		return "", "", fmt.Errorf("video file not found: %v", err)
	}

	mime = ref.MimeType
	if mime == "" || mime == "application/octet-stream" {
		mime = mimeFromVideoExt(filepath.Ext(p))
	}

	return p, mime, nil
}

// resolveVideoProviderWithConfig checks builtin settings, then hardcoded priority.
func (t *ReadVideoTool) resolveVideoProviderWithConfig(ctx context.Context) (providers.Provider, string, error) {
	if p, model, ok := t.resolveFromVideoSettings(ctx); ok {
		return p, model, nil
	}
	return t.resolveVideoProvider()
}

// resolveFromVideoSettings checks global builtin tool settings for provider/model config.
func (t *ReadVideoTool) resolveFromVideoSettings(ctx context.Context) (providers.Provider, string, bool) {
	settings := BuiltinToolSettingsFromCtx(ctx)
	if settings == nil {
		return nil, "", false
	}
	raw, ok := settings["read_video"]
	if !ok || len(raw) == 0 {
		return nil, "", false
	}
	var cfg struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.Provider == "" {
		return nil, "", false
	}
	p, err := t.registry.Get(cfg.Provider)
	if err != nil {
		return nil, "", false
	}
	model := cfg.Model
	if model == "" {
		model = p.DefaultModel()
	}
	return p, model, true
}

// resolveVideoProvider finds the first available video-capable provider.
func (t *ReadVideoTool) resolveVideoProvider() (providers.Provider, string, error) {
	for _, name := range videoProviderPriority {
		p, err := t.registry.Get(name)
		if err != nil {
			continue
		}
		model := p.DefaultModel()
		if override, ok := videoModelOverrides[name]; ok {
			model = override
		}
		return p, model, nil
	}
	return nil, "", fmt.Errorf("no video-capable provider available (need one of: %v)", videoProviderPriority)
}

// resolveVideoProviderByName gets a specific provider by name and applies model override.
func (t *ReadVideoTool) resolveVideoProviderByName(name string) (providers.Provider, string, error) {
	p, err := t.registry.Get(name)
	if err != nil {
		return nil, "", err
	}
	model := p.DefaultModel()
	if override, ok := videoModelOverrides[name]; ok {
		model = override
	}
	return p, model, nil
}

// callVideoProvider sends video to a provider for analysis.
// Gemini: uses File API (upload → poll → file_data in generateContent).
// Others: falls back to base64 in image_url (OpenRouter routes to Gemini which handles video).
func (t *ReadVideoTool) callVideoProvider(ctx context.Context, provider providers.Provider, model, prompt string, data []byte, mime string) (*providers.ChatResponse, string, string) {
	provName := provider.Name()

	// Gemini: use File API.
	if strings.HasPrefix(provName, "gemini") {
		oaiProv, ok := provider.(*providers.OpenAIProvider)
		if !ok {
			slog.Warn("read_video: gemini provider is not OpenAIProvider", "provider", provName)
			return nil, "", ""
		}
		apiKey := oaiProv.APIKey()
		slog.Info("read_video: using gemini file API", "provider", provName, "model", model, "size", len(data), "mime", mime)
		resp, err := geminiFileAPICall(ctx, apiKey, model, prompt, data, mime, 180*time.Second)
		if err != nil {
			slog.Warn("read_video: gemini file API call failed", "error", err)
			return nil, "", ""
		}
		return resp, provName, model
	}

	// Other providers: try standard Chat API with base64 as image_url (best effort).
	slog.Info("read_video: using chat API fallback", "provider", provName, "model", model, "size", len(data))
	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{
				Role:    "user",
				Content: prompt,
				Images:  []providers.ImageContent{{MimeType: mime, Data: base64.StdEncoding.EncodeToString(data)}},
			},
		},
		Model: model,
		Options: map[string]interface{}{
			"max_tokens":  16384,
			"temperature": 0.2,
		},
	})
	if err != nil {
		slog.Warn("read_video: chat API fallback failed", "provider", provName, "error", err)
		return nil, "", ""
	}
	return resp, provName, model
}
