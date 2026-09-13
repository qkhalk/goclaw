package vworker

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// FFmpegConfig holds the path to ffmpeg binary and rendering parameters.
type FFmpegConfig struct {
	FFmpegPath string // default "ffmpeg"
	FFProbePath string // default "ffprobe"
	FontFile   string // font file path for drawtext; empty = skip captions
	MaxSceneSec float64 // cap per-scene duration for safety; 0 = no cap
}

// common FFmpeg flags for "light" rendering.
var baseFlags = []string{"-preset", "veryfast", "-crf", "28", "-threads", "1", "-y"}

// escapeDrawText escapes text for the ffmpeg drawtext filter.
// ffmpeg drawtext requires escaping: single quotes, colons, backslashes, and brackets.
func escapeDrawText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `'\''`)
	s = strings.ReplaceAll(s, `:`, `\\:`)
	s = strings.ReplaceAll(s, `[`, `\\[`)
	s = strings.ReplaceAll(s, `]`, `\\]`)
	return s
}

// fontFile returns the fontfile arg portion if a font is configured; empty string otherwise.
func fontFileArg(fontFile string) string {
	if fontFile == "" {
		return ""
	}
	// escape the path for drawtext (spaces, colons, etc.)
	escaped := strings.ReplaceAll(fontFile, `:`, `\\:`)
	escaped = strings.ReplaceAll(escaped, `'`, `'\''`)
	return fmt.Sprintf("fontfile=%s", escaped)
}

// buildImageSceneArgs builds ffmpeg argv for an image scene with optional Ken Burns and caption.
// Output: scene_N.mp4
func buildImageSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}

	// Input: still image looped for duration_sec
	args = append(args, "-loop", "1", "-framerate", fmt.Sprint(fps), "-i", sc.Source)

	// Duration
	dur := sc.DurationSec
	if cfg.MaxSceneSec > 0 && dur > cfg.MaxSceneSec {
		dur = cfg.MaxSceneSec
	}
	totalFrames := int(math.Ceil(float64(fps) * dur))

	// Build filtergraph
	filters := []string{}

	// Ken Burns zoompan
	kb := sc.KenBurns
	if kb == nil {
		kb = &contract.KenBurns{ZoomFrom: 1.0, ZoomTo: 1.0, Pan: "none"}
	}
	if kb.ZoomFrom == 0 {
		kb.ZoomFrom = 1.0
	}
	if kb.ZoomTo == 0 {
		kb.ZoomTo = 1.0
	}

	zoomFrom := kb.ZoomFrom
	zoomTo := kb.ZoomTo

	// zoompan: zoom interpolates linearly from zoomFrom to zoomTo over totalFrames
	// panning based on direction
	panExpr := buildPanExpr(kb.Pan)
	zoomExpr := fmt.Sprintf("%f+(%f-%f)*on/%d", zoomFrom, zoomTo, zoomFrom, totalFrames)
	zoompanFilter := fmt.Sprintf("zoompan=z='%s':x='%s':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=%d",
		zoomExpr, panExpr, totalFrames, canvasW, canvasH, fps)
	filters = append(filters, zoompanFilter)

	// Caption via drawtext
	if sc.Caption != nil && sc.Caption.Text != "" && cfg.FontFile != "" {
		text := escapeDrawText(sc.Caption.Text)
		fa := fontFileArg(cfg.FontFile)
		fontSize := sc.Caption.FontSize
		if fontSize <= 0 {
			fontSize = 48
		}
		pos := "h-th-60" // bottom (60px from bottom)
		switch sc.Caption.Position {
		case "top":
			pos = "60"
		case "center":
			pos = "(h-th)/2"
		}
		dt := fmt.Sprintf("drawtext=%s:text='%s':fontsize=%d:fontcolor=white:borderw=2:bordercolor=black:x=(w-tw)/2:y=%s",
			fa, text, fontSize, pos)
		filters = append(filters, dt)
	}

	// Pixel format for compatibility
	filters = append(filters, "format=yuv420p")

	args = append(args, "-vf", strings.Join(filters, ","))

	// Output flags
	args = append(args, "-frames:v", fmt.Sprint(totalFrames))
	args = append(args, baseFlags...)
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, outputPath)
	return args
}

// buildVideoSceneArgs builds ffmpeg argv for a video scene (trim + scale to canvas).
func buildVideoSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}
	args = append(args, "-i", sc.Source)

	dur := sc.DurationSec
	if cfg.MaxSceneSec > 0 && dur > cfg.MaxSceneSec {
		dur = cfg.MaxSceneSec
	}

	// Scale + pixel format
	// scale to fit within canvas, maintaining aspect ratio
	scaleFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,format=yuv420p", canvasW, canvasH, canvasW, canvasH)

	// Apply mute if needed (no audio from source)
	if sc.Mute {
		args = append(args, "-an")
	}

	args = append(args, "-vf", scaleFilter)

	// Trim duration
	args = append(args, "-t", fmt.Sprintf("%.3f", dur))
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, baseFlags...)
	args = append(args, outputPath)
	return args
}

