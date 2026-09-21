package clone

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
)

func TestSynthesizeForwardsVoiceAndParams(t *testing.T) {
	var gotPath, gotAuth, gotContentType string
	var gotBody synthesizeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/v1/tts" {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte("RIFF....WAVE"))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewProvider(Config{Endpoint: srv.URL, APIKey: "sekret", Voice: "anh"})
	res, err := p.Synthesize(context.Background(), "Xin chào", audio.TTSOptions{
		Params: map[string]any{"base_voice": "vi-VN-NamMinhNeural", "speed": float64(1.1)},
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if gotPath != "/v1/tts" {
		t.Errorf("path = %q, want /v1/tts", gotPath)
	}
	if gotAuth != "Bearer sekret" {
		t.Errorf("auth = %q, want bearer token", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("content type = %q", gotContentType)
	}
	if gotBody.VoiceID != "anh" {
		t.Errorf("voice_id = %q, want configured default %q", gotBody.VoiceID, "anh")
	}
	if gotBody.BaseVoice != "vi-VN-NamMinhNeural" {
		t.Errorf("base_voice = %q", gotBody.BaseVoice)
	}
	if gotBody.Speed != 1.1 {
		t.Errorf("speed = %v, want 1.1", gotBody.Speed)
	}
	if res.MimeType != "audio/wav" || res.Extension != "wav" {
		t.Errorf("mime/ext = %s/%s, want audio/wav/wav", res.MimeType, res.Extension)
	}
	if string(res.Audio) != "RIFF....WAVE" {
		t.Errorf("audio bytes = %q", res.Audio)
	}
}

func TestSynthesizeVoiceOverrideAndMp3Mime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body synthesizeRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.VoiceID != "me" {
			t.Errorf("voice_id = %q, want override %q", body.VoiceID, "me")
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3"))
	}))
	defer srv.Close()

	p := NewProvider(Config{Endpoint: srv.URL})
	res, err := p.Synthesize(context.Background(), "hi", audio.TTSOptions{Voice: "me"})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if res.MimeType != "audio/mpeg" || res.Extension != "mp3" {
		t.Errorf("mime/ext = %s/%s, want audio/mpeg/mp3", res.MimeType, res.Extension)
	}
}

func TestSynthesizeWorkerErrorSurfacesStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "voice not found: zz", http.StatusBadRequest)
	}))
	defer srv.Close()

	p := NewProvider(Config{Endpoint: srv.URL})
	_, err := p.Synthesize(context.Background(), "hi", audio.TTSOptions{})
	if err == nil {
		t.Fatal("want error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "voice not found") {
		t.Errorf("error should carry status + body, got: %v", err)
	}
}

func TestSynthesizeUnreachableEndpoint(t *testing.T) {
	p := NewProvider(Config{Endpoint: "http://127.0.0.1:1"})
	_, err := p.Synthesize(context.Background(), "hi", audio.TTSOptions{})
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("want unreachable error, got: %v", err)
	}
}

func TestListVoicesCachesUntilTTL(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/voices" && r.Method == http.MethodGet {
			calls++
			_, _ = w.Write([]byte(`{"voices":[{"id":"anh","name":"Giọng anh","created_at":"2026-09-19T00:00:00Z"}]}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewProvider(Config{Endpoint: srv.URL})
	for i := 0; i < 3; i++ {
		voices, err := p.ListVoices(context.Background())
		if err != nil {
			t.Fatalf("ListVoices: %v", err)
		}
		if len(voices) != 1 || voices[0].ID != "anh" {
			t.Fatalf("voices = %+v", voices)
		}
		if voices[0].Category != "cloned" {
			t.Errorf("category = %q, want cloned", voices[0].Category)
		}
	}
	if calls != 1 {
		t.Errorf("worker hit %d times, want 1 (cache)", calls)
	}
}

func TestRegisterAndDeleteInvalidateCache(t *testing.T) {
	listCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/voices":
			listCalls++
			_, _ = w.Write([]byte(`{"voices":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/voices":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("multipart parse: %v", err)
			}
			if r.FormValue("name") != "Giọng anh" {
				t.Errorf("name field = %q", r.FormValue("name"))
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("file field: %v", err)
			}
			defer file.Close()
			data, _ := io.ReadAll(file)
			if string(data) != "WAVDATA" || header.Filename != "sample.wav" {
				t.Errorf("file = %q (%s)", data, header.Filename)
			}
			_, _ = w.Write([]byte(`{"voice":{"id":"anh","name":"Giọng anh"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/voices/anh":
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewProvider(Config{Endpoint: srv.URL})
	if _, err := p.ListVoices(context.Background()); err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	v, err := p.RegisterVoice(context.Background(), "Giọng anh", []byte("WAVDATA"), "sample.wav")
	if err != nil {
		t.Fatalf("RegisterVoice: %v", err)
	}
	if v.ID != "anh" || v.Name != "Giọng anh" {
		t.Errorf("registered voice = %+v", v)
	}
	if _, err := p.ListVoices(context.Background()); err != nil {
		t.Fatalf("ListVoices after register: %v", err)
	}
	if listCalls != 2 {
		t.Errorf("list calls = %d, want 2 (register must invalidate cache)", listCalls)
	}
	if err := p.DeleteVoice(context.Background(), "anh"); err != nil {
		t.Fatalf("DeleteVoice: %v", err)
	}
}

func TestCapabilitiesCatalogEntry(t *testing.T) {
	p := NewProvider(Config{Endpoint: "http://x"})
	caps := p.Capabilities()
	if caps.Provider != "clone" || caps.DisplayName == "" {
		t.Errorf("capabilities = %+v", caps)
	}
	if caps.RequiresAPIKey {
		t.Error("clone must not require an API key (worker token optional)")
	}
}
