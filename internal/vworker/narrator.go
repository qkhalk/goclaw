package vworker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Narrator synthesizes text to speech audio files.
type Narrator interface {
	// Synthesize converts text to an audio file and returns the path.
	// voice is optional (provider default used if empty).
	Synthesize(ctx context.Context, text, voice, outputPath string) error
}

// edgeNarrator implements Narrator by shelling out to the edge-tts CLI.
type edgeNarrator struct {
	// Voice is the default voice when not overridden by per-scene voice.
	Voice string
}

// NewEdgeNarrator returns an Edge TTS narrator with defaults.
func NewEdgeNarrator(defaultVoice string) *edgeNarrator {
	if defaultVoice == "" {
		defaultVoice = "en-US-MichelleNeural"
	}
	return &edgeNarrator{Voice: defaultVoice}
}

// Synthesize calls edge-tts CLI to produce an MP3 file.
func (n *edgeNarrator) Synthesize(ctx context.Context, text, voice, outputPath string) error {
	if voice == "" {
		voice = n.Voice
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// edge-tts writes to a file, so we use a temp file then rename.
	tmpPath := outputPath + ".tmp"

	args := []string{
		"--voice", voice,
		"--text", text,
		"--write-media", tmpPath,
	}

	cmd := exec.CommandContext(ctx, "edge-tts", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("edge-tts failed: %w (output: %s)", err, truncate(string(output), 300))
	}

	// Rename temp to final
	if err := os.Rename(tmpPath, outputPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename tts output: %w", err)
	}

	slog.Info("tts synthesized", "voice", voice, "path", outputPath, "text_len", len(text))
	return nil
}

// NarratorFromName returns a Narrator for the given provider name.
func NarratorFromName(provider, defaultVoice string) Narrator {
	switch provider {
	case "edge", "":
		return NewEdgeNarrator(defaultVoice)
	default:
		slog.Warn("unknown narrator provider, falling back to edge", "provider", provider)
		return NewEdgeNarrator(defaultVoice)
	}
}

// CloneVoicePrefix marks narration voices that must be synthesized by the
// voice-clone worker: "clone:<voice-id>" (e.g. "clone:anh").
const CloneVoicePrefix = "clone:"

// IsCloneVoice reports whether the narration voice targets the clone worker.
func IsCloneVoice(voice string) bool { return strings.HasPrefix(voice, CloneVoicePrefix) }

// pickNarrator selects the narrator for a scene voice. Clone voices are
// stripped of their prefix and routed to cloneNar when configured; when the
// worker is absent the voice is cleared so the scene falls back to the default
// edge voice instead of feeding a bogus "clone:..." name to edge-tts.
func pickNarrator(voice string, edge, cloneNar Narrator) (Narrator, string) {
	if !IsCloneVoice(voice) {
		return edge, voice
	}
	stripped := strings.TrimPrefix(voice, CloneVoicePrefix)
	if cloneNar == nil {
		return edge, ""
	}
	return cloneNar, stripped
}

// cloneNarrator implements Narrator by POSTing to the self-hosted voice-clone
// worker (contrib/voiceclone — OpenVoice v2 tone conversion over edge-tts).
// Inference runs on the worker machine; the worker box does nothing but
// forward text and receive audio bytes.
type cloneNarrator struct {
	endpoint string
	apiKey   string
	// Voice is the worker-side default clone voice when a scene has none.
	Voice string
}

// NewCloneNarrator returns a clone worker narrator.
func NewCloneNarrator(endpoint, apiKey, defaultVoice string) *cloneNarrator {
	return &cloneNarrator{
		endpoint: strings.TrimRight(endpoint, "/"),
		apiKey:   apiKey,
		Voice:    defaultVoice,
	}
}

// cloneSynthRequest is the JSON body for the worker's POST /v1/tts.
type cloneSynthRequest struct {
	Text    string  `json:"text"`
	VoiceID string  `json:"voice_id,omitempty"`
	Speed   float64 `json:"speed,omitempty"`
}

// Synthesize forwards the scene text to the clone worker and writes the
// returned audio (WAV) to outputPath.
func (n *cloneNarrator) Synthesize(ctx context.Context, text, voice, outputPath string) error {
	// Defensive strip — pickNarrator normally removes the prefix, but the
	// narrator stays correct even if a future call path forgets.
	voice = strings.TrimPrefix(voice, CloneVoicePrefix)
	if voice == "" {
		voice = n.Voice
	}

	body, err := json.Marshal(cloneSynthRequest{Text: text, VoiceID: voice})
	if err != nil {
		return fmt.Errorf("clone narrator: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint+"/v1/tts", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("clone narrator: build request: %w", err)
	}
	if n.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+n.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("clone worker unreachable at %s: %w", n.endpoint, err)
	}
	defer resp.Body.Close()
	audio, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return fmt.Errorf("clone narrator: read audio: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("clone worker returned %d: %s", resp.StatusCode, truncate(string(bytes.TrimSpace(audio)), 300))
	}
	if len(audio) == 0 {
		return fmt.Errorf("clone worker returned empty audio")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	tmpPath := outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, audio, 0o644); err != nil {
		return fmt.Errorf("write tts output: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename tts output: %w", err)
	}

	slog.Info("clone tts synthesized", "voice", voice, "path", outputPath, "text_len", len(text))
	return nil
}

// SynthesizeScene narrates a scene's narration text if present.
// Returns the audio file path, or empty string if no narration.
// edge-tts intermittently 403s a datacenter IP on first contact — retry
// once after a short backoff before giving up on the scene's audio.
func SynthesizeScene(ctx context.Context, narrator Narrator, sceneIndex int, narrationText, narrationVoice, tempDir string) (string, error) {
	if narrationText == "" {
		return "", nil
	}

	outPath := filepath.Join(tempDir, fmt.Sprintf("narr_%03d.mp3", sceneIndex))
	start := time.Now()

	err := narrator.Synthesize(ctx, narrationText, narrationVoice, outPath)
	if err != nil {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("synthesize scene %d: %w", sceneIndex, err)
		case <-time.After(2 * time.Second):
		}
		slog.Warn("narration synth retrying after failure", "scene", sceneIndex, "err", err)
		if retryErr := narrator.Synthesize(ctx, narrationText, narrationVoice, outPath); retryErr == nil {
			err = nil
		} else {
			err = retryErr
		}
	}
	if err != nil {
		return "", fmt.Errorf("synthesize scene %d: %w", sceneIndex, err)
	}

	slog.Info("scene narrated", "scene", sceneIndex, "duration", time.Since(start), "path", outPath)
	return outPath, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
