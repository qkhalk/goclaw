package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/audio/clone"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Clone voice registry routes. The gateway holds no voice data itself — these
// handlers proxy to the configured clone worker (contrib/voiceclone), so the
// gateway box never stores reference audio or speaker embeddings beyond the
// lifetime of a single request.

const (
	// maxCloneVoiceUploadBytes caps reference-audio uploads. A minute of WAV
	// is ~5MB; 16MB gives headroom for uncompressed samples without letting
	// uploads become a memory DoS.
	maxCloneVoiceUploadBytes = 16 << 20
	// maxCloneVoiceNameChars bounds the human label stored on the worker.
	maxCloneVoiceNameChars = 100
)

// cloneVoiceExts lists accepted reference-audio extensions. The worker
// transcodes to its working format; the gateway only gates obvious garbage.
var cloneVoiceExts = map[string]bool{
	".wav": true, ".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".webm": true, ".flac": true,
}

// resolveCloneProvider builds the clone provider for this request: tenant
// config first (system_configs tts.clone.*), falling back to the globally
// registered manager provider. Errors when neither is configured.
func (h *TTSHandler) resolveCloneProvider(ctx context.Context) (*clone.Provider, error) {
	if h.systemConfigs != nil {
		endpoint, _ := h.systemConfigs.Get(ctx, "tts.clone.endpoint")
		if endpoint != "" {
			var apiKey, voice string
			if h.configSecrets != nil {
				apiKey, _ = h.configSecrets.Get(ctx, "tts.clone.api_key")
			}
			voice, _ = h.systemConfigs.Get(ctx, "tts.clone.voice")
			return clone.NewProvider(clone.Config{Endpoint: endpoint, APIKey: apiKey, Voice: voice}), nil
		}
	}

	h.mu.RLock()
	mgr := h.manager
	h.mu.RUnlock()
	if mgr != nil {
		if p, ok := mgr.GetProvider("clone"); ok {
			if cp, ok := p.(*clone.Provider); ok {
				return cp, nil
			}
		}
	}
	return nil, errors.New("clone provider not configured")
}

// handleCloneVoicesList serves GET /v1/tts/clone/voices — proxies the
// worker's registered clone voices.
func (h *TTSHandler) handleCloneVoicesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := store.LocaleFromContext(ctx)

	p, err := h.resolveCloneProvider(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, i18n.T(locale, i18n.MsgTtsCloneNotConfigured)), http.StatusServiceUnavailable)
		return
	}

	voices, err := p.ListVoices(ctx)
	if err != nil {
		slog.Warn("tts.clone.voices.failed", "error", err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"voices": voices})
}

// handleCloneVoicesRegister serves POST /v1/tts/clone/voices — multipart
// upload (fields: name, file) forwarded to the worker, which extracts the
// speaker embedding.
func (h *TTSHandler) handleCloneVoicesRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := store.LocaleFromContext(ctx)

	p, err := h.resolveCloneProvider(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, i18n.T(locale, i18n.MsgTtsCloneNotConfigured)), http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCloneVoiceUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, `{"error":"invalid multipart form (16MB upload cap)"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || len([]rune(name)) > maxCloneVoiceNameChars {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, i18n.T(locale, i18n.MsgTtsCloneNameInvalid, maxCloneVoiceNameChars)), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file field is required (reference audio)"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	dot := strings.LastIndex(header.Filename, ".")
	if dot < 0 || !cloneVoiceExts[strings.ToLower(header.Filename[dot:])] {
		http.Error(w, `{"error":"unsupported audio format (use wav, mp3, m4a, aac, ogg, webm or flac)"}`, http.StatusUnsupportedMediaType)
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxCloneVoiceUploadBytes+1))
	if err != nil {
		http.Error(w, `{"error":"failed reading upload"}`, http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, `{"error":"empty audio file"}`, http.StatusBadRequest)
		return
	}
	if len(data) > maxCloneVoiceUploadBytes {
		http.Error(w, `{"error":"reference audio exceeds 16MB"}`, http.StatusRequestEntityTooLarge)
		return
	}

	voice, err := p.RegisterVoice(ctx, name, data, header.Filename)
	if err != nil {
		slog.Warn("tts.clone.register.failed", "name_len", len(name), "bytes", len(data), "error", err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	slog.Info("tts.clone.register.ok", "voice_id", voice.ID, "bytes", len(data))
	writeJSON(w, http.StatusOK, map[string]any{"voice": voice})
}

// handleCloneVoicesDelete serves DELETE /v1/tts/clone/voices/{id}.
func (h *TTSHandler) handleCloneVoicesDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := store.LocaleFromContext(ctx)

	p, err := h.resolveCloneProvider(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, i18n.T(locale, i18n.MsgTtsCloneNotConfigured)), http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if !validCloneVoiceID(id) {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, i18n.T(locale, i18n.MsgTtsCloneIDInvalid)), http.StatusBadRequest)
		return
	}

	if err := p.DeleteVoice(ctx, id); err != nil {
		slog.Warn("tts.clone.delete.failed", "voice_id", id, "error", err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// validCloneVoiceID gates the id before it is appended to the worker URL —
// only slug characters, so a crafted id can never leave the /v1/voices path.
func validCloneVoiceID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
