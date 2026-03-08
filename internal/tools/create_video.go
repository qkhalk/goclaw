package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// videoGenProviderPriority is the default order for video generation providers.
var videoGenProviderPriority = []string{"gemini", "openrouter"}

// videoGenModelDefaults maps provider names to default video generation models.
var videoGenModelDefaults = map[string]string{
	"gemini":     "veo-3.0-generate-preview",
	"openrouter": "google/veo-3.0-generate-preview",
}

// CreateVideoTool generates videos using a video generation API.
// Uses Gemini Veo via native generateContent API with VIDEO response modality.
type CreateVideoTool struct {
	registry *providers.Registry
}

func NewCreateVideoTool(registry *providers.Registry) *CreateVideoTool {
	return &CreateVideoTool{registry: registry}
}

func (t *CreateVideoTool) Name() string { return "create_video" }

func (t *CreateVideoTool) Description() string {
	return "Generate a video from a text description using a video generation model. Returns a MEDIA: path to the generated video file."
}

func (t *CreateVideoTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Text description of the video to generate.",
			},
			"duration": map[string]interface{}{
				"type":        "integer",
				"description": "Video duration in seconds (default 5, max 30).",
			},
			"aspect_ratio": map[string]interface{}{
				"type":        "string",
				"description": "Aspect ratio: '16:9' (default), '9:16', '1:1'.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *CreateVideoTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return ErrorResult("prompt is required")
	}

	// Parse and enforce duration (default 5, max 30).
	duration := 5
	if d, ok := args["duration"].(float64); ok {
		duration = int(d)
	}
	if duration < 1 {
		duration = 1
	}
	if duration > 30 {
		duration = 30
	}

	// Parse and validate aspect ratio.
	aspectRatio := "16:9"
	if ar, _ := args["aspect_ratio"].(string); ar != "" {
		switch ar {
		case "16:9", "9:16", "1:1":
			aspectRatio = ar
		default:
			return ErrorResult(fmt.Sprintf("unsupported aspect_ratio %q; use '16:9', '9:16', or '1:1'", ar))
		}
	}

	providerName, model := t.resolveConfig(ctx)

	p, err := t.registry.Get(providerName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("video generation provider %q not available", providerName))
	}

	cp, ok := p.(credentialProvider)
	if !ok {
		return ErrorResult(fmt.Sprintf("provider %q does not expose API credentials for video generation", providerName))
	}

	slog.Info("create_video: calling video generation API", "provider", providerName, "model", model, "duration", duration, "aspect_ratio", aspectRatio)

	var videoBytes []byte
	var usage *providers.Usage
	switch providerName {
	case "gemini":
		videoBytes, usage, err = t.callGeminiVideoGen(ctx, cp.APIKey(), cp.APIBase(), model, prompt, duration, aspectRatio)
	default:
		// OpenRouter and others: try chat completions with VIDEO modality
		videoBytes, usage, err = t.callChatVideoGen(ctx, cp.APIKey(), cp.APIBase(), model, prompt, duration, aspectRatio)
	}
	if err != nil {
		return ErrorResult(fmt.Sprintf("video generation failed: %v", err))
	}

	// Save to workspace under date-based folder.
	workspace := ToolWorkspaceFromCtx(ctx)
	if workspace == "" {
		workspace = os.TempDir()
	}
	dateDir := filepath.Join(workspace, "generated", time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create output directory: %v", err))
	}
	videoPath := filepath.Join(dateDir, fmt.Sprintf("goclaw_gen_%d.mp4", time.Now().UnixNano()))
	if err := os.WriteFile(videoPath, videoBytes, 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to save generated video: %v", err))
	}

	result := &Result{ForLLM: fmt.Sprintf("MEDIA:%s", videoPath)}
	result.Media = []bus.MediaFile{{Path: videoPath, MimeType: "video/mp4"}}
	result.Deliverable = fmt.Sprintf("[Generated video: %s]\nPrompt: %s", filepath.Base(videoPath), prompt)
	result.Provider = providerName
	result.Model = model
	if usage != nil {
		result.Usage = usage
	}
	return result
}

