package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/audio"
)

const voicesCacheTTL = 5 * time.Minute

// VoiceLister fetches MiniMax voices via POST /v1/get_voice.
// It holds a per-tenant in-process cache with 5-min TTL and
// stale-while-refetch semantics on upstream 5xx errors.
type VoiceLister struct {
	apiKey    string
	apiBase   string
	timeoutMs int
	tenantID  uuid.UUID

	mu        sync.Mutex
	cached    []audio.Voice
	expiresAt time.Time
}

// NewVoiceLister creates a VoiceLister for the given tenant credentials.
func NewVoiceLister(apiKey, apiBase string, timeoutMs int, tenantID uuid.UUID) *VoiceLister {
	if apiBase == "" {
		apiBase = "https://api.minimax.io/v1"
	}
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	return &VoiceLister{
		apiKey:    apiKey,
		apiBase:   apiBase,
		timeoutMs: timeoutMs,
		tenantID:  tenantID,
	}
}

// ListVoices implements audio.VoiceListProvider.
// Returns cached voices when fresh. On upstream 5xx, returns stale cache
// (if available) or empty slice + structured error.
func (vl *VoiceLister) ListVoices(ctx context.Context) ([]audio.Voice, error) {
	vl.mu.Lock()
	defer vl.mu.Unlock()

	// Cache hit.
	if len(vl.cached) > 0 && time.Now().Before(vl.expiresAt) {
		return vl.cached, nil
	}

	voices, err := vl.fetchVoices(ctx)
	if err != nil {
		// Stale cache fallback on upstream errors.
		if len(vl.cached) > 0 {
			return vl.cached, nil
		}
		return []audio.Voice{}, err
	}

	vl.cached = voices
	vl.expiresAt = time.Now().Add(voicesCacheTTL)
	return voices, nil
}

// voiceGetRequest is the POST body for MiniMax /v1/get_voice.
type voiceGetRequest struct {
	VoiceType string `json:"voice_type"`
}

// voiceGetResponse mirrors the MiniMax /v1/get_voice response envelope.
type voiceGetResponse struct {
	SystemVoice    []voiceEntry `json:"system_voice"`
	VoiceCloning   []voiceEntry `json:"voice_cloning"`
	VoiceGeneration []voiceEntry `json:"voice_generation"`
}

type voiceEntry struct {
	VoiceID   string `json:"voice_id"`
	VoiceName string `json:"voice_name"`
}

func (vl *VoiceLister) fetchVoices(ctx context.Context) ([]audio.Voice, error) {
	reqBody, err := json.Marshal(voiceGetRequest{VoiceType: "all"})
	if err != nil {
		return nil, fmt.Errorf("minimax voices: marshal request: %w", err)
	}

	url := vl.apiBase + "/get_voice"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("minimax voices: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+vl.apiKey)

	hc := &http.Client{Timeout: time.Duration(vl.timeoutMs) * time.Millisecond}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("minimax voices: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("minimax voices: read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("minimax voices: unauthorized (401)")
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("minimax voices: upstream error %d: %s", resp.StatusCode, string(respBytes))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("minimax voices: unexpected status %d: %s", resp.StatusCode, string(respBytes))
	}

	var result voiceGetResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("minimax voices: parse response: %w", err)
	}

	voices := make([]audio.Voice, 0, len(result.SystemVoice)+len(result.VoiceCloning)+len(result.VoiceGeneration))
	for _, v := range result.SystemVoice {
		voices = append(voices, audio.Voice{
			ID:       v.VoiceID,
			Name:     v.VoiceName,
			Category: "system",
		})
	}
	for _, v := range result.VoiceCloning {
		voices = append(voices, audio.Voice{
			ID:       v.VoiceID,
			Name:     v.VoiceName,
			Category: "cloning",
		})
	}
	for _, v := range result.VoiceGeneration {
		voices = append(voices, audio.Voice{
			ID:       v.VoiceID,
			Name:     v.VoiceName,
			Category: "generation",
		})
	}
	return voices, nil
}
