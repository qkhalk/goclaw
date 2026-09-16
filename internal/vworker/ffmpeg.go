package vworker

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// FFmpegConfig holds the path to ffmpeg binary and rendering parameters.
type FFmpegConfig struct {
	FFmpegPath  string  // default "ffmpeg"
	FFProbePath string  // default "ffprobe"
	FontFile    string  // font file path for drawtext; empty = skip captions
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

// captionFilterValue returns the drawtext text source for a caption. With a
// tempDir it writes the text to a file and returns textfile=<path> — the only
// robust way to pass arbitrary caption text (commas, colons, quotes, unicode)
// through ffmpeg's two-level filtergraph escaping. The path is embedded in
// single quotes with quote escaping; the path comes from our own temp dir and
// contains none of the other filtergraph metacharacters.
func captionFilterValue(tempDir string, sceneIdx int, text string) (string, error) {
	if tempDir == "" {
		return "text='" + escapeDrawText(text) + "'", nil
	}
	p := filepath.Join(tempDir, fmt.Sprintf("caption_%03d.txt", sceneIdx))
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return "", fmt.Errorf("write caption file: %w", err)
	}
	quoted := strings.ReplaceAll(p, `'`, `'\''`)
	return "textfile='" + quoted + "'", nil
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

// buildImageSceneArgs builds ffmpeg argv for an image scene: a blurred,
// darkened cover-fit backdrop fills the canvas behind the contain-fit photo
// (landscape article photos keep their full frame instead of a hard center
// crop), a margin-bound eased Ken Burns rides on the composite, and the
// caption burns over it. Output: scene_N.mp4
func buildImageSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath, tempDir string, sceneIdx int) ([]string, error) {
	args := []string{"-hide_banner", "-loglevel", "warning"}

	// Input: still image looped for duration_sec
	args = append(args, "-loop", "1", "-framerate", fmt.Sprint(fps), "-i", sc.Source)

	// Duration
	dur := sc.DurationSec
	if cfg.MaxSceneSec > 0 && dur > cfg.MaxSceneSec {
		dur = cfg.MaxSceneSec
	}
	totalFrames := int(math.Ceil(float64(fps) * dur))

	// Ken Burns zoom/pan — pan travels inside the real zoom margin with a
	// smoothstep ease (accelerate, cruise, decelerate) instead of the old
	// 1px-per-frame linear drift that read as camera shake.
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
	if kb.Pan == "" {
		kb.Pan = "none"
	}
	// A pan without zoom headroom has zero margin to travel — give it some.
	if kb.Pan != "none" && kb.ZoomTo <= kb.ZoomFrom {
		kb.ZoomTo = kb.ZoomFrom + 0.08
	}

	xExpr, yExpr := buildPanExprs(kb.Pan, totalFrames)
	zoomExpr := fmt.Sprintf("%f+(%f-%f)*on/%d", kb.ZoomFrom, kb.ZoomTo, kb.ZoomFrom, totalFrames)
	zoompanFilter := fmt.Sprintf("zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d",
		zoomExpr, xExpr, yExpr, totalFrames, canvasW, canvasH, fps)

	// Composite: blurred cover backdrop (downscale→upscale is the cheap blur)
	// under the contain-fit foreground, then Ken Burns, then caption.
	var fc strings.Builder
	fmt.Fprintf(&fc, "[0:v]split=2[bg][fg];")
	fmt.Fprintf(&fc, "[bg]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,scale=%d:%d,eq=brightness=-0.12:saturation=1.15[bgf];",
		canvasW/8, canvasH/8, canvasW/8, canvasH/8, canvasW, canvasH)
	fmt.Fprintf(&fc, "[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgf];", canvasW, canvasH)
	fc.WriteString("[bgf][fgf]overlay=0:0[comp];")
	fmt.Fprintf(&fc, "[comp]%s[zp];", zoompanFilter)

	if sc.Caption != nil && sc.Caption.Text != "" && cfg.FontFile != "" {
		tv, err := captionFilterValue(tempDir, sceneIdx, sc.Caption.Text)
		if err != nil {
			return nil, err
		}
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
		fmt.Fprintf(&fc, "[zp]drawtext=%s:%s:fontsize=%d:fontcolor=white:borderw=2:bordercolor=black:x=(w-tw)/2:y=%s,format=yuv420p[vout];",
			fa, tv, fontSize, pos)
	} else {
		fc.WriteString("[zp]format=yuv420p[vout];")
	}

	args = append(args, "-filter_complex", strings.TrimSuffix(fc.String(), ";"))
	args = append(args, "-map", "[vout]")

	// Output flags
	args = append(args, "-frames:v", fmt.Sprint(totalFrames))
	args = append(args, baseFlags...)
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, outputPath)
	return args, nil
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

