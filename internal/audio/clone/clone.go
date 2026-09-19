// Package clone implements TTS by proxying to an external self-hosted voice
// clone worker (see contrib/voiceclone — OpenVoice v2 tone conversion layered
// on edge-tts, CPU-friendly). The gateway holds no models and spends no CPU:
// it forwards text synthesis and reference-voice management to the worker
// endpoint, so a 512MB gateway stays unaffected by clone inference.
//
// The endpoint URL is operator-configured (config tts.clone.endpoint or tenant
// system_configs). Because the worker typically lives on the LAN, the
// test-connection path enforces the same private-network opt-in as other
// remote provider types (GOCLAW_ALLOW_PRIVATE_PROVIDER_URLS).
package clone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
)

// Config configures the voice clone proxy provider.
type Config struct {
	// Endpoint is the clone worker base URL, e.g. "http://192.168.1.50:18795".
	// Required to enable the provider.
	Endpoint string
	// APIKey is the optional bearer token shared with the worker.
	APIKey string
	// Voice is the default clone voice id used when a request has none.
	Voice string
	// TimeoutMs bounds a single synthesis call. CPU tone conversion is slow —
	// default 180s, well above the edge-only paths.
	TimeoutMs int
}

// Provider implements audio.TTSProvider plus voice-registry management by
// proxying to the clone worker.
type Provider struct {
	endpoint string
	apiKey   string
	voice    string
	client   *http.Client

	mu       sync.Mutex
	voices   []audio.Voice
	voicesAt time.Time
}

// NewProvider returns a clone proxy provider with defaults applied.
func NewProvider(cfg Config) *Provider {
	p := &Provider{
		endpoint: strings.TrimRight(cfg.Endpoint, "/"),
		apiKey:   cfg.APIKey,
		voice:    cfg.Voice,
	}
	if p.voice == "" {
		p.voice = "default"
	}
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	p.client = &http.Client{Timeout: timeout}
	return p
}

// Name returns the stable provider identifier used by the Manager.
func (p *Provider) Name() string { return "clone" }

// voicesTTL bounds how long ListVoices results are reused. Keeps the
// capabilities endpoint cheap while still reflecting recent registrations
// within half a minute.
const voicesTTL = 30 * time.Second

// synthesizeRequest is the JSON body for the worker's POST /v1/tts.
type synthesizeRequest struct {
	Text      string  `json:"text"`
	VoiceID   string  `json:"voice_id,omitempty"`
	BaseVoice string  `json:"base_voice,omitempty"`
	Speed     float64 `json:"speed,omitempty"`
}

// Synthesize forwards text to the worker and returns the WAV bytes it renders.
// opts.Voice overrides the configured clone voice; opts.Params accept
// "base_voice" (string — the edge-tts voice the worker reads with before tone
// conversion) and "speed" (number). MUST NOT mutate opts.Params.
func (p *Provider) Synthesize(ctx context.Context, text string, opts audio.TTSOptions) (*audio.SynthResult, error) {
	req := synthesizeRequest{Text: text, VoiceID: p.voice}
	if opts.Voice != "" {
		req.VoiceID = opts.Voice
	}
	for k, v := range opts.Params {
		switch k {
		case "base_voice":
			if s, ok := v.(string); ok && s != "" {
				req.BaseVoice = s
			}
		case "speed":
			if f, ok := v.(float64); ok && f > 0 {
				req.Speed = f
			}
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("clone: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/v1/tts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("clone: build request: %w", err)
	}
	p.setAuth(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("clone worker unreachable at %s: %w", p.endpoint, err)
	}
	defer resp.Body.Close()
	audioBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxWorkerAudioBytes))
	if err != nil {
		return nil, fmt.Errorf("clone: read audio: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("clone worker returned %d: %s", resp.StatusCode, truncate(bytes.TrimSpace(audioBytes), 300))
	}
	if len(audioBytes) == 0 {
		return nil, fmt.Errorf("clone worker returned empty audio")
	}

	mime := resp.Header.Get("Content-Type")
	if mime == "" || strings.HasPrefix(mime, "application/") {
		mime = "audio/wav"
	}
	ext := "wav"
	if strings.Contains(mime, "mpeg") {
		ext = "mp3"
	}
	return &audio.SynthResult{Audio: audioBytes, Extension: ext, MimeType: mime}, nil
}

// Capabilities returns the static catalog entry. Voices stay nil here —
// ListVoices is dynamic and handlers enrich it where needed.
func (p *Provider) Capabilities() audio.ProviderCapabilities {
	return audio.ProviderCapabilities{
		Provider:       "clone",
		DisplayName:    "Voice Clone (self-hosted)",
		RequiresAPIKey: false,
	}
}

// maxWorkerAudioBytes caps audio reads at 50MB — minutes of WAV headroom,
// while still stopping a misbehaving worker from exhausting gateway memory.
const maxWorkerAudioBytes = 50 << 20

// voiceEntry mirrors one record of the worker's GET /v1/voices response.
type voiceEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
}