// buildColorSceneArgs builds ffmpeg argv for a solid color scene with optional caption.
func buildColorSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}

	dur := sc.DurationSec
	if cfg.MaxSceneSec > 0 && dur > cfg.MaxSceneSec {
		dur = cfg.MaxSceneSec
	}
	totalFrames := int(math.Ceil(float64(fps) * dur))

	// lavfi color source
	color := strings.TrimPrefix(sc.Color, "#")
	args = append(args, "-f", "lavfi", "-i",
		fmt.Sprintf("color=c=0x%s:s=%dx%d:d=%.3f:r=%d", color, canvasW, canvasH, dur, fps))

	// Caption via drawtext
	filters := []string{}
	if sc.Caption != nil && sc.Caption.Text != "" && cfg.FontFile != "" {
		text := escapeDrawText(sc.Caption.Text)
		fa := fontFileArg(cfg.FontFile)
		fontSize := sc.Caption.FontSize
		if fontSize <= 0 {
			fontSize = 48
		}
		pos := "h-th-60"
		switch sc.Caption.Position {
		case "top":
			pos = "60"
		case "center":
			pos = "(h-th)/2"
		}
		dt := fmt.Sprintf("drawtext=%s:text='%s':fontsize=%d:fontcolor=white:borderw=2:bordercolor=black:x=(w-tw)/2:y=%s",
			fa, text, fontSize, pos)
		filters = append(filters, dt)
	}

	filters = append(filters, "format=yuv420p")
	args = append(args, "-vf", strings.Join(filters, ","))
	args = append(args, "-frames:v", fmt.Sprint(totalFrames))
	args = append(args, baseFlags...)
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, outputPath)
	return args
}

// buildConcatArgs builds the ffmpeg concat demuxer command.
func buildConcatArgs(cfg FFmpegConfig, sceneFiles []string, concatFilePath, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}
	args = append(args, "-f", "concat", "-safe", "0", "-i", concatFilePath)
	args = append(args, "-c", "copy")
	args = append(args, "-y", outputPath)
	return args
}

// buildMixArgs builds the ffmpeg command that mixes narration + BGM audio into the video.
func buildMixArgs(cfg FFmpegConfig, videoPath, outputPath string, narrationFiles []string, bgmPath string, audioMix contract.AudioMix, fps int) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}

	// Input 0: the concat video (has no audio or silent audio)
	args = append(args, "-i", videoPath)

	inputIdx := 1

	// Narration inputs: concatenate them with delays
	narrIdx := -1
	if len(narrationFiles) > 0 {
		for _, nf := range narrationFiles {
			args = append(args, "-i", nf)
			inputIdx++
		}
		narrIdx = 1 // first narration input index
	}

	// BGM input
	bgmIdx := -1
	if bgmPath != "" {
		args = append(args, "-i", bgmPath)
		bgmIdx = inputIdx
		inputIdx++
	}

	// Build filter complex for audio mixing
	if narrIdx >= 0 || bgmIdx >= 0 {
		filters := []string{}
		audioInputs := []string{}

		if narrIdx >= 0 {
			// Concat all narration files into one stream
			if len(narrationFiles) == 1 {
				audioInputs = append(audioInputs, fmt.Sprintf("[%d:a]", narrIdx))
			} else {
				concatParts := ""
				for i := range narrationFiles {
					concatParts += fmt.Sprintf("[%d:a]", narrIdx+i)
				}
				n := len(narrationFiles)
				filters = append(filters, fmt.Sprintf("%sconcat=n=%d:v=0:a=1[narr]", concatParts, n))
				audioInputs = append(audioInputs, "[narr]")
			}
		}

		if bgmIdx >= 0 {
			vol := audioMix.BGMVolume
			if vol <= 0 {
				vol = 0.2
			}
			filters = append(filters, fmt.Sprintf("[%d:a]volume=%.2f[bgm]", bgmIdx, vol))
			audioInputs = append(audioInputs, "[bgm]")
		}

		// Mix all audio streams
		if len(audioInputs) > 1 {
			mixInputs := strings.Join(audioInputs, "")
			narVol := audioMix.NarrationVolume
			if narVol <= 0 {
				narVol = 1.0
			}
			filters = append(filters, fmt.Sprintf("%samix=inputs=%d:duration=first:dropout_transition=2,volume=%.2f[aout]",
				mixInputs, len(audioInputs), narVol))
		} else if len(audioInputs) == 1 {
			// Single audio source, just copy
			filters = append(filters, fmt.Sprintf("%sacopy[aout]", audioInputs[0]))
		}

		args = append(args, "-filter_complex", strings.Join(filters, ";"))
		args = append(args, "-map", "0:v", "-map", "[aout]")
	} else {
		// No audio — just copy video
		args = append(args, "-map", "0:v")
	}

	args = append(args, "-c:v", "copy")
	args = append(args, "-c:a", "aac", "-b:a", "128k")
	args = append(args, baseFlags...)
	args = append(args, "-y", outputPath)
	return args
}

// buildPanExpr returns the x-expression for pan in zoompan.
func buildPanExpr(pan string) string {
	switch pan {
	case "left":
		return "iw/2-(iw/zoom/2)-on*1"
	case "right":
		return "iw/2-(iw/zoom/2)+on*1"
	case "up":
		return "iw/2-(iw/zoom/2)"
	case "down":
		return "iw/2-(iw/zoom/2)"
	default: // "none" or empty
		return "iw/2-(iw/zoom/2)"
	}
}

// execFFmpeg runs an ffmpeg command and returns an error with stderr tail on failure.
func execFFmpeg(ctx context.Context, ffmpegPath string, args []string) error {
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Truncate output to last 500 chars for error message
		tail := string(output)
		if len(tail) > 500 {
			tail = tail[len(tail)-500:]
		}
		return fmt.Errorf("ffmpeg failed: %w\nstderr (tail): %s", err, tail)
	}
	return nil
}

// sceneOutputPath returns the temp file path for a rendered scene.
func sceneOutputPath(tempDir string, sceneIndex int) string {
	return filepath.Join(tempDir, fmt.Sprintf("scene_%03d.mp4", sceneIndex))
}
