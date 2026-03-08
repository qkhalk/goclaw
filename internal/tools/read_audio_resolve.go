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

// audioProviderPriority is the order in which providers are tried for audio analysis.
var audioProviderPriority = []string{"gemini", "openai", "openrouter"}

// audioModelOverrides maps provider names to preferred audio-capable models.
var audioModelOverrides = map[string]string{
	"gemini":     "gemini-2.5-flash",
	"openai":     "gpt-4o-audio-preview",
	"openrouter": "google/gemini-2.5-flash",
}

// resolveAudioFile finds the audio file path from context MediaRefs.
func (t *ReadAudioTool) resolveAudioFile(ctx context.Context, mediaID string) (path, mime string, err error) {
	if t.mediaLoader == nil {
		return "", "", fmt.Errorf("no media storage configured — cannot access audio files")
	}

	refs := MediaAudioRefsFromCtx(ctx)
	if len(refs) == 0 {
		return "", "", fmt.Errorf("no audio files available in this conversation. The user may not have sent an audio file.")
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
			return "", "", fmt.Errorf("audio with media_id %q not found in conversation", mediaID)
		}
	} else {
		ref = &refs[len(refs)-1]
	}

	p, err := t.mediaLoader.LoadPath(ref.ID)
	if err != nil {
		return "", "", fmt.Errorf("audio file not found: %v", err)
	}

	mime = ref.MimeType
	if mime == "" || mime == "application/octet-stream" {
		mime = mimeFromAudioExt(filepath.Ext(p))
	}

	return p, mime, nil
}

// resolveAudioProviderWithConfig checks builtin settings, then hardcoded priority.
func (t *ReadAudioTool) resolveAudioProviderWithConfig(ctx context.Context) (providers.Provider, string, error) {
	if p, model, ok := t.resolveFromAudioSettings(ctx); ok {
		return p, model, nil
	}
	return t.resolveAudioProvider()
}

// resolveFromAudioSettings checks global builtin tool settings for provider/model config.
func (t *ReadAudioTool) resolveFromAudioSettings(ctx context.Context) (providers.Provider, string, bool) {
	settings := BuiltinToolSettingsFromCtx(ctx)
	if settings == nil {
		return nil, "", false
	}
	raw, ok := settings["read_audio"]
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

// resolveAudioProvider finds the first available audio-capable provider.
func (t *ReadAudioTool) resolveAudioProvider() (providers.Provider, string, error) {
	for _, name := range audioProviderPriority {
		p, err := t.registry.Get(name)
		if err != nil {
			continue
		}
		model := p.DefaultModel()
		if override, ok := audioModelOverrides[name]; ok {
			model = override
		}
		return p, model, nil
	}
	return nil, "", fmt.Errorf("no audio-capable provider available (need one of: %v)", audioProviderPriority)
}

// resolveAudioProviderByName gets a specific provider by name and applies model override.
func (t *ReadAudioTool) resolveAudioProviderByName(name string) (providers.Provider, string, error) {
	p, err := t.registry.Get(name)
	if err != nil {
		return nil, "", err
	}
	model := p.DefaultModel()
	if override, ok := audioModelOverrides[name]; ok {
		model = override
	}
	return p, model, nil
}

// callAudioProvider sends audio to a provider for analysis.
// Gemini: uses File API (upload → poll → file_data in generateContent).
// OpenAI: uses input_audio content part in chat completions.
// Others: falls back to base64 in image_url (may not work for all).
func (t *ReadAudioTool) callAudioProvider(ctx context.Context, provider providers.Provider, model, prompt string, data []byte, mime string) (*providers.ChatResponse, string, string) {
	provName := provider.Name()

	// Gemini: use File API (inlineData doesn't work for audio).
	if strings.HasPrefix(provName, "gemini") {
		oaiProv, ok := provider.(*providers.OpenAIProvider)
		if !ok {
			slog.Warn("read_audio: gemini provider is not OpenAIProvider", "provider", provName)
			return nil, "", ""
		}
		apiKey := oaiProv.APIKey()
		slog.Info("read_audio: using gemini file API", "provider", provName, "model", model, "size", len(data), "mime", mime)
		resp, err := geminiFileAPICall(ctx, apiKey, model, prompt, data, mime, 120*time.Second)
		if err != nil {
			slog.Warn("read_audio: gemini file API call failed", "error", err)
			return nil, "", ""
		}
		return resp, provName, model
	}

	// OpenAI: use input_audio content part (supports wav, mp3).
	if strings.HasPrefix(provName, "openai") {
		oaiProv, ok := provider.(*providers.OpenAIProvider)
		if !ok {
			slog.Warn("read_audio: openai provider is not OpenAIProvider", "provider", provName)
			return nil, "", ""
		}
		slog.Info("read_audio: using openai input_audio API", "provider", provName, "model", model, "size", len(data), "mime", mime)
		resp, err := openaiAudioCall(ctx, oaiProv.APIKey(), oaiProv.APIBase(), model, prompt, data, mime)
		if err != nil {
			slog.Warn("read_audio: openai audio call failed", "error", err)
			return nil, "", ""
		}
		return resp, provName, model
	}

	// Other providers: try standard Chat API with base64 audio as image_url (best effort).
	slog.Info("read_audio: using chat API fallback", "provider", provName, "model", model, "size", len(data))
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
		slog.Warn("read_audio: chat API fallback failed", "provider", provName, "error", err)
		return nil, "", ""
	}
	return resp, provName, model
}

// openaiAudioCall sends audio to OpenAI using the input_audio content part.
func openaiAudioCall(ctx context.Context, apiKey, baseURL, model, prompt string, data []byte, mime string) (*providers.ChatResponse, error) {
	// Determine format from MIME (OpenAI supports: wav, mp3).
	format := "mp3"
	switch {
	case strings.Contains(mime, "wav"):
		format = "wav"
	case strings.Contains(mime, "mp3"), strings.Contains(mime, "mpeg"):
		format = "mp3"
	}

	b64 := base64.StdEncoding.EncodeToString(data)

	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": prompt},
					{"type": "input_audio", "input_audio": map[string]string{
						"data":   b64,
						"format": format,
					}},
				},
			},
		},
		"max_tokens": 16384,
	}

	return callOpenAICompatJSON(ctx, apiKey, baseURL, body, 120*time.Second)
}