// voicesResponse is the envelope for GET/POST /v1/voices on the worker.
type voicesResponse struct {
	Voices []voiceEntry `json:"voices"`
	Voice  *voiceEntry  `json:"voice,omitempty"`
}

// ListVoices returns the worker's registered clone voices, cached 30s.
func (p *Provider) ListVoices(ctx context.Context) ([]audio.Voice, error) {
	p.mu.Lock()
	if p.voices != nil && time.Since(p.voicesAt) < voicesTTL {
		out := p.voices
		p.mu.Unlock()
		return out, nil
	}
	p.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/v1/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("clone: build request: %w", err)
	}
	p.setAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clone worker unreachable at %s: %w", p.endpoint, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("clone: read voices: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("clone worker returned %d: %s", resp.StatusCode, truncate(bytes.TrimSpace(raw), 300))
	}
	var parsed voicesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("clone: decode voices: %w", err)
	}

	out := make([]audio.Voice, 0, len(parsed.Voices))
	for _, v := range parsed.Voices {
		out = append(out, audio.Voice{
			ID:       v.ID,
			Name:     v.Name,
			Labels:   map[string]string{"backend": "openvoice"},
			Category: "cloned",
		})
	}

	p.mu.Lock()
	p.voices = out
	p.voicesAt = time.Now()
	p.mu.Unlock()
	return out, nil
}

// RegisterVoice uploads reference audio to the worker, which extracts the
// speaker embedding. name becomes the human label; id is derived on the worker.
func (p *Provider) RegisterVoice(ctx context.Context, name string, audioBytes []byte, filename string) (audio.Voice, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("name", name); err != nil {
		return audio.Voice{}, fmt.Errorf("clone: encode name: %w", err)
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return audio.Voice{}, fmt.Errorf("clone: encode file: %w", err)
	}
	if _, err := fw.Write(audioBytes); err != nil {
		return audio.Voice{}, fmt.Errorf("clone: write file: %w", err)
	}
	if err := mw.Close(); err != nil {
		return audio.Voice{}, fmt.Errorf("clone: finish multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/v1/voices", &buf)
	if err != nil {
		return audio.Voice{}, fmt.Errorf("clone: build request: %w", err)
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return audio.Voice{}, fmt.Errorf("clone worker unreachable at %s: %w", p.endpoint, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return audio.Voice{}, fmt.Errorf("clone: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return audio.Voice{}, fmt.Errorf("clone worker returned %d: %s", resp.StatusCode, truncate(bytes.TrimSpace(raw), 300))
	}
	var parsed voicesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Voice == nil {
		return audio.Voice{}, fmt.Errorf("clone: decode register response: %w", err)
	}
	p.invalidateVoices()
	return audio.Voice{ID: parsed.Voice.ID, Name: parsed.Voice.Name, Category: "cloned"}, nil
}

// DeleteVoice removes a registered clone voice on the worker.
func (p *Provider) DeleteVoice(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, p.endpoint+"/v1/voices/"+id, nil)
	if err != nil {
		return fmt.Errorf("clone: build request: %w", err)
	}
	p.setAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("clone worker unreachable at %s: %w", p.endpoint, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("clone worker returned %d deleting voice %q", resp.StatusCode, id)
	}
	p.invalidateVoices()
	return nil
}

// invalidateVoices drops the cached voice list so the next ListVoices refetches.
func (p *Provider) invalidateVoices() {
	p.mu.Lock()
	p.voices = nil
	p.voicesAt = time.Time{}
	p.mu.Unlock()
}

func (p *Provider) setAuth(req *http.Request) {
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

func truncate(b []byte, maxLen int) string {
	s := string(b)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