// resolveConfig returns the provider name and model for video generation.
func (t *CreateVideoTool) resolveConfig(ctx context.Context) (providerName, model string) {
	// 1. Check global builtin_tools.settings
	if settings := BuiltinToolSettingsFromCtx(ctx); settings != nil {
		if raw, ok := settings["create_video"]; ok && len(raw) > 0 {
			var cfg struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			}
			if json.Unmarshal(raw, &cfg) == nil && cfg.Provider != "" {
				if _, err := t.registry.Get(cfg.Provider); err == nil {
					providerName = cfg.Provider
					model = cfg.Model
				}
			}
		}
	}

	// 2. Find first available from priority list
	if providerName == "" {
		for _, name := range videoGenProviderPriority {
			if _, err := t.registry.Get(name); err == nil {
				providerName = name
				break
			}
		}
	}
	if providerName == "" {
		providerName = "gemini"
	}

	// 3. Default model
	if model == "" {
		if m, ok := videoGenModelDefaults[providerName]; ok {
			model = m
		}
	}

	return providerName, model
}

// callGeminiVideoGen uses the native Gemini generateContent API with VIDEO response modality.
func (t *CreateVideoTool) callGeminiVideoGen(ctx context.Context, apiKey, apiBase, model, prompt string, duration int, aspectRatio string) ([]byte, *providers.Usage, error) {
	nativeBase := strings.TrimRight(apiBase, "/")
	nativeBase = strings.TrimSuffix(nativeBase, "/openai")

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", nativeBase, model, apiKey)

	genConfig := map[string]interface{}{
		"responseModalities": []string{"VIDEO"},
		"videoDuration":      duration,
		"aspectRatio":        aspectRatio,
	}

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]interface{}{{"text": prompt}}},
		},
		"generationConfig": genConfig,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Video generation can take a while.
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	// Parse Gemini response — look for inlineData with video MIME.
	var gemResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		return nil, nil, fmt.Errorf("parse response: %w", err)
	}

	for _, cand := range gemResp.Candidates {
		for _, part := range cand.Content.Parts {
			if part.InlineData != nil && strings.HasPrefix(part.InlineData.MimeType, "video/") {
				videoBytes, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
				if err != nil {
					return nil, nil, fmt.Errorf("decode base64: %w", err)
				}
				var usage *providers.Usage
				if gemResp.UsageMetadata != nil {
					usage = &providers.Usage{
						PromptTokens:     gemResp.UsageMetadata.PromptTokenCount,
						CompletionTokens: gemResp.UsageMetadata.CandidatesTokenCount,
						TotalTokens:      gemResp.UsageMetadata.TotalTokenCount,
					}
				}
				return videoBytes, usage, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("no video data in Gemini response")
}

// callChatVideoGen tries OpenAI-compatible chat completions with video modality.
func (t *CreateVideoTool) callChatVideoGen(ctx context.Context, apiKey, apiBase, model, prompt string, duration int, aspectRatio string) ([]byte, *providers.Usage, error) {
	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
		"modalities":   []string{"video", "text"},
		"duration":     duration,
		"aspect_ratio": aspectRatio,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(apiBase, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	// Try to extract video from multipart content or data URL.
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, nil, fmt.Errorf("parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, nil, fmt.Errorf("no choices in response")
	}

	// Look for video data URL in multipart content.
	if parts, ok := chatResp.Choices[0].Message.Content.([]interface{}); ok {
		for _, part := range parts {
			if m, ok := part.(map[string]interface{}); ok {
				if m["type"] == "video_url" || m["type"] == "image_url" {
					if vidURL, ok := m["video_url"].(map[string]interface{}); ok {
						if urlStr, ok := vidURL["url"].(string); ok {
							if videoBytes, err := decodeDataURL(urlStr); err == nil {
								return videoBytes, convertUsage(chatResp.Usage), nil
							}
						}
					}
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("no video data found in response")
}
