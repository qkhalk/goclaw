package elevenlabs

import "github.com/nextlevelbuilder/goclaw/internal/audio"

// elevenLabsModels lists all allowed ElevenLabs TTS model IDs (mirrors AllowedElevenLabsModels).
var elevenLabsModels = []string{
	"eleven_v3",
	"eleven_flash_v2_5",
	"eleven_multilingual_v2",
	"eleven_turbo_v2_5",
}

var (
	stabilityMin      = 0.0
	stabilityMax      = 1.0
	stabilityStep     = 0.01
	simBoostMin       = 0.0
	simBoostMax       = 1.0
	simBoostStep      = 0.01
	styleMin          = 0.0
	styleMax          = 1.0
	styleStep         = 0.01
	elSpeedMin        = 0.7
	elSpeedMax        = 1.2
	elSpeedStep       = 0.01
	latencyMin        = 0.0
	latencyMax        = 4.0
	latencyStep       = 1.0
	seedMin           = 0.0
	seedMax           = 4294967295.0 // 2^32-1
	seedStep          = 1.0
)

// elevenLabsParams is the enriched param schema for ElevenLabs TTS.
// Defaults MUST match the hardcoded values in tts.go:54-59 (characterization fixture):
//   stability=0.5, similarity_boost=0.75, style=0.0, use_speaker_boost=true, speed=1.0
var elevenLabsParams = []audio.ParamSchema{
	{
		Key:         "voice_settings.stability",
		Type:        audio.ParamTypeRange,
		Label:       "Stability",
		Description: "Voice stability (0 = more variable, 1 = more consistent).",
		Default:     0.5,
		Min:         &stabilityMin,
		Max:         &stabilityMax,
		Step:        &stabilityStep,
	},
	{
		Key:         "voice_settings.similarity_boost",
		Type:        audio.ParamTypeRange,
		Label:       "Similarity Boost",
		Description: "Similarity to original voice (0 = more creative, 1 = more similar).",
		Default:     0.75,
		Min:         &simBoostMin,
		Max:         &simBoostMax,
		Step:        &simBoostStep,
	},
	{
		Key:         "voice_settings.style",
		Type:        audio.ParamTypeRange,
		Label:       "Style Exaggeration",
		Description: "Style exaggeration amount (0 = none, 1 = maximum).",
		Default:     0.0,
		Min:         &styleMin,
		Max:         &styleMax,
		Step:        &styleStep,
	},
	{
		Key:     "voice_settings.use_speaker_boost",
		Type:    audio.ParamTypeBoolean,
		Label:   "Speaker Boost",
		Default: true,
	},
	{
		Key:         "voice_settings.speed",
		Type:        audio.ParamTypeRange,
		Label:       "Speed",
		Description: "Speech speed (0.7 = slowest, 1.2 = fastest).",
		Default:     1.0,
		Min:         &elSpeedMin,
		Max:         &elSpeedMax,
		Step:        &elSpeedStep,
	},
	{
		Key:     "apply_text_normalization",
		Type:    audio.ParamTypeEnum,
		Label:   "Text Normalization",
		Default: "", // empty = omit from request (API defaults to "auto" server-side)
		Enum: []audio.EnumOption{
			{Value: "", Label: "Auto (default)"},
			{Value: "on", Label: "On"},
			{Value: "off", Label: "Off"},
		},
	},
	{
		Key:         "seed",
		Type:        audio.ParamTypeInteger,
		Label:       "Seed",
		Description: "Deterministic seed for reproducible output (0 = random). Advanced.",
		Default:     0,
		Min:         &seedMin,
		Max:         &seedMax,
		Step:        &seedStep,
	},
	{
		Key:         "optimize_streaming_latency",
		Type:        audio.ParamTypeRange,
		Label:       "Streaming Latency Optimization",
		Description: "Latency optimization level (0 = none, 4 = maximum). Advanced.",
		Default:     0.0,
		Min:         &latencyMin,
		Max:         &latencyMax,
		Step:        &latencyStep,
	},
	{
		Key:         "language_code",
		Type:        audio.ParamTypeString,
		Label:       "Language Code",
		Description: "ISO 639-1 language hint (e.g. 'en', 'vi'). Advanced.",
		Default:     "",
	},
}

// Capabilities returns the full capability schema for the ElevenLabs TTS provider.
// Voices is nil — ElevenLabs voices are user-specific and fetched dynamically via /v1/voices.
func (p *TTSProvider) Capabilities() audio.ProviderCapabilities {
	return audio.ProviderCapabilities{
		Provider:       "elevenlabs",
		DisplayName:    "ElevenLabs TTS",
		RequiresAPIKey: true,
		Models:         elevenLabsModels,
		Params:         elevenLabsParams,
	}
}
