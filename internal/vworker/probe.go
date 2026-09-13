package vworker

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// ProbeResult holds parsed ffprobe output.
type ProbeResult struct {
	Format  ProbeFormat  `json:"format"`
	Streams []ProbeStream `json:"streams"`
}

// ProbeFormat holds format-level info from ffprobe.
type ProbeFormat struct {
	Duration string `json:"duration"` // seconds as string
	Size     string `json:"size"`     // bytes as string
}

// ProbeStream holds per-stream info.
type ProbeStream struct {
	CodecType string `json:"codec_type"` // "video", "audio"
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// ProbeDuration calls ffprobe to get the duration in seconds of a media file.
// Returns 0 if duration cannot be determined.
func ProbeDuration(ctx context.Context, ffprobePath, filePath string) (float64, error) {
	result, err := Probe(ctx, ffprobePath, filePath)
	if err != nil {
		return 0, err
	}
	if result.Format.Duration == "" {
		return 0, nil
	}
	var dur float64
	if _, err := fmt.Sscanf(result.Format.Duration, "%f", &dur); err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", result.Format.Duration, err)
	}
	return dur, nil
}

// Probe runs ffprobe -v quiet -print_format json -show_format -show_streams
// and returns the parsed result.
func Probe(ctx context.Context, ffprobePath, filePath string) (*ProbeResult, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed for %s: %w", filePath, err)
	}

	var result ProbeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}
	return &result, nil
}

// ProbeVideoResolution calls ffprobe and returns width, height of the first
// video stream. Returns 0,0 if no video stream is found.
func ProbeVideoResolution(ctx context.Context, ffprobePath, filePath string) (int, int, error) {
	result, err := Probe(ctx, ffprobePath, filePath)
	if err != nil {
		return 0, 0, err
	}
	for _, s := range result.Streams {
		if s.CodecType == "video" {
			return s.Width, s.Height, nil
		}
	}
	return 0, 0, nil
}

// ProbeHasAudio returns true if the file has an audio stream.
func ProbeHasAudio(ctx context.Context, ffprobePath, filePath string) (bool, error) {
	result, err := Probe(ctx, ffprobePath, filePath)
	if err != nil {
		return false, err
	}
	for _, s := range result.Streams {
		if s.CodecType == "audio" {
			return true, nil
		}
	}
	return false, nil
}