// buildColorSceneArgs builds ffmpeg argv for a color scene. With animated=true
// the flat color becomes a slowly drifting two-stop gradient derived from the
// scene color (much richer than a solid frame); the caller falls back to
// animated=false when the local ffmpeg lacks the gradients source.
func buildColorSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath, tempDir string, sceneIdx int, animated bool) ([]string, error) {
	args := []string{"-hide_banner", "-loglevel", "warning"}

	dur := sc.DurationSec
	if cfg.MaxSceneSec > 0 && dur > cfg.MaxSceneSec {
		dur = cfg.MaxSceneSec
	}
	totalFrames := int(math.Ceil(float64(fps) * dur))

	// lavfi source: animated gradient or flat color
	color := strings.TrimPrefix(sc.Color, "#")
	if animated {
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("gradients=s=%dx%d:c0=0x%s:c1=0x%s:speed=0.008:d=%.3f:r=%d",
				canvasW, canvasH, color, darkerHex(color), dur, fps))
	} else {
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("color=c=0x%s:s=%dx%d:d=%.3f:r=%d", color, canvasW, canvasH, dur, fps))
	}

	// Caption via drawtext
	filters := []string{}
	if sc.Caption != nil && sc.Caption.Text != "" && cfg.FontFile != "" {
		tv, err := captionFilterValue(tempDir, sceneIdx, sc.Caption.Text)
		if err != nil {
			return nil, err
		}
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
		dt := fmt.Sprintf("drawtext=%s:%s:fontsize=%d:fontcolor=white:borderw=2:bordercolor=black:x=(w-tw)/2:y=%s",
			fa, tv, fontSize, pos)
		filters = append(filters, dt)
	}

	filters = append(filters, "format=yuv420p")
	args = append(args, "-vf", strings.Join(filters, ","))
	args = append(args, "-frames:v", fmt.Sprint(totalFrames))
	args = append(args, baseFlags...)
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, outputPath)
	return args, nil
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
				var concatParts strings.Builder
				for i := range narrationFiles {
					concatParts.WriteString(fmt.Sprintf("[%d:a]", narrIdx+i))
				}
				n := len(narrationFiles)
				filters = append(filters, fmt.Sprintf("%sconcat=n=%d:v=0:a=1[narr]", concatParts.String(), n))
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

// buildPanExprs returns the zoompan x/y expressions for a pan direction.
// Motion stays inside the zoom crop margin (mx = iw-iw/zoom, my = ih-ih/zoom)
// and follows a smoothstep ease over totalFrames, so the camera glides
// instead of stepping a fixed pixel per frame. 45% of the margin is used —
// enough travel to feel alive, not enough to hit the frame edge.
func buildPanExprs(pan string, totalFrames int) (xExpr, yExpr string) {
	const center = "(iw-iw/zoom)/2"
	const centerV = "(ih-ih/zoom)/2"
	ease := fmt.Sprintf("(on/%d*on/%d*(3-2*on/%d))", totalFrames, totalFrames, totalFrames)
	switch pan {
	case "left":
		xExpr = fmt.Sprintf("%s-(iw-iw/zoom)*0.45*%s", center, ease)
		yExpr = centerV
	case "right":
		xExpr = fmt.Sprintf("%s+(iw-iw/zoom)*0.45*%s", center, ease)
		yExpr = centerV
	case "up":
		xExpr = center
		yExpr = fmt.Sprintf("%s-(ih-ih/zoom)*0.45*%s", centerV, ease)
	case "down":
		xExpr = center
		yExpr = fmt.Sprintf("%s+(ih-ih/zoom)*0.45*%s", centerV, ease)
	default: // "none" or empty
		xExpr = center
		yExpr = centerV
	}
	return xExpr, yExpr
}

