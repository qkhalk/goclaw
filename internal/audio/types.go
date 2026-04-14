// Package audio unifies TTS, STT, Music, and SFX generation under a single
// Manager. Replaces internal/tts (surface preserved via internal/tts/alias.go
// backward-compat layer).
//
// Phase 1 delivers TTS parity; STT/Music/SFX interfaces ship as stubs —
// implementations land in Phase 3 (Music/SFX) and Phase 4 (STT).
package audio

import "context"

// ---- TTS (implemented Phase 1) ----

// TTSProvider synthesizes text into audio bytes.
type TTSProvider interface {
	Name() string
	Synthesize(ctx context.Context, text string, opts TTSOptions) (*SynthResult, error)
}

// TTSOptions controls TTS synthesis parameters.
type TTSOptions struct {
	Voice  string // provider-specific voice ID
	Model  string // provider-specific model ID
	Format string // output format: "mp3", "opus", etc.
}

// SynthResult is the output of a TTS synthesis call.
type SynthResult struct {
	Audio     []byte // raw audio bytes
	Extension string // file extension without dot: "mp3", "opus", "ogg"
	MimeType  string // e.g. "audio/mpeg", "audio/ogg"
}

// AutoMode controls when TTS is automatically applied to replies.
type AutoMode string

const (
	AutoOff     AutoMode = "off"     // Disabled
	AutoAlways  AutoMode = "always"  // Apply to all eligible replies
	AutoInbound AutoMode = "inbound" // Only if user sent audio/voice
	AutoTagged  AutoMode = "tagged"  // Only if reply contains [[tts]] directive
)

// Mode controls which reply kinds get TTS.
type Mode string

const (
	ModeFinal Mode = "final" // Only final replies (default)
	ModeAll   Mode = "all"   // All replies including tool/block
)

// ---- STT (stubs — implementations land in Phase 4) ----

// STTProvider transcribes audio bytes to text.
type STTProvider interface {
	Name() string
	Transcribe(ctx context.Context, in STTInput, opts STTOptions) (*TranscriptResult, error)
}

// STTInput is the audio to transcribe. At most one of Audio/FilePath is set.
type STTInput struct {
	Audio    []byte // raw audio bytes (in-memory)
	FilePath string // path on disk (used by proxy_stt)
	MimeType string // e.g. "audio/ogg"
}

// STTOptions tunes transcription.
type STTOptions struct {
	Language string // BCP-47 hint, empty = auto-detect
	Model    string // provider-specific model ID
}

// TranscriptResult is the output of transcription.
type TranscriptResult struct {
	Text     string
	Language string // detected or hinted language
}

// ---- Music (stubs — implementations land in Phase 3) ----

// MusicProvider generates music from prompt + optional lyrics.
type MusicProvider interface {
	Name() string
	GenerateMusic(ctx context.Context, opts MusicOptions) (*AudioResult, error)
}

// MusicOptions controls music generation.
type MusicOptions struct {
	Prompt   string
	Lyrics   string
	Duration int // seconds
}

// ---- SFX (stubs — implementations land in Phase 3) ----

// SFXProvider generates short sound effects from a prompt.
type SFXProvider interface {
	Name() string
	GenerateSFX(ctx context.Context, opts SFXOptions) (*AudioResult, error)
}

// SFXOptions controls SFX generation.
type SFXOptions struct {
	Prompt   string
	Duration int // seconds (provider may cap)
}

// AudioResult is the shared output of music/SFX generation.
type AudioResult struct {
	Audio     []byte
	Extension string
	MimeType  string
}
