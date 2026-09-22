package vworker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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

// SynthesizeScene narrates a scene's narration text if present.
// Returns the audio file path, or empty string if no narration.
func SynthesizeScene(ctx context.Context, narrator Narrator, sceneIndex int, narrationText, narrationVoice, tempDir string) (string, error) {
	if narrationText == "" {
		return "", nil
	}

	outPath := filepath.Join(tempDir, fmt.Sprintf("narr_%03d.mp3", sceneIndex))
	start := time.Now()

	if err := narrator.Synthesize(ctx, narrationText, narrationVoice, outPath); err != nil {
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
