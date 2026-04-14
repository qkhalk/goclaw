package audio

import (
	"context"
	"fmt"
	"log/slog"
)

// Manager orchestrates audio providers across TTS, STT, Music, and SFX
// operations. Each op has its own provider map + primary/fallback chain.
//
// Phase 1 exercises the TTS path end-to-end. STT/Music/SFX maps and chains
// are present but empty (providers register in Phase 3/4).
type Manager struct {
	ttsProviders   map[string]TTSProvider
	sttProviders   map[string]STTProvider
	musicProviders map[string]MusicProvider
	sfxProviders   map[string]SFXProvider

	primary   string   // primary TTS provider
	sttChain  []string // STT fallback order (Phase 4)
	musicChain []string // Music fallback order (Phase 3)

	auto      AutoMode
	mode      Mode
	maxLength int // max text length before truncation (default 1500)
	timeoutMs int // provider timeout (default 30000)
}

// ManagerConfig configures the audio manager. Preserved from legacy TTS
// package — new STT/Music fields are set via RegisterSTT/RegisterMusic and
// (optionally) cfg.Audio in config_audio.go.
type ManagerConfig struct {
	Primary   string   // primary TTS provider name
	Auto      AutoMode // auto-apply mode (default "off")
	Mode      Mode     // "final" or "all" (default "final")
	MaxLength int      // default 1500
	TimeoutMs int      // default 30000
}

// NewManager creates an audio manager with empty provider maps.
func NewManager(cfg ManagerConfig) *Manager {
	m := &Manager{
		ttsProviders:   make(map[string]TTSProvider),
		sttProviders:   make(map[string]STTProvider),
		musicProviders: make(map[string]MusicProvider),
		sfxProviders:   make(map[string]SFXProvider),
		primary:        cfg.Primary,
		auto:           cfg.Auto,
		mode:           cfg.Mode,
		maxLength:      cfg.MaxLength,
		timeoutMs:      cfg.TimeoutMs,
	}
	if m.auto == "" {
		m.auto = AutoOff
	}
	if m.mode == "" {
		m.mode = ModeFinal
	}
	if m.maxLength <= 0 {
		m.maxLength = 1500
	}
	if m.timeoutMs <= 0 {
		m.timeoutMs = 30000
	}
	return m
}

// ---- Registration ----

// RegisterTTS adds a TTS provider. If no primary is set, the first registered
// provider becomes primary — matches legacy tts.Manager.RegisterProvider.
func (m *Manager) RegisterTTS(p TTSProvider) {
	m.ttsProviders[p.Name()] = p
	if m.primary == "" {
		m.primary = p.Name()
	}
}

// RegisterProvider is a backward-compat alias for RegisterTTS — lets pre-Phase-1
// callers that go through tts.Manager (= audio.Manager via alias) keep working.
func (m *Manager) RegisterProvider(p TTSProvider) { m.RegisterTTS(p) }

// RegisterSTT adds an STT provider (Phase 4).
func (m *Manager) RegisterSTT(p STTProvider) {
	m.sttProviders[p.Name()] = p
}

// RegisterMusic adds a music provider (Phase 3).
func (m *Manager) RegisterMusic(p MusicProvider) {
	m.musicProviders[p.Name()] = p
}

// RegisterSFX adds an SFX provider (Phase 3).
func (m *Manager) RegisterSFX(p SFXProvider) {
	m.sfxProviders[p.Name()] = p
}

// ---- Introspection ----

// GetProvider returns a TTS provider by name. Preserved from legacy API.
func (m *Manager) GetProvider(name string) (TTSProvider, bool) {
	p, ok := m.ttsProviders[name]
	return p, ok
}

// PrimaryProvider returns the primary TTS provider name.
func (m *Manager) PrimaryProvider() string { return m.primary }

// AutoMode returns the current auto-apply mode.
func (m *Manager) AutoMode() AutoMode { return m.auto }

// HasProviders reports whether any TTS provider is registered.
func (m *Manager) HasProviders() bool { return len(m.ttsProviders) > 0 }

// ---- TTS dispatch ----

// Synthesize uses the primary provider.
func (m *Manager) Synthesize(ctx context.Context, text string, opts TTSOptions) (*SynthResult, error) {
	p, ok := m.ttsProviders[m.primary]
	if !ok {
		return nil, fmt.Errorf("tts provider not found: %s", m.primary)
	}
	return p.Synthesize(ctx, text, opts)
}

// SynthesizeWithFallback tries primary first, then any other registered
// provider on error. Returns first success or aggregate failure.
func (m *Manager) SynthesizeWithFallback(ctx context.Context, text string, opts TTSOptions) (*SynthResult, error) {
	if p, ok := m.ttsProviders[m.primary]; ok {
		if result, err := p.Synthesize(ctx, text, opts); err == nil {
			return result, nil
		} else {
			slog.Warn("tts primary provider failed, trying fallback", "provider", m.primary, "error", err)
		}
	}
	for name, p := range m.ttsProviders {
		if name == m.primary {
			continue
		}
		result, err := p.Synthesize(ctx, text, opts)
		if err == nil {
			slog.Info("tts fallback succeeded", "provider", name)
			return result, nil
		}
		slog.Warn("tts fallback provider failed", "provider", name, "error", err)
	}
	return nil, fmt.Errorf("all tts providers failed")
}