// darkerHex halves each RGB channel of a 6-digit hex string — the second stop
// for the animated gradient of a color scene.
func darkerHex(hex string) string {
	out := make([]byte, 6)
	for i := 0; i < 3; i++ {
		v, _ := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		out[i*2] = hexDigit(v / 2 / 16)
		out[i*2+1] = hexDigit(v / 2 % 16)
	}
	return string(out)
}

func hexDigit(v uint64) byte {
	if v < 10 {
		return byte('0' + v)
	}
	return byte('a' + v - 10)
}

// xfadeTransition maps a storyboard transition to the ffmpeg xfade type.
// Unset junctions blend with a plain crossfade — xfade cannot hard-cut.
func xfadeTransition(transition string) string {
	switch transition {
	case "fade":
		return "fadeblack"
	case "slide_left":
		return "slideleft"
	case "slide_up":
		return "slideup"
	default: // crossfade, none, empty
		return "fade"
	}
}

// buildXfadeArgs chains the per-scene files with xfade transitions. offsets[i]
// is the absolute start time of scene i+1 in the OUTPUT timeline; the runner
// computes them from the pre-transition durations so narration alignment is
// preserved (each scene except the last renders transitionSec longer).
func buildXfadeArgs(cfg FFmpegConfig, sceneFiles []string, transitions []string, offsets []float64, fps int, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}
	for _, f := range sceneFiles {
		args = append(args, "-i", f)
	}

	var fc strings.Builder
	prev := "[0:v]"
	for i := 1; i < len(sceneFiles); i++ {
		out := fmt.Sprintf("[v%d]", i)
		fmt.Fprintf(&fc, "%s[%d:v]xfade=transition=%s:duration=%.2f:offset=%.3f%s;",
			prev, i, xfadeTransition(transitions[i]), transitionSec, offsets[i-1], out)
		prev = out
	}

	args = append(args, "-filter_complex", strings.TrimSuffix(fc.String(), ";"))
	args = append(args, "-map", prev)
	args = append(args, baseFlags...)
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, "-pix_fmt", "yuv420p")
	args = append(args, outputPath)
	return args
}

// buildXfadeChainArgs builds an xfade filter chain for N inputs where each
// offset is relative to the previous input's start (not the global timeline).
// This is used for batched xfade processing: each batch produces an
// intermediate file whose internal timeline starts at 0, so the offsets
// within the batch are relative.
func buildXfadeChainArgs(sceneFiles []string, transitions []string, offsets []float64, fps int, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}
	for _, f := range sceneFiles {
		args = append(args, "-i", f)
	}

	var fc strings.Builder
	prev := "[0:v]"
	absOffset := 0.0
	for i := 1; i < len(sceneFiles); i++ {
		out := fmt.Sprintf("[v%d]", i)
		fmt.Fprintf(&fc, "%s[%d:v]xfade=transition=%s:duration=%.2f:offset=%.3f%s;",
			prev, i, xfadeTransition(transitions[i]), transitionSec, absOffset+offsets[i-1], out)
		// Next offset is relative to the END of the current output segment
		absOffset += offsets[i-1]
		prev = out
	}

	args = append(args, "-filter_complex", strings.TrimSuffix(fc.String(), ";"))
	args = append(args, "-map", prev)
	args = append(args, baseFlags...)
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, "-pix_fmt", "yuv420p")
	args = append(args, outputPath)
	return args
}

// xfadeBatchSize is the maximum number of scenes processed in one ffmpeg
// xfade invocation. Larger batches hold more frame buffers simultaneously
// and can OOM on memory-constrained servers (e.g. 350 MB cgroup limit).
// With batch size 3, at most 3 input file handles + 2 chained xfade filter
// contexts are open at once — well within 350 MB at 720p.
const xfadeBatchSize = 3

// transitionSec is the xfade overlap at scene junctions (matches the browser
// preview's TRANSITION_SEC).
const transitionSec = 0.5

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
