package http

import (
	"context"
	"net/http"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
	"github.com/nextlevelbuilder/goclaw/internal/audio/edge"
	"github.com/nextlevelbuilder/goclaw/internal/audio/elevenlabs"
	"github.com/nextlevelbuilder/goclaw/internal/audio/gemini"
	"github.com/nextlevelbuilder/goclaw/internal/audio/minimax"
	"github.com/nextlevelbuilder/goclaw/internal/audio/openai"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
)

// ttsCapabilitiesResponse is the JSON envelope for GET /v1/tts/capabilities.
type ttsCapabilitiesResponse struct {
	Providers []audio.ProviderCapabilities `json:"providers"`
}

// builtinTTSCapabilities returns the static capability catalog for every known
// TTS provider — independent of which providers happen to be registered in the
// manager at runtime. Frontend needs this so users can browse + configure a
// provider before saving credentials. Each provider's Capabilities() reads no
// receiver state, so calling on a zero-value Provider is safe.
func builtinTTSCapabilities() []audio.ProviderCapabilities {
	return []audio.ProviderCapabilities{
		(&openai.Provider{}).Capabilities(),
		(&elevenlabs.TTSProvider{}).Capabilities(),
		(&edge.Provider{}).Capabilities(),
		(&minimax.Provider{}).Capabilities(),
		(&gemini.Provider{}).Capabilities(),
	}
}

// handleCapabilities serves GET /v1/tts/capabilities.
// Returns catalog-level metadata for all known TTS providers — registered
// providers in the manager take precedence; unregistered ones fall back to
// the static builtin catalog so users can configure a fresh provider.
// Registered providers that expose dynamic voices (VoiceListProvider) with
// no static catalog get their live voices merged in; failures are silent
// (the worker may simply be offline). Response is tenant-agnostic.
func (h *TTSHandler) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	mgr := h.manager
	h.mu.RUnlock()

	var registered []audio.ProviderCapabilities
	if mgr != nil {
		registered = mgr.ListCapabilities()
	}

	seen := make(map[string]bool, len(registered))
	caps := make([]audio.ProviderCapabilities, 0, 5)
	for _, c := range registered {
		if len(c.Voices) == 0 {
			c.Voices = dynamicVoiceOptions(r, mgr, c.Provider)
		}
		caps = append(caps, c)
		seen[c.Provider] = true
	}
	for _, c := range builtinTTSCapabilities() {
		if !seen[c.Provider] {
			caps = append(caps, c)
		}
	}

	writeJSON(w, http.StatusOK, ttsCapabilitiesResponse{Providers: caps})
}

// dynamicVoiceOptions fetches live voices from a registered VoiceListProvider
// with a short deadline — the endpoint must stay responsive even when the
// backing worker is slow or offline.
func dynamicVoiceOptions(r *http.Request, mgr *audio.Manager, providerName string) []audio.VoiceOption {
	if mgr == nil {
		return nil
	}
	lp, ok := mgr.LookupVoiceProvider(providerName)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
	defer cancel()
	voices, err := lp.ListVoices(ctx)
	if err != nil {
		return nil
	}
	out := make([]audio.VoiceOption, 0, len(voices))
	for _, v := range voices {
		out = append(out, audio.VoiceOption{VoiceID: v.ID, Name: v.Name})
	}
	return out
}

// registerCapabilitiesRoute wires GET /v1/tts/capabilities onto mux.
// Called from TTSHandler.RegisterRoutes to keep all TTS routes co-located.
func (h *TTSHandler) registerCapabilitiesRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/tts/capabilities",
		requireAuth(permissions.RoleOperator, h.handleCapabilities))
}
