package vworker

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
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

// buildImageSceneArgs builds ffmpeg argv for an image scene with optional Ken Burns and caption.
// Output: scene_N.mp4
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

	// Per-scene transform, color grade and opacity — appended after zoompan
	// (Ken Burns bakes into the fitted frame first, then fx, then caption),
	// mirroring the web editor's canvas draw order.
	if fx := transformFilters(sc.Transform, canvasW, canvasH); fx != "" {
		filters = append(filters, fx)
	}
	if fx := colorFilterFilters(sc.Filter); fx != "" {
		filters = append(filters, fx)
	}
	if fx := opacityFilters(sc.Transform); fx != "" {
		filters = append(filters, fx)
	}

	// Caption via drawtext
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
		dt := fmt.Sprintf("drawtext=%s:%s:fontsize=%d:fontcolor=white:borderw=2:bordercolor=black:x=(w-tw)/2:y=%s",
			fa, tv, fontSize, pos)
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
	// scale to fit within canvas, maintaining aspect ratio; per-scene fx sits
	// between the fit stage and the final pixel-format conversion.
	filters := []string{
		fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black", canvasW, canvasH, canvasW, canvasH),
	}
	if fx := transformFilters(sc.Transform, canvasW, canvasH); fx != "" {
		filters = append(filters, fx)
	}
	if fx := colorFilterFilters(sc.Filter); fx != "" {
		filters = append(filters, fx)
	}
	if fx := opacityFilters(sc.Transform); fx != "" {
		filters = append(filters, fx)
	}
	filters = append(filters, "format=yuv420p")

	// Apply mute if needed (no audio from source)
	if sc.Mute {
		args = append(args, "-an")
	}

	args = append(args, "-vf", strings.Join(filters, ","))

	// Trim duration
	args = append(args, "-t", fmt.Sprintf("%.3f", dur))
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, baseFlags...)
	args = append(args, outputPath)
	return args
}

