package vworker

import (
	"context"
	"fmt"
	"log/slog"
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
	FontFile    string  // legacy drawtext font; empty = skip legacy captions
	MaxSceneSec float64 // cap per-scene duration for safety; 0 = no cap
	// Fonts routes the bundled display/body/mono faces. When Display is set,
	// captions render as PNG overlays (chip styles, karaoke reveal); text
	// layers prefer the bundled Inter over FontFile.
	Fonts FontSet
}

// common FFmpeg flags for "light" rendering.
// baseFlags keeps the encoder lean for tiny boxes: single-threaded encode
// and single-threaded filter graph (each filter_complex input spawns its
// own thread pool — with 7 inputs the pools alone can eat 100+ MB), and a
// short x264 lookahead so the encoder holds few reference frames.
var baseFlags = []string{"-preset", "veryfast", "-crf", "28",
	"-threads", "1", "-filter_threads", "1", "-filter_complex_threads", "1",
	"-x264-params", "rc-lookahead=8:ref=1:bframes=2", "-y"}

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

// --- timed overlay layers (storyboard layers[]) ---

// hasImageLayers reports whether any layer needs its own ffmpeg input.
// isPNGLayer reports whether a layer is composited from an extra image
// input (photo, embedded icon, card panel, stamp/cta pill, or a highlighted
// text rendered to PNG). The Source must already be materialized — an empty
// Source skips the layer instead of feeding ffmpeg a broken "-i ''".
func isPNGLayer(l contract.Layer) bool {
	if l.Source == "" {
		return false
	}
	switch l.Kind {
	case contract.LayerImage, contract.LayerIcon, contract.LayerCard,
		contract.LayerStamp, contract.LayerCTA:
		return true
	case contract.LayerText:
		return len(l.Highlights) > 0
	}
	return false
}

func hasImageLayers(sc contract.Scene) bool {
	for j := range sc.Layers {
		if isPNGLayer(sc.Layers[j]) {
			return true
		}
	}
	return false
}

// layerEnableExpr restricts a timeline-capable filter to the layer's visible
// window (scene-relative seconds).
func layerEnableExpr(l contract.Layer, sceneSec float64) string {
	end := l.Start + l.EffectiveDuration(sceneSec)
	return fmt.Sprintf("enable='between(t,%.3f,%.3f)'", l.Start, end)
}

// layerTextValue mirrors captionFilterValue for layer text (per-layer textfile
// keeps quoting safe for arbitrary content).
func layerTextValue(tempDir string, sceneIdx, layerIdx int, text string) (string, error) {
	if tempDir == "" {
		return "text='" + escapeDrawText(text) + "'", nil
	}
	p := filepath.Join(tempDir, fmt.Sprintf("layer_%03d_%02d.txt", sceneIdx, layerIdx))
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return "", fmt.Errorf("write layer text file: %w", err)
	}
	quoted := strings.ReplaceAll(p, `'`, `'''`)
	return "textfile='" + quoted + "'", nil
}

// layerXExpr positions the text inside the layer box per its align.
func layerXExpr(align string, x0, bw int) string {
	switch align {
	case "left":
		return fmt.Sprint(x0)
	case "right":
		return fmt.Sprintf("%d+%d-tw", x0, bw)
	default:
		return fmt.Sprintf("%d+(%d-tw)/2", x0, bw)
	}
}

// layerFontFor resolves a text layer's font path: display → bundled bold,
// mono → bundled monospace, body/empty → the layer default font.
func layerFontFor(l contract.Layer, fonts FontSet, fallback string) string {
	switch l.Font {
	case "display":
		if fonts.BodyBold != "" {
			return fonts.BodyBold
		}
	case "mono":
		if fonts.Mono != "" {
			return fonts.Mono
		}
	}
	return fallback
}

