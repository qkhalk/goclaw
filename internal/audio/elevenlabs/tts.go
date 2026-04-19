package elevenlabs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
)

// TTSProvider synthesizes text via POST /v1/text-to-speech/{voice_id}.
// Legacy name: internal/tts.ElevenLabsProvider.
type TTSProvider struct {
	cfg Config
	c   *client
}

// NewTTSProvider returns an ElevenLabs TTS provider backed by the shared client.
func NewTTSProvider(cfg Config) *TTSProvider {
	cfg = cfg.withDefaults()
	return &TTSProvider{
		cfg: cfg,
		c:   newClient(cfg.APIKey, cfg.BaseURL, cfg.TimeoutMs),
	}
}

// Name returns the stable provider identifier used by the Manager.
func (p *TTSProvider) Name() string { return "elevenlabs" }

// Synthesize converts text to audio. Opts.Voice/Opts.Model override the
// configured defaults; Opts.Format="opus" switches to Ogg Opus output.
// opts.Params keys (nested dot-path):
//   - "voice_settings.stability"        float64  (default 0.5)
//   - "voice_settings.similarity_boost" float64  (default 0.75)
//   - "voice_settings.style"            float64  (default 0.0)
//   - "voice_settings.use_speaker_boost" bool    (default true)
//   - "voice_settings.speed"            float64  (default 1.0)
//   - "apply_text_normalization"        string   (default "auto")
//   - "seed"                            int      (default 0 = omitted)
//   - "optimize_streaming_latency"      int      (query param, default 0 = omitted)
//   - "language_code"                   string   (query param, default "" = omitted)
//
// MUST NOT mutate opts.Params — reads only.
func (p *TTSProvider) Synthesize(ctx context.Context, text string, opts audio.TTSOptions) (*audio.SynthResult, error) {
	voiceID := opts.Voice
	if voiceID == "" {
		voiceID = p.cfg.VoiceID
	}
	modelID := opts.Model
	if modelID == "" {
		modelID = p.cfg.ModelID
	}
	if err := ValidateModel(modelID); err != nil {
		return nil, err
	}

	// Output format: MP3 128kbps default, Opus 64kbps for Telegram voice.
	outputFormat, ext, mime := "mp3_44100_128", "mp3", "audio/mpeg"
	if opts.Format == "opus" {
		outputFormat, ext, mime = "opus_48000_64", "ogg", "audio/ogg"
	}

	// Override output format from params if provided (27 codecs supported).
	if pf := resolveELString(opts.Params, "output_format", ""); pf != "" {
		outputFormat = pf
	}

	// Build voice_settings from params, falling back to hardcoded defaults.
	// Defaults MUST match characterization fixture (stability=0.5, etc.).
	voiceSettings := map[string]any{
		"stability":         resolveELFloat(opts.Params, "voice_settings.stability", 0.5),
		"similarity_boost":  resolveELFloat(opts.Params, "voice_settings.similarity_boost", 0.75),
		"style":             resolveELFloat(opts.Params, "voice_settings.style", 0.0),
		"use_speaker_boost": resolveELBool(opts.Params, "voice_settings.use_speaker_boost", true),
		"speed":             resolveELFloat(opts.Params, "voice_settings.speed", 1.0),
	}

	body := map[string]any{
		"text":           text,
		"model_id":       modelID,
		"voice_settings": voiceSettings,
	}

	// Optional top-level params.
	if norm := resolveELString(opts.Params, "apply_text_normalization", ""); norm != "" {
		body["apply_text_normalization"] = norm
	}
	if seed := resolveELInt(opts.Params, "seed", -1); seed > 0 {
		body["seed"] = seed
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal elevenlabs tts request: %w", err)
	}

	// Build path with query params.
	path := fmt.Sprintf("/v1/text-to-speech/%s?output_format=%s", voiceID, outputFormat)
	if latency := resolveELInt(opts.Params, "optimize_streaming_latency", -1); latency > 0 {
		path += fmt.Sprintf("&optimize_streaming_latency=%d", latency)
	}
	if lang := resolveELString(opts.Params, "language_code", ""); lang != "" {
		path += "&language_code=" + lang
	}

	audioBytes, err := p.c.postJSON(ctx, path, bodyJSON, 0)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: %w", err)
	}

	return &audio.SynthResult{
		Audio:     audioBytes,
		Extension: ext,
		MimeType:  mime,
	}, nil
}

// resolveELFloat reads a float param from params map via nested key, falling back to def.
func resolveELFloat(params map[string]any, key string, def float64) float64 {
	if params == nil {
		return def
	}
	v, ok := audio.GetNested(params, key)
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	}
	return def
}

// resolveELBool reads a bool param from params map via nested key, falling back to def.
func resolveELBool(params map[string]any, key string, def bool) bool {
	if params == nil {
		return def
	}
	v, ok := audio.GetNested(params, key)
	if !ok {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

// resolveELString reads a string param from params map via nested key, falling back to def.
func resolveELString(params map[string]any, key, def string) string {
	if params == nil {
		return def
	}
	v, ok := audio.GetNested(params, key)
	if !ok {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

// resolveELInt reads an int param from params map via nested key.
// Returns def (-1 by convention for "not set") when absent.
func resolveELInt(params map[string]any, key string, def int) int {
	if params == nil {
		return def
	}
	v, ok := audio.GetNested(params, key)
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}
