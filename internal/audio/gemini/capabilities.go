package gemini

import "github.com/nextlevelbuilder/goclaw/internal/audio"

// Capabilities returns the static capability schema for the Gemini TTS provider.
// CustomFeatures drives frontend custom slot rendering in Phase C.
func (p *Provider) Capabilities() audio.ProviderCapabilities {
	return audio.ProviderCapabilities{
		Provider:       "gemini",
		DisplayName:    "Google Gemini TTS",
		RequiresAPIKey: true,
		Models:         geminiModels,
		Voices:         geminiVoices,
		// Params is nil — Gemini TTS has no speed/pitch knobs.
		CustomFeatures: map[string]any{
			"multi_speaker": true,
			"audio_tags":    true,
		},
	}
}