// layerInlineFilter renders one text/shape layer as a plain (chainable)
// filter — usable inside -vf and -filter_complex alike. Image layers go
// through appendLayerSteps (dual-input overlay).
func layerInlineFilter(sc contract.Scene, l contract.Layer, canvasW, canvasH int, tempDir string, sceneIdx, layerIdx int, fonts FontSet, fontFile string) (string, error) {
	_, opacity, fontSize, align := l.EffectiveStyle()
	x, y, w, _ := l.EffectiveBox()
	x0 := int(math.Round(x * float64(canvasW)))
	y0 := int(math.Round(y * float64(canvasH)))
	bw := int(math.Round(w * float64(canvasW)))
	en := layerEnableExpr(l, sc.DurationSec)
	switch l.Kind {
	case contract.LayerText:
		fontPath := layerFontFor(l, fonts, fontFile)
		if fontPath == "" {
			return "", nil // no font on the worker — skip like captions
		}
		tv, err := layerTextValue(tempDir, sceneIdx, layerIdx, l.Text)
		if err != nil {
			return "", err
		}
		hex := strings.ToUpper(strings.TrimPrefix(l.Fill, "#"))
		if hex == "" {
			// Style packs retheme the default text color (e.g. dark text on
			// paper_light); plain scenes keep white.
			hex = strings.ToUpper(strings.TrimPrefix(sceneTextColor(sc), "#"))
		}
		if hex == "" {
			hex = "FFFFFF"
		}
		// 0.3s alpha fade at the layer's start — text stops popping in.
		fade := fmt.Sprintf("alpha='min(1,(t-%.2f)/0.30)'", l.Start)
		// Slide entrances share the ease-out used by PNG layers; drawtext
		// values are quoted so the commas inside pow() parse safely.
		xe := fmt.Sprintf("'%s'", layerXExpr(align, x0, bw))
		ye := fmt.Sprint(y0)
		offX := int(math.Round(0.05 * float64(canvasW)))
		offY := int(math.Round(0.05 * float64(canvasH)))
		eo := easeOut(l.Start)
		switch l.Anim {
		case "up":
			ye = fmt.Sprintf("'%d+%d*%s'", y0, offY, eo)
		case "down":
			ye = fmt.Sprintf("'%d-%d*%s'", y0, offY, eo)
		case "left":
			xe = fmt.Sprintf("'%s+%d*%s'", layerXExpr(align, x0, bw), offX, eo)
		case "right":
			xe = fmt.Sprintf("'%s-%d*%s'", layerXExpr(align, x0, bw), offX, eo)
		}
		// Soft halo instead of a hard outline: half-strength border plus a
		// 2px drop shadow keeps light text readable over glow orbs.
		return fmt.Sprintf("drawtext=%s:%s:fontsize=%d:fontcolor=0x%s@%.2f:borderw=2:bordercolor=black@0.5:shadowcolor=black@0.35:shadowx=0:shadowy=2:x=%s:y=%s:%s:%s",
			fontFileArg(fontPath), tv, fontSize, hex, opacity, xe, ye, fade, en), nil
	case contract.LayerShape:
		hex := strings.ToUpper(strings.TrimPrefix(l.Fill, "#"))
		_, _, _, h := l.EffectiveBox()
		bh := int(math.Round(h * float64(canvasH)))
		return fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=0x%s@%.2f:t=fill:%s",
			x0, y0, bw, bh, hex, opacity, en), nil
	case contract.LayerCounter:
		return counterFilter(sc, l, canvasW, canvasH, tempDir, sceneIdx, layerIdx, fonts, fontFile)
	case contract.LayerToggleGrid:
		return toggleGridFilters(sc, l, canvasW, canvasH), nil
	case contract.LayerCompareBars:
		return compareBarsFilters(sc, l, canvasW, canvasH, tempDir, sceneIdx, layerIdx, fonts, fontFile)
	case contract.LayerStack:
		return stackFilters(sc, l, canvasW, canvasH, tempDir, sceneIdx, layerIdx, fonts, fontFile)
	case contract.LayerStamp, contract.LayerCTA:
		// PNG-overlay kinds — composited by appendLayerSteps after
		// prepareMotionAssets; nothing inline.
		return "", nil
	default:
		return "", fmt.Errorf("layer %d: kind %q has no inline filter", layerIdx, l.Kind)
	}
}

// appendLayerSteps writes the full layer section for -filter_complex flows:
// PNG layers (image/icon/card) chain entrance animation + alpha-reduce +
// width-fit scale, then overlay; text/shape layers stay inline. Returns the
// label holding the composited frame.
func appendLayerSteps(fc *strings.Builder, sc contract.Scene, canvasW, canvasH int, tempDir string, sceneIdx int, fonts FontSet, fontFile, cur string) string {
	nextInput := 1 // input 0 is the scene base
	for j := range sc.Layers {
		l := sc.Layers[j]
		en := layerEnableExpr(l, sc.DurationSec)
		out := fmt.Sprintf("ly%d", j)
		if isPNGLayer(l) {
			_, opacity, _, _ := l.EffectiveStyle()
			x, y, w, _ := l.EffectiveBox()
			x0 := int(math.Round(x * float64(canvasW)))
			y0 := int(math.Round(y * float64(canvasH)))
			bw := int(math.Round(w * float64(canvasW)))
			src := fmt.Sprintf("li%d", j)
			// Width-fitted (scale=w:-1), horizontally centered in the box —
			// mirrors the browser painter exactly.
			anim := layerAnimChain(l)
			sep := ""
			if anim != "" {
				sep = ","
			}
			fmt.Fprintf(fc, "[%d:v]format=rgba%s%s,colorchannelmixer=aa=%.2f,scale=%d:-1[%s];",
				nextInput, sep, anim, opacity, bw, src)
			ox, oy := layerOverlayPos(l, x0, y0, bw, canvasW, canvasH)
			fmt.Fprintf(fc, "[%s][%s]overlay=x=%s:y=%s:%s[%s];", cur, src, ox, oy, en, out)
			nextInput++
			cur = out
			continue
		}
		f, err := layerInlineFilter(sc, l, canvasW, canvasH, tempDir, sceneIdx, j, fonts, fontFile)
		if err != nil || f == "" {
			continue
		}
		fmt.Fprintf(fc, "[%s]%s[%s];", cur, f, out)
		cur = out
	}
	return cur
}

