package minimax

import "github.com/nextlevelbuilder/goclaw/internal/audio"

// minimaxModels lists MiniMax T2A v2 model IDs.
var minimaxModels = []string{
	"speech-02-hd",
	"speech-02-turbo",
	"speech-01-hd",
	"speech-01-turbo",
}

var (
	mmSpeedMin  = 0.5
	mmSpeedMax  = 2.0
	mmSpeedStep = 0.1
	mmVolMin    = 0.01
	mmVolMax    = 10.0
	mmVolStep   = 0.01
	mmPitchMin  = -12.0
	mmPitchMax  = 12.0
	mmPitchStep = 1.0
)

// minimaxParams is the enriched param schema for MiniMax TTS.
// Defaults MUST match the hardcoded body in tts.go (characterization fixture):
//   speed=1.0, vol=1.0 (omitted from original = 1.0 default), pitch=0.
// Note: "vol" key is not in the original body; adding it with default 1.0
// is safe because server default is also 1.0.
var minimaxParams = []audio.ParamSchema{
	{
		Key:         "speed",
		Type:        audio.ParamTypeRange,
		Label:       "Speed",
		Description: "Speech speed multiplier (0.5 = slowest, 2.0 = fastest).",
		Default:     1.0,
		Min:         &mmSpeedMin,
		Max:         &mmSpeedMax,
		Step:        &mmSpeedStep,
	},
	{
		Key:         "vol",
		Type:        audio.ParamTypeRange,
		Label:       "Volume",
		Description: "Volume multiplier (0.01 = quietest, 10.0 = loudest).",
		Default:     1.0,
		Min:         &mmVolMin,
		Max:         &mmVolMax,
		Step:        &mmVolStep,
	},
	{
		Key:         "pitch",
		Type:        audio.ParamTypeInteger,
		Label:       "Pitch",
		Description: "Pitch shift in semitones (-12 to +12).",
		Default:     0,
		Min:         &mmPitchMin,
		Max:         &mmPitchMax,
		Step:        &mmPitchStep,
	},
	{
		Key:   "emotion",
		Type:  audio.ParamTypeEnum,
		Label: "Emotion",
		Default: "",
		Enum: []audio.EnumOption{
			{Value: "", Label: "None (default)"},
			{Value: "happy", Label: "Happy"},
			{Value: "sad", Label: "Sad"},
			{Value: "angry", Label: "Angry"},
			{Value: "fearful", Label: "Fearful"},
			{Value: "disgusted", Label: "Disgusted"},
			{Value: "surprised", Label: "Surprised"},
			{Value: "neutral", Label: "Neutral"},
			{Value: "excited", Label: "Excited"},
			{Value: "anxious", Label: "Anxious"},
		},
	},
	{
		Key:     "text_normalization",
		Type:    audio.ParamTypeBoolean,
		Label:   "Text Normalization",
		Default: nil, // omit from body when not explicitly set
	},
	{
		Key:     "audio.format",
		Type:    audio.ParamTypeEnum,
		Label:   "Audio Format",
		Default: "mp3",
		Enum: []audio.EnumOption{
			{Value: "mp3", Label: "MP3"},
			{Value: "pcm", Label: "PCM"},
			{Value: "flac", Label: "FLAC"},
			{Value: "wav", Label: "WAV"},
		},
	},
	{
		Key:     "audio.sample_rate",
		Type:    audio.ParamTypeEnum,
		Label:   "Sample Rate",
		Default: "",
		Enum: []audio.EnumOption{
			{Value: "", Label: "Default"},
			{Value: "8000", Label: "8 kHz"},
			{Value: "16000", Label: "16 kHz"},
			{Value: "22050", Label: "22 kHz"},
			{Value: "24000", Label: "24 kHz"},
			{Value: "32000", Label: "32 kHz"},
			{Value: "44100", Label: "44.1 kHz"},
		},
	},
	{
		Key:     "audio.bitrate",
		Type:    audio.ParamTypeEnum,
		Label:   "Bitrate (MP3 only)",
		Default: "",
		Enum: []audio.EnumOption{
			{Value: "", Label: "Default"},
			{Value: "32000", Label: "32 kbps"},
			{Value: "64000", Label: "64 kbps"},
			{Value: "128000", Label: "128 kbps"},
			{Value: "256000", Label: "256 kbps"},
		},
		DependsOn: []audio.Dependency{
			{Field: "audio.format", Op: "eq", Value: "mp3"},
		},
	},
	{
		Key:     "audio.channel",
		Type:    audio.ParamTypeEnum,
		Label:   "Channels",
		Default: "",
		Enum: []audio.EnumOption{
			{Value: "", Label: "Default"},
			{Value: "1", Label: "Mono"},
			{Value: "2", Label: "Stereo"},
		},
	},
}

// Capabilities returns the full capability schema for the MiniMax TTS provider.
// VoicesDynamic=true signals the frontend to fetch voices from /v1/voices?provider=minimax.
func (p *Provider) Capabilities() audio.ProviderCapabilities {
	return audio.ProviderCapabilities{
		Provider:       "minimax",
		DisplayName:    "MiniMax TTS",
		RequiresAPIKey: true,
		Models:         minimaxModels,
		// Voices is nil — fetched dynamically via VoiceListProvider
		Params: minimaxParams,
		CustomFeatures: map[string]any{
			"voices_dynamic": true,
		},
	}
}