// buildColorSceneArgs builds ffmpeg argv for a solid color scene with optional caption.
func buildColorSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath, tempDir string, sceneIdx int) ([]string, error) {
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

// clampF clamps v into [lo, hi].
func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// transformFilters renders the OpenCut-style per-scene transform as ffmpeg
// filter fragments. Uniform scale and rotation around the frame center
// commute, so the chain is: rotate (around center, black corners) → scale →
// pad a bleed margin → crop the canvas-sized window back out at the offset
// position. That pad+crop combo handles both grow (crop) and shrink (pad)
// exactly like the web editor's canvas draw (black letterbox, clipped
// overflow). Returns "" for identity so neutral scenes keep a clean
// filtergraph.
func transformFilters(t *contract.Transform, canvasW, canvasH int) string {
	if t == nil {
		return ""
	}
	var parts []string
	// Defensive clamps mirror the validation bounds so a drifted caller can
	// never produce a giant pad allocation or an Inf filter parameter.
	rot := clampF(t.Rotate, -360, 360)
	if rot != 0 {
		parts = append(parts, fmt.Sprintf("rotate=%.6f:c=black", rot*(math.Pi/180)))
	}
	s := t.Scale
	if s == 0 {
		s = 1 // omitempty: unset == identity
	}
	s = clampF(s, 0.01, 16)
	dx := clampF(t.X, -100, 100) / 100 * float64(canvasW)
	dy := clampF(t.Y, -100, 100) / 100 * float64(canvasH)
	if s == 1 && dx == 0 && dy == 0 {
		return strings.Join(parts, ",")
	}

	// Even-dimension content size after scaling (source frames are WxH).
	cw := 2 * int(float64(canvasW)*s/2)
	ch := 2 * int(float64(canvasH)*s/2)
	if cw < 2 {
		cw = 2
	}
	if ch < 2 {
		ch = 2
	}
	if cw != canvasW || ch != canvasH {
		parts = append(parts, fmt.Sprintf("scale=%d:%d", cw, ch))
	}
	// Black frame large enough to hold the content, the canvas-sized crop
	// window, AND the full x/y shift on either side — the window below then
	// never clamps, so offsets stay exact. Content is centered in it.
	bleedX := 2 * int(math.Ceil(math.Abs(dx)))
	bleedY := 2 * int(math.Ceil(math.Abs(dy)))
	padW := max(cw, canvasW) + bleedX
	padH := max(ch, canvasH) + bleedY
	parts = append(parts, fmt.Sprintf("pad=%d:%d:%d:%d:black", padW, padH, (padW-cw)/2, (padH-ch)/2))
	// Crop window whose top-left sits so the content center lands at
	// canvas center + (dx, dy) — the web editor's translate+rotate+scale pivot.
	x := clampF(float64(padW)/2-(float64(canvasW)/2+dx), 0, float64(padW-canvasW))
	y := clampF(float64(padH)/2-(float64(canvasH)/2+dy), 0, float64(padH-canvasH))
	parts = append(parts, fmt.Sprintf("crop=%d:%d:%d:%d", canvasW, canvasH, int(math.Round(x)), int(math.Round(y))))
	return strings.Join(parts, ",")
}

// colorFilterFilters maps the CSS-style color grade onto ffmpeg filters:
// brightness/contrast/saturation via eq, blur via gblur. CSS brightness(k)
// is multiplicative while eq's is additive around black, hence the k-1
// mapping (k=0 → -1 black, k=1 → 0 neutral, k=2 → +1); the mapping is exact
// at 0/1/2 and an accepted v1 approximation in midtones (the web canvas
// preview is multiplicative). All fields use omitempty semantics: 0 means
// unset, so meaningful CSS zeros (brightness=0 black, saturate=0 grayscale)
// are not representable on this contract. Returns "" when the grade is
// identity so neutral scenes keep a clean filtergraph.
func colorFilterFilters(f *contract.Filter) string {
	if f == nil {
		return ""
	}
	var eq []string
	if b := f.Brightness; b != 0 && b != 1 {
		eq = append(eq, fmt.Sprintf("brightness=%.4f", clampF(b-1, -1, 1)))
	}
	if c := f.Contrast; c != 0 && c != 1 {
		eq = append(eq, fmt.Sprintf("contrast=%.4f", clampF(c, 0, 3)))
	}
	if sa := f.Saturate; sa != 0 && sa != 1 {
		eq = append(eq, fmt.Sprintf("saturation=%.4f", clampF(sa, 0, 3)))
	}
	parts := make([]string, 0, 2)
	if len(eq) > 0 {
		parts = append(parts, "eq="+strings.Join(eq, ":"))
	}
	if f.Blur > 0 {
		parts = append(parts, fmt.Sprintf("gblur=sigma=%.3f", clampF(f.Blur, 0, 100)))
	}
	return strings.Join(parts, ",")
}

// opacityFilters flattens the scene toward black at the given alpha. Rendered
// scene MP4s have no alpha channel, so opacity composites over black — the
// same result as the client recording a transparent canvas. Implemented as a
// uniform RGB channel multiply (exact alpha-over-black math). Returns "" when
// opaque. Omitempty semantics: 0 on the wire means unset, i.e. fully opaque —
// only a value strictly inside (0,1) flattens.
func opacityFilters(t *contract.Transform) string {
	if t == nil || t.Opacity <= 0 || t.Opacity >= 1 {
		return ""
	}
	a := t.Opacity
	return fmt.Sprintf("format=rgba,colorchannelmixer=rr=%.4f:gg=%.4f:bb=%.4f", a, a, a)
}

// Transition timing. The wire contract carries only the transition type (the
// web editor uses a fixed TRANSITION_SEC enter window); the clamp bounds keep
// offsets valid if a per-scene duration field is added to the contract later.
const (
	defaultTransitionSec = 0.5
	minTransitionSec     = 0.1
	maxTransitionSec     = 2.0
)

// clampTransitionSec bounds a transition duration to [0.1, 2.0]s.
func clampTransitionSec(d float64) float64 {
	if d < minTransitionSec {
		return minTransitionSec
	}
	if d > maxTransitionSec {
		return maxTransitionSec
	}
	return d
}

// xfadeTransition maps a contract transition constant to the ffmpeg xfade
// transition name. "fade" becomes fadeblack (the editor fades the incoming
// scene in from black; xfade's plain "fade" is a dissolve, used for
// "crossfade"). ok=false marks a hard cut (none/unset).
func xfadeTransition(t string) (name string, ok bool) {
	switch t {
	case contract.TransitionFade:
		return "fadeblack", true
	case contract.TransitionCrossfade:
		return "fade", true
	case contract.TransitionSlideLeft:
		return "slideleft", true
	case contract.TransitionSlideUp:
		return "slideup", true
	default: // none, "" — anything else is rejected by Validate()
		return "", false
	}
}

// anyEnterTransition reports whether any scene after the first declares an
// enter transition (scene i's transition decorates the edge i-1 → i).
func anyEnterTransition(scenes []contract.Scene) bool {
	for i := 1; i < len(scenes); i++ {
		if _, ok := xfadeTransition(scenes[i].Transition); ok {
			return true
		}
	}
	return false
}

// effectiveSceneDur returns the duration a scene was actually rendered with
// (DurationSec capped by MaxSceneSec, mirroring the per-scene builders) —
// xfade offsets must be computed against rendered lengths, not requested ones.
func effectiveSceneDur(cfg FFmpegConfig, sc contract.Scene) float64 {
	d := sc.DurationSec
	if cfg.MaxSceneSec > 0 && d > cfg.MaxSceneSec {
		d = cfg.MaxSceneSec
	}
	return d
}

// buildTransitionArgs builds the ffmpeg command that joins the rendered scene
// files with their enter transitions in a single filter_complex pass: xfade
// for fade/crossfade/slide edges, concat for hard cuts, chained left to
// right. Inputs are normalized (timebase, fps, square SAR) because xfade
// requires frame-aligned streams with identical timebases.
//
// Offset math: cursor is the timeline position where the current scene starts
// inside the chained output stream. A transition of length T starting at
// cursor+dur-T overlaps T seconds of the tail, so the next scene begins at
// that offset in the new stream; a hard cut simply advances the cursor by
// dur. For scenes d0,d1,d2 with transitions on both edges the offsets are
// d0-T and d0+d1-2T.
func buildTransitionArgs(cfg FFmpegConfig, scenes []contract.Scene, sceneFiles []string, outputPath string, fps int) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}
	for _, f := range sceneFiles {
		args = append(args, "-i", f)
	}

	var g strings.Builder
	for i := range sceneFiles {
		// tpad holds the last frame if a rendered file EOFs early (a short
		// video source trimmed with -t can end before its declared duration),
		// keeping xfade offsets valid; it is a no-op for full-length files.
		fmt.Fprintf(&g, "[%d:v]settb=AVTB,fps=%d,setsar=1,tpad=stop_mode=clone:stop_duration=%.3f[v%d];", i, fps, effectiveSceneDur(cfg, scenes[i]), i)
	}

	cursor := 0.0
	prev := "[v0]"
	for i := 1; i < len(sceneFiles); i++ {
		dur := effectiveSceneDur(cfg, scenes[i-1])
		dNext := effectiveSceneDur(cfg, scenes[i])
		t := clampTransitionSec(defaultTransitionSec)
		t = math.Min(t, 0.8*math.Min(dur, dNext)) // never consume a whole short scene
		out := fmt.Sprintf("[vx%d]", i)
		if name, ok := xfadeTransition(scenes[i].Transition); ok {
			offset := cursor + dur - t
			fmt.Fprintf(&g, "%s[v%d]xfade=transition=%s:duration=%.3f:offset=%.3f%s;", prev, i, name, t, offset, out)
			cursor = offset
		} else {
			fmt.Fprintf(&g, "%s[v%d]concat=n=2:v=1:a=0%s;", prev, i, out)
			cursor += dur
		}
		prev = out
	}
	fmt.Fprintf(&g, "%sformat=yuv420p[vout]", prev)

	args = append(args, "-filter_complex", g.String())
	args = append(args, "-map", "[vout]")
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, baseFlags...)
	args = append(args, outputPath)
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