// animSec is the entrance-animation window shared by every anim kind.
const animSec = 0.45

// easeOut returns the ease-out-quad progress expression for a layer's
// entrance: 0 before Start, 1 after animSec.
func easeOut(start float64) string {
	return fmt.Sprintf("pow(1-clip((t-%.3f)/%.2f,0,1),2)", start, animSec)
}

// layerAnimChain returns the per-input filters implementing entrance
// animations that must run on the PNG layer stream itself (fade alpha,
// pop scale). Slide animations live in layerOverlayPos instead.
func layerAnimChain(l contract.Layer) string {
	switch l.Anim {
	case "fade":
		return fmt.Sprintf("fade=t=in:st=%.3f:d=0.30:alpha=1", l.Start)
	case "pop":
		return fmt.Sprintf("fade=t=in:st=%.3f:d=0.20:alpha=1,"+
			"scale=w='ceil(iw*(1+0.35*%s))':h=-2:eval=frame",
			l.Start, easeOut(l.Start))
	}
	return ""
}

// layerOverlayPos returns the overlay x/y expressions — static box centering
// plus the slide offset for up/down/left/right entrances.
func layerOverlayPos(l contract.Layer, x0, y0, bw, canvasW, canvasH int) (string, string) {
	xe := fmt.Sprintf("'%d+(%d-w)/2'", x0, bw)
	ye := fmt.Sprint(y0)
	offX := int(math.Round(0.06 * float64(canvasW)))
	offY := int(math.Round(0.06 * float64(canvasH)))
	eo := easeOut(l.Start)
	switch l.Anim {
	case "up":
		ye = fmt.Sprintf("'%d+%d*%s'", y0, offY, eo)
	case "down":
		ye = fmt.Sprintf("'%d-%d*%s'", y0, offY, eo)
	case "left": // enters from the right edge, moving left
		xe = fmt.Sprintf("'%d+(%d-w)/2+%d*%s'", x0, bw, offX, eo)
	case "right": // enters from the left edge, moving right
		xe = fmt.Sprintf("'%d+(%d-w)/2-%d*%s'", x0, bw, offX, eo)
	}
	return xe, ye
}

// appendImageLayerInputs adds the -i args for PNG layers (in layer order —
// must match appendLayerSteps' input numbering). Inputs are looped for the
// scene duration: a single-frame input can't animate (fade/scale need a
// timeline), and repeating frames cost nothing.
//
// The loop rate is deliberately far below the output fps: these are still
// images, and a 30fps loop pushes a full RGBA frame per tick into the
// filtergraph queues — on 512MB boxes that buffering alone exhausted swap
// and pushed renders into multi-minute thrash. 12fps keeps the pop scale
// and fades visually smooth while cutting queue growth ~3x; overlay x/y
// expressions are evaluated per output frame either way.
const pngLayerInputRate = 12

// glowInputRate applies to full-canvas glow PNGs — their drift is entirely
// overlay-expression-driven (per output frame), so they can loop far slower.
const glowInputRate = 2

// captionInputRate keeps caption fade steps smooth on small strip PNGs.
const captionInputRate = 10

func appendImageLayerInputs(args []string, sc contract.Scene, fps int, dur float64) []string {
	for j := range sc.Layers {
		if isPNGLayer(sc.Layers[j]) {
			args = append(args, "-loop", "1", "-framerate", fmt.Sprint(pngLayerInputRate),
				"-t", fmt.Sprintf("%.3f", dur), "-i", sc.Layers[j].Source)
		}
	}
	return args
}

