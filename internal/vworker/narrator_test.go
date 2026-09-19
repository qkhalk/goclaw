package vworker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsCloneVoiceAndStrip(t *testing.T) {
	cases := []struct {
		voice string
		want  bool
	}{
		{"clone:anh", true},
		{"clone:", true},
		{"vi-VN-HoaiMyNeural", false},
		{"", false},
		{"clone", false},
	}
	for _, c := range cases {
		if got := IsCloneVoice(c.voice); got != c.want {
			t.Errorf("IsCloneVoice(%q) = %v, want %v", c.voice, got, c.want)
		}
	}
}

func TestPickNarrator(t *testing.T) {
	edge := NewEdgeNarrator("vi-VN-HoaiMyNeural")
	cloneNar := NewCloneNarrator("http://worker:1", "tok", "anh")

	// Clone voice + worker configured → clone narrator with stripped id.
	nar, voice := pickNarrator("clone:me", edge, cloneNar)
	if nar != Narrator(cloneNar) {
		t.Error("clone voice with worker must route to clone narrator")
	}
	if voice != "me" {
		t.Errorf("voice = %q, want stripped %q", voice, "me")
	}

	// Clone voice without worker → edge narrator, empty voice (default).
	nar, voice = pickNarrator("clone:me", edge, nil)
	if nar != Narrator(edge) {
		t.Error("clone voice without worker must fall back to edge narrator")
	}
	if voice != "" {
		t.Errorf("voice = %q, want empty fallback", voice)
	}

	// Regular voice → edge narrator untouched.
	nar, voice = pickNarrator("vi-VN-NamMinhNeural", edge, cloneNar)
	if nar != Narrator(edge) {
		t.Error("edge voice must stay on edge narrator")
	}
	if voice != "vi-VN-NamMinhNeural" {
		t.Errorf("voice = %q, want unchanged", voice)
	}
}

func TestCloneNarratorSynthesize(t *testing.T) {
	var gotAuth, gotVoice, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tts" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		var body cloneSynthRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotVoice = body.VoiceID
		gotText = body.Text
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFF....WAVE"))
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "narr_000.mp3")
	nar := NewCloneNarrator(srv.URL, "sekret", "anh")
	if err := nar.Synthesize(context.Background(), "Xin chào", "clone:me", outPath); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if gotAuth != "Bearer sekret" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotVoice != "me" {
		t.Errorf("voice_id = %q, want prefix stripped %q", gotVoice, "me")
	}
	if gotText != "Xin chào" {
		t.Errorf("text = %q", gotText)
	}
	data, err := os.ReadFile(outPath)
	if err != nil || string(data) != "RIFF....WAVE" {
		t.Errorf("output file = %q err=%v", data, err)
	}
}

func TestCloneNarratorDefaultsAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body cloneSynthRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.VoiceID != "default-voice" {
			t.Errorf("voice_id = %q, want narrator default", body.VoiceID)
		}
		http.Error(w, "voice not found", http.StatusInternalServerError)
	}))
	defer srv.Close()

	nar := NewCloneNarrator(srv.URL, "", "default-voice")
	err := nar.Synthesize(context.Background(), "hi", "", filepath.Join(t.TempDir(), "out.mp3"))
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("want 500 error surfaced, got: %v", err)
	}
}