// buildImageSceneArgs builds ffmpeg argv for an image scene: a blurred,
// darkened cover-fit backdrop fills the canvas behind the contain-fit photo
// (landscape article photos keep their full frame instead of a hard center
// crop), a margin-bound eased Ken Burns rides on the composite, and the
// caption renders as chip/karaoke PNG overlays. Output: scene_N.mp4
func buildImageSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath, tempDir string, sceneIdx int, narrSec float64) ([]string, error) {
	if err := prepareLayerAssets(&sc, canvasW, canvasH, tempDir, sceneIdx); err != nil {
		return nil, err
	}
	sc = resolveStylePack(sc)
	prepareMotionAssets(&sc, canvasW, canvasH, tempDir, sceneIdx, cfg.Fonts, layerFontFile(cfg))
	args := []string{"-hide_banner", "-loglevel", "warning"}

	// Input: still image looped for duration_sec
	args = append(args, "-loop", "1", "-framerate", fmt.Sprint(fps), "-i", sc.Source)

	// Duration
	dur := sc.DurationSec
	if cfg.MaxSceneSec > 0 && dur > cfg.MaxSceneSec {
		dur = cfg.MaxSceneSec
	}
	totalFrames := int(math.Ceil(float64(fps) * dur))

	if hasImageLayers(sc) {
		args = appendImageLayerInputs(args, sc, fps, dur)
	}

	// Caption overlays (PNG inputs appended after the image-layer inputs).
	plan := sceneCaptionPlan(cfg, sc, canvasW, canvasH, narrSec, tempDir, sceneIdx)
	glowPath := sceneGlowPath(tempDir, sc, sceneIdx)
	if glowPath != "" {
		args = append(args, "-loop", "1", "-framerate", fmt.Sprint(glowInputRate),
			"-t", fmt.Sprintf("%.3f", dur), "-i", glowPath)
	}
	args = appendCaptionInputs(args, plan, dur, fps)

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

	// zoompan crops at integer pixel offsets, which at delivery resolution
	// reads as camera shake. Supersampling the composited frame ×2 (small
	// canvases only, to bound memory) makes each crop step sub-pixel at
	// output scale.
	zpSrc := "comp"
	supersample := canvasW*canvasH <= 1<<20

	// Composite: blurred cover backdrop (downscale→upscale is the cheap blur)
	// under the contain-fit foreground, Ken Burns, v2 style filters, then the
	// timed layers and caption on top.
	var fc strings.Builder
	fmt.Fprintf(&fc, "[0:v]split=2[bg][fg];")
	fmt.Fprintf(&fc, "[bg]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,scale=%d:%d,eq=brightness=-0.12:saturation=1.15[bgf];",
		canvasW/8, canvasH/8, canvasW/8, canvasH/8, canvasW, canvasH)
	fmt.Fprintf(&fc, "[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgf];", canvasW, canvasH)
	fc.WriteString("[bgf][fgf]overlay=0:0[comp];")
	if supersample {
		fmt.Fprintf(&fc, "[comp]scale=%d:%d:flags=lanczos[css];", canvasW*2, canvasH*2)
		zpSrc = "css"
	}
	fmt.Fprintf(&fc, "[%s]%s[zp];", zpSrc, zoompanFilter)

	cur := "zp"
	for _, vf := range visualStyleFilters(sc) {
		fmt.Fprintf(&fc, "[%s]%s[vs];", cur, vf)
		cur = "vs"
	}
	if glowPath != "" {
		cur = appendGlowSteps(&fc, 1+countImageLayers(sc), cur, canvasW, canvasH)
	}
	if len(sc.Layers) > 0 {
		cur = appendLayerSteps(&fc, sc, canvasW, canvasH, tempDir, sceneIdx, cfg.Fonts, layerFontFile(cfg), cur)
	}
	cur = appendCaptionSteps(&fc, plan, 1+countImageLayers(sc)+boolInt(glowPath != ""), cur, dur)
	if plan == nil && sc.Caption != nil && sc.Caption.Text != "" && cfg.FontFile != "" {
		// No bundled fonts — legacy drawtext caption keeps renders alive.
		tv, err := captionFilterValue(tempDir, sceneIdx, sc.Caption.Text)
		if err != nil {
			return nil, err
		}
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
		fmt.Fprintf(&fc, "[%s]drawtext=%s:%s:fontsize=%d:fontcolor=white:borderw=2:bordercolor=black:x=(w-tw)/2:y=%s[cpl];",
			cur, fontFileArg(cfg.FontFile), tv, fontSize, pos)
		cur = "cpl"
	}
	fmt.Fprintf(&fc, "[%s]format=yuv420p[vout];", cur)

	args = append(args, "-filter_complex", strings.TrimSuffix(fc.String(), ";"))
	args = append(args, "-map", "[vout]")

	// Output flags
	args = append(args, "-frames:v", fmt.Sprint(totalFrames))
	args = append(args, baseFlags...)
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, outputPath)
	return args, nil
}

// countImageLayers returns the number of PNG layers (extra ffmpeg inputs).
func countImageLayers(sc contract.Scene) int {
	n := 0
	for j := range sc.Layers {
		if isPNGLayer(sc.Layers[j]) {
			n++
		}
	}
	return n
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// buildVideoSceneArgs builds ffmpeg argv for a video scene (trim + scale to
// canvas). Caption/glow overlays upgrade the render to a filter_complex
// graph; the plain -vf path stays for scenes without them.
func buildVideoSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath, tempDir string, sceneIdx int, narrSec float64) []string {
	if err := prepareLayerAssets(&sc, canvasW, canvasH, tempDir, sceneIdx); err != nil {
		slog.Warn("video layer assets failed", "scene", sceneIdx, "err", err)
	}
	sc = resolveStylePack(sc)
	prepareMotionAssets(&sc, canvasW, canvasH, tempDir, sceneIdx, cfg.Fonts, layerFontFile(cfg))
	args := []string{"-hide_banner", "-loglevel", "warning"}
	args = append(args, "-i", sc.Source)

	dur := sc.DurationSec
	if cfg.MaxSceneSec > 0 && dur > cfg.MaxSceneSec {
		dur = cfg.MaxSceneSec
	}
	if hasImageLayers(sc) {
		args = appendImageLayerInputs(args, sc, fps, dur)
	}

	plan := sceneCaptionPlan(cfg, sc, canvasW, canvasH, narrSec, tempDir, sceneIdx)
	glowPath := sceneGlowPath(tempDir, sc, sceneIdx)
	if glowPath != "" {
		args = append(args, "-loop", "1", "-framerate", fmt.Sprint(glowInputRate),
			"-t", fmt.Sprintf("%.3f", dur), "-i", glowPath)
	}
	args = appendCaptionInputs(args, plan, dur, fps)

	// Apply mute if needed (no audio from source)
	if sc.Mute {
		args = append(args, "-an")
	}

	// Scale + pixel format (format=yuv420p moves to the end of the chain when
	// overlays join, so overlay inputs stay rgba-capable until composite time)
	scalePrefix := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black", canvasW, canvasH, canvasW, canvasH)

	if plan != nil || glowPath != "" || hasImageLayers(sc) {
		// Overlays → filter_complex graph.
		var fc strings.Builder
		fmt.Fprintf(&fc, "[0:v]%s[base];", scalePrefix)
		cur := "base"
		for _, vf := range visualStyleFilters(sc) {
			fmt.Fprintf(&fc, "[%s]%s[vs];", cur, vf)
			cur = "vs"
		}
		if glowPath != "" {
			cur = appendGlowSteps(&fc, 1+countImageLayers(sc), cur, canvasW, canvasH)
		}
		if len(sc.Layers) > 0 {
			cur = appendLayerSteps(&fc, sc, canvasW, canvasH, tempDir, sceneIdx, cfg.Fonts, layerFontFile(cfg), cur)
		}
		cur = appendCaptionSteps(&fc, plan, 1+countImageLayers(sc)+boolInt(glowPath != ""), cur, dur)
		fmt.Fprintf(&fc, "[%s]format=yuv420p[vout];", cur)
		args = append(args, "-filter_complex", strings.TrimSuffix(fc.String(), ";"))
		args = append(args, "-map", "[vout]")
	} else {
		parts := []string{scalePrefix}
		parts = append(parts, visualStyleFilters(sc)...)
		layerFilters := make([]string, 0, len(sc.Layers))
		for j := range sc.Layers {
			f, err := layerInlineFilter(sc, sc.Layers[j], canvasW, canvasH, tempDir, sceneIdx, j, cfg.Fonts, layerFontFile(cfg))
			if err != nil {
				continue // video scenes keep rendering on a bad layer
			}
			if f != "" {
				layerFilters = append(layerFilters, f)
			}
		}
		parts = append(parts, layerFilters...)
		if sc.Caption != nil && sc.Caption.Text != "" && cfg.FontFile != "" && plan == nil {
			tv, err := captionFilterValue(tempDir, sceneIdx, sc.Caption.Text)
			if err == nil {
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
				parts = append(parts, fmt.Sprintf("drawtext=%s:%s:fontsize=%d:fontcolor=white:borderw=2:bordercolor=black:x=(w-tw)/2:y=%s",
					fontFileArg(cfg.FontFile), tv, fontSize, pos))
			}
		}
		parts = append(parts, "format=yuv420p")
		args = append(args, "-vf", strings.Join(parts, ","))
	}

	// Trim duration
	args = append(args, "-t", fmt.Sprintf("%.3f", dur))
	args = append(args, "-r", fmt.Sprint(fps))
	args = append(args, baseFlags...)
	args = append(args, outputPath)
	return args
}

// gradientStops resolves a color scene's two gradient stops: the scene color
// and its explicit second stop, or a darker shade when Color2 is unset.
func gradientStops(sc contract.Scene) (c0, c1 string) {
	c0 = strings.TrimPrefix(sc.Color, "#")
	c1 = strings.TrimPrefix(sc.Color2, "#")
	if c1 == "" {
		c1 = darkerHex(c0)
	}
	return c0, c1
}

// gridFilter overlays a faint blueprint grid over a color scene — the
// "developer dark-mode" backdrop. ~24 cells per edge, 1px lines.
func gridFilter(canvasW, canvasH int) string {
	cellW := canvasW / 24
	cellH := canvasH / 24
	if cellW < 1 {
		cellW = 1
	}
	if cellH < 1 {
		cellH = 1
	}
	return fmt.Sprintf("drawgrid=w=%d:h=%d:t=1:c=0x94A3B8@0.10", cellW, cellH)
}

// --- caption / visual-v2 compositing ---

// sceneCaptionPlan renders the caption PNGs for a scene (when the bundled
// fonts are available). Falls back to a nil plan — the builder then uses the
// legacy drawtext caption — when rendering the overlays fails, so a font
// hiccup can never kill a whole render.
func sceneCaptionPlan(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH int, narrSec float64, tempDir string, sceneIdx int) *captionPlan {
	if sc.Caption == nil || sc.Caption.Text == "" || cfg.Fonts.Display == "" {
		return nil
	}
	plan, err := renderCaption(sc, canvasW, canvasH, cfg.Fonts, narrSec, tempDir, sceneIdx)
	if err != nil {
		slog.Warn("caption overlay render failed, falling back to drawtext",
			"scene", sceneIdx, "error", err)
		return nil
	}
	return plan
}

// appendCaptionInputs adds the -i args for a caption plan's overlay PNGs
// (looped stills bounded to the scene length). Input order must match
// appendCaptionSteps' numbering.
func appendCaptionInputs(args []string, plan *captionPlan, dur float64, fps int) []string {
	if plan == nil {
		return args
	}
	for _, ov := range plan.Overlays {
		args = append(args, "-loop", "1", "-framerate", fmt.Sprint(captionInputRate),
			"-t", fmt.Sprintf("%.3f", dur), "-i", ov.Path)
	}
	return args
}

// appendCaptionSteps composites a caption plan over `cur`. firstIdx is the
// input index of the plan's first PNG (after the scene base and image-layer
// inputs).
func appendCaptionSteps(fc *strings.Builder, plan *captionPlan, firstIdx int, cur string, dur float64) string {
	if plan == nil {
		return cur
	}
	for k, ov := range plan.Overlays {
		in := firstIdx + k
		src := fmt.Sprintf("cs%d", k)
		fmt.Fprintf(fc, "[%d:v]format=rgba,fade=t=in:st=%.3f:d=%.2f:alpha=1[%s];", in, ov.RevealAt, ov.FadeSec, src)
		out := fmt.Sprintf("cc%d", k)
		en := ""
		if ov.RevealAt > 0 {
			en = fmt.Sprintf(":enable='between(t,%.3f,%.3f)'", ov.RevealAt, dur+0.5)
		}
		fmt.Fprintf(fc, "[%s][%s]overlay=x=%d:y=%d:format=auto:eval=init%s[%s];", cur, src, ov.X, ov.Y, en, out)
		cur = out
	}
	return cur
}

// appendGlowSteps composites two slowly drifting glow orbs (from one radial
// PNG, two scaled branches) over `cur`. The drift formulas are mirrored by
// the browser painter (render-shared.ts drawGlowOrbs) — keep them in sync.
func appendGlowSteps(fc *strings.Builder, inIdx int, cur string, canvasW, canvasH int) string {
	fmt.Fprintf(fc, "[%d:v]format=rgba,split=2[ga][gb];", inIdx)
	fmt.Fprintf(fc, "[ga]scale=%d:-1[g1];", int(float64(canvasW)*0.95))
	fmt.Fprintf(fc, "[gb]scale=%d:-1[g2];", int(float64(canvasW)*0.72))
	fmt.Fprintf(fc, "[%s][g1]overlay=x='%d*(0.26+0.10*sin(t/5.3))-w/2':y='%d*(0.26+0.05*cos(t/4.1))-h/2':format=auto:eval=frame[go1];", cur, canvasW, canvasH)
	fmt.Fprintf(fc, "[go1][g2]overlay=x='%d*(0.74+0.08*sin(t/6.1+2.2))-w/2':y='%d*(0.72+0.05*sin(t/5.0+1.0))-h/2':format=auto:eval=frame[go2];", canvasW, canvasH)
	return "go2"
}

// visualStyleFilters returns the chainable v2 style filters (vignette, film
// grain) applied under the text layers so type stays crisp.
func visualStyleFilters(sc contract.Scene) []string {
	var out []string
	if sc.Vignette {
		out = append(out, "vignette=PI/4.6")
	}
	if sc.Grain {
		out = append(out, "noise=alls=4:allf=t+u")
	}
	return out
}

// sceneGlowPath renders (once per scene) the radial glow PNG; empty when the
// scene has no glow.
func sceneGlowPath(tempDir string, sc contract.Scene, sceneIdx int) string {
	if sc.Glow == "" {
		return ""
	}
	p, err := renderGlowPNG(tempDir, sc.Glow, sceneIdx)
	if err != nil {
		slog.Warn("glow render failed, skipping orbs", "scene", sceneIdx, "error", err)
		return ""
	}
	return p
}

// layerFontFile picks the drawtext font for text layers: bundled Inter first,
// the legacy configured FontFile as fallback.
func layerFontFile(cfg FFmpegConfig) string {
	if cfg.Fonts.Body != "" {
		return cfg.Fonts.Body
	}
	return cfg.FontFile
}

// buildColorSceneArgs builds ffmpeg argv for a color scene. With animated=true
// the flat color becomes a slowly drifting two-stop gradient derived from the
// scene color (the caller falls back to animated=false when the local ffmpeg
// lacks the gradients source). Caption chip/karaoke overlays, glow orbs and
// the v2 style filters upgrade the render to a filter_complex graph; plain
// scenes keep the cheap -vf path.
func buildColorSceneArgs(cfg FFmpegConfig, sc contract.Scene, canvasW, canvasH, fps int, outputPath, tempDir string, sceneIdx int, animated bool, narrSec float64) ([]string, error) {
	if err := prepareLayerAssets(&sc, canvasW, canvasH, tempDir, sceneIdx); err != nil {
		return nil, err
	}
	sc = resolveStylePack(sc)
	prepareMotionAssets(&sc, canvasW, canvasH, tempDir, sceneIdx, cfg.Fonts, layerFontFile(cfg))
	args := []string{"-hide_banner", "-loglevel", "warning"}

	dur := sc.DurationSec
	if cfg.MaxSceneSec > 0 && dur > cfg.MaxSceneSec {
		dur = cfg.MaxSceneSec
	}
	totalFrames := int(math.Ceil(float64(fps) * dur))

	plan := sceneCaptionPlan(cfg, sc, canvasW, canvasH, narrSec, tempDir, sceneIdx)
	glowPath := sceneGlowPath(tempDir, sc, sceneIdx)

	// lavfi source: animated gradient or flat color
	c0, c1 := gradientStops(sc)
	lavfi := func() string {
		if animated {
			return fmt.Sprintf("gradients=s=%dx%d:c0=0x%s:c1=0x%s:speed=0.008:d=%.3f:r=%d",
				canvasW, canvasH, c0, c1, dur, fps)
		}
		return fmt.Sprintf("color=c=0x%s:s=%dx%d:d=%.3f:r=%d", c0, canvasW, canvasH, dur, fps)
	}

	if plan != nil || glowPath != "" || hasImageLayers(sc) {
		args = append(args, "-f", "lavfi", "-i", lavfi())
		if hasImageLayers(sc) {
			args = appendImageLayerInputs(args, sc, fps, dur)
		}
		if glowPath != "" {
			args = append(args, "-loop", "1", "-framerate", fmt.Sprint(fps),
				"-t", fmt.Sprintf("%.3f", dur), "-i", glowPath)
		}
		args = appendCaptionInputs(args, plan, dur, fps)

		var fc strings.Builder
		src := "0:v"
		if sc.Grid {
			fmt.Fprintf(&fc, "[0:v]%s[gridv];", gridFilter(canvasW, canvasH))
			src = "gridv"
		}
		cur := src
		if glowPath != "" {
			cur = appendGlowSteps(&fc, 1+countImageLayers(sc), cur, canvasW, canvasH)
		}
		for _, vf := range visualStyleFilters(sc) {
			fmt.Fprintf(&fc, "[%s]%s[vs];", cur, vf)
			cur = "vs"
		}
		if len(sc.Layers) > 0 {
			cur = appendLayerSteps(&fc, sc, canvasW, canvasH, tempDir, sceneIdx, cfg.Fonts, layerFontFile(cfg), cur)
		}
		cur = appendCaptionSteps(&fc, plan, 1+countImageLayers(sc)+boolInt(glowPath != ""), cur, dur)
		fmt.Fprintf(&fc, "[%s]format=yuv420p[vout];", cur)

		args = append(args, "-filter_complex", strings.TrimSuffix(fc.String(), ";"))
		args = append(args, "-map", "[vout]")
		args = append(args, "-frames:v", fmt.Sprint(totalFrames))
		args = append(args, baseFlags...)
		args = append(args, "-r", fmt.Sprint(fps))
		args = append(args, outputPath)
		return args, nil
	}

	// Plain path: single-input -vf chain.
	args = append(args, "-f", "lavfi", "-i", lavfi())

	filters := []string{}
	if sc.Grid {
		filters = append(filters, gridFilter(canvasW, canvasH))
	}
	filters = append(filters, visualStyleFilters(sc)...)
	for j := range sc.Layers {
		f, err := layerInlineFilter(sc, sc.Layers[j], canvasW, canvasH, tempDir, sceneIdx, j, cfg.Fonts, layerFontFile(cfg))
		if err != nil {
			return nil, err
		}
		if f != "" {
			filters = append(filters, f)
		}
	}
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
// NarrTrack pins one narration clip to its start time in the final timeline.
// Unlike back-to-back concatenation, each clip plays exactly when its scene
// is on screen — scenes without narration no longer shift later audio.
type NarrTrack struct {
	Path     string
	StartSec float64
}

func buildMixArgs(cfg FFmpegConfig, videoPath, outputPath string, narr []NarrTrack, bgmPath string, audioMix contract.AudioMix, fps int, videoDur float64) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}

	// Input 0: the concat video (has no audio or silent audio)
	args = append(args, "-i", videoPath)

	hasNarr := len(narr) > 0
	hasBGM := bgmPath != ""
	if !hasNarr && !hasBGM {
		// No audio — just copy video
		args = append(args, "-map", "0:v")
		args = append(args, "-c:v", "copy")
		args = append(args, baseFlags...)
		args = append(args, "-y", outputPath)
		return args
	}

	// Input 1: a silence base spanning the whole video. amix duration=first
	// then bounds the mix to the video length regardless of narration/BGM.
	dur := videoDur
	if dur <= 0 {
		dur = 1
	}
	args = append(args, "-f", "lavfi", "-t", fmt.Sprintf("%.3f", dur), "-i", "anullsrc=r=44100:cl=stereo")

	// Narration inputs
	narrBase := 2
	for _, nt := range narr {
		args = append(args, "-i", nt.Path)
	}

	// BGM input
	bgmIdx := -1
	if hasBGM {
		args = append(args, "-i", bgmPath)
		bgmIdx = narrBase + len(narr)
	}

	filters := []string{}
	audioInputs := []string{"[1:a]"} // silence base first (duration anchor)

	// Each narration clip is delayed to its scene's start on the timeline.
	for k, nt := range narr {
		delayMs := max(int(math.Round(nt.StartSec*1000)), 0)
		filters = append(filters, fmt.Sprintf("[%d:a]adelay=%d:all=1[an%d]", narrBase+k, delayMs, k))
		audioInputs = append(audioInputs, fmt.Sprintf("[an%d]", k))
	}

	if bgmIdx >= 0 {
		vol := audioMix.BGMVolume
		if vol <= 0 {
			vol = 0.2
		}
		filters = append(filters, fmt.Sprintf("[%d:a]volume=%.2f[bgm]", bgmIdx, vol))
		audioInputs = append(audioInputs, "[bgm]")
	}

	narVol := audioMix.NarrationVolume
	if narVol <= 0 {
		narVol = 1.0
	}
	filters = append(filters, fmt.Sprintf("%samix=inputs=%d:duration=first:dropout_transition=2,volume=%.2f[aout]",
		strings.Join(audioInputs, ""), len(audioInputs), narVol))

	args = append(args, "-filter_complex", strings.Join(filters, ";"))
	args = append(args, "-map", "0:v", "-map", "[aout]")
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
	for i := range 3 {
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
	for i := 1; i < len(sceneFiles); i++ {
		out := fmt.Sprintf("[v%d]", i)
		// offsets are already absolute on the accumulated timeline (batched
		// callers pass batch-relative values) — accumulating them here
		// double-counts and pushes later joins past the input's end, silently
		// truncating every scene after the second.
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
