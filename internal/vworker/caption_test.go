package vworker

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// fontSetForTest extracts the embedded fonts into a temp dir.
func fontSetForTest(t *testing.T) FontSet {
	t.Helper()
	fs, err := ExtractFonts(t.TempDir())
	if err != nil {
		t.Fatalf("ExtractFonts: %v", err)
	}
	for _, p := range []string{fs.Display, fs.Body, fs.BodyBold, fs.Mono} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("font not extracted: %v", err)
		}
	}
	return fs
}

// TestRenderCaption_ChipKaraoke pins the karaoke geometry: a dim base strip
// plus one bright word overlay per word, word k revealed at k/N of the
// narration (the browser painter's exact rule).
func TestRenderCaption_ChipKaraoke(t *testing.T) {
	fs := fontSetForTest(t)
	tmp := t.TempDir()
	sc := contract.Scene{
		Type: contract.SceneColor, Color: "#0D1117", DurationSec: 5,
		Caption: &contract.Caption{
			Text: "SỰ THẬT VỀ OPEN SOURCE", Position: "center", FontSize: 64, Style: "chip",
		},
	}
	const narrSec = 4.0
	plan, err := renderCaption(sc, 1080, 1920, fs, narrSec, tmp, 0)
	if err != nil {
		t.Fatal(err)
	}
	words := 5 // "SỰ THẬT VỀ OPEN SOURCE"
	if len(plan.Overlays) != 1+words {
		t.Fatalf("expected 1 base + %d word overlays, got %d", words, len(plan.Overlays))
	}

	base := plan.Overlays[0]
	if base.RevealAt != 0 || base.FadeSec != 0.25 {
		t.Errorf("base strip must fade in at t=0: %+v", base)
	}
	for k := 1; k <= words; k++ {
		ov := plan.Overlays[k]
		want := float64(k-1) / float64(words) * narrSec
		if diff := ov.RevealAt - want; diff < -0.001 || diff > 0.001 {
			t.Errorf("word %d reveal = %.3f, want %.3f", k-1, ov.RevealAt, want)
		}
	}

	// Base PNG decodes and is centered horizontally on the canvas.
	f, err := os.Open(base.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("base PNG does not decode: %v", err)
	}
	if img.Bounds().Dx() >= 1080 {
		t.Errorf("strip must stay within the canvas, got width %d", img.Bounds().Dx())
	}
	if base.X < 0 || base.X+img.Bounds().Dx() > 1080 {
		t.Errorf("base strip placed off-canvas: x=%d w=%d", base.X, img.Bounds().Dx())
	}
	if base.Y < 0 || base.Y+img.Bounds().Dy() > 1920 {
		t.Errorf("base strip placed off-canvas: y=%d h=%d", base.Y, img.Bounds().Dy())
	}

	// Word overlays must sit inside the base strip's box (plus 1px slack).
	for k := 1; k <= words; k++ {
		ov := plan.Overlays[k]
		wf, err := os.Open(ov.Path)
		if err != nil {
			t.Fatal(err)
		}
		wimg, err := png.Decode(wf)
		wf.Close()
		if err != nil {
			t.Fatalf("word PNG %d does not decode: %v", k, err)
		}
		if ov.X < base.X-1 || ov.X+wimg.Bounds().Dx() > base.X+img.Bounds().Dx()+1 {
			t.Errorf("word %d outside the strip horizontally", k-1)
		}
		if ov.Y < base.Y-1 || ov.Y+wimg.Bounds().Dy() > base.Y+img.Bounds().Dy()+1 {
			t.Errorf("word %d outside the strip vertically", k-1)
		}
	}
}

// TestRenderCaption_PlainNoNarration: without narration the caption is a
// single full-color strip (no per-word overlays).
func TestRenderCaption_PlainNoNarration(t *testing.T) {
	fs := fontSetForTest(t)
	sc := contract.Scene{
		Type: contract.SceneColor, Color: "#101820", DurationSec: 3,
		Caption: &contract.Caption{Text: "Kết thúc", Position: "bottom"},
	}
	plan, err := renderCaption(sc, 1080, 1920, fs, 0, t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Overlays) != 1 {
		t.Fatalf("expected exactly 1 overlay, got %d", len(plan.Overlays))
	}
}

// TestBuildColorSceneArgs_VisualsV2 pins the v2 wiring: glow orbs drift via
// sin() overlay expressions, vignette + grain ride the chain, and the caption
// composites as fading PNG overlays.
func TestBuildColorSceneArgs_VisualsV2(t *testing.T) {
	fs := fontSetForTest(t)
	cfg := FFmpegConfig{Fonts: fs}
	sc := contract.Scene{
		Type: contract.SceneColor, Color: "#0D1117", Color2: "#1E293B",
		Grid: true, Glow: "#F97316", Vignette: true, Grain: true,
		DurationSec: 4,
		Caption: &contract.Caption{Text: "hook line", Position: "center", Style: "chip"},
	}
	tmp := t.TempDir()
	args, err := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/v2.mp4", tmp, 0, true, 3.0)
	if err != nil {
		t.Fatal(err)
	}

	fc := filterArg(args, "-filter_complex")
	if fc == "" {
		t.Fatal("visuals v2 scene must use filter_complex")
	}
	for _, want := range []string{
		"vignette=PI/4.6",
		"noise=alls=4:allf=t+u",
		"sin(t/5.3)",       // glow orb 1 drift
		"sin(t/6.1+2.2)",   // glow orb 2 drift
		"fade=t=in:st=0.000:d=0.25:alpha=1",  // base strip fade
		"enable='between(t,", // karaoke windows
		"[vs][cs0]overlay=", // caption chain starts after the style filters
		"[cc0][cs1]overlay=", // chained karaoke overlays reuse the prior label
		"[cc1][cs2]overlay=",
	} {
		if !strings.Contains(fc, want) {
			t.Errorf("filtergraph missing %q in: %s", want, fc)
		}
	}
	// A double-bracketed label is invalid ffmpeg syntax — the whole render
	// dies with "Invalid argument" before the first frame.
	if strings.Contains(fc, "[[") {
		t.Errorf("double-bracketed label in filtergraph: %s", fc)
	}
	// Glow + caption PNGs are real inputs.
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "glow_000.png") {
		t.Errorf("glow PNG input missing: %s", joined)
	}
	if !strings.Contains(joined, "cap_000_base.png") {
		t.Errorf("caption base PNG input missing: %s", joined)
	}
	// Karaoke: 2 words → 3 caption inputs (base + 2 bright words).
	if got := strings.Count(joined, "cap_000_w"); got != 2 {
		t.Errorf("expected 2 karaoke word inputs, found %d in: %s", got, joined)
	}
}

// TestBuildImageSceneArgs_CaptionOverlays: image scenes get the same caption
// overlay treatment.
func TestBuildImageSceneArgs_CaptionOverlays(t *testing.T) {
	fs := fontSetForTest(t)
	cfg := FFmpegConfig{Fonts: fs}
	sc := contract.Scene{
		Type: contract.SceneImage, Source: "media/a.jpg", DurationSec: 4,
		Caption: &contract.Caption{Text: "hello world", Position: "bottom"},
	}
	args, err := buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/s.mp4", t.TempDir(), 0, 2.0)
	if err != nil {
		t.Fatal(err)
	}
	fc := filterArg(args, "-filter_complex")
	if !strings.Contains(fc, "fade=t=in") || !strings.Contains(fc, "overlay=") {
		t.Errorf("caption overlays missing from image scene: %s", fc)
	}
	if strings.Contains(fc, "[[") {
		t.Errorf("double-bracketed label in filtergraph: %s", fc)
	}
	if strings.Contains(fc, "drawtext=") {
		t.Errorf("bundled fonts present — caption must not fall back to drawtext: %s", fc)
	}
}

func TestLooksLikeHTML(t *testing.T) {
	cases := map[string]bool{
		"<!DOCTYPE html><html>":            true,
		"<html><body>blocked</body>":       true,
		"\n\n<HTML>":                       true,
		"\x89PNG\r\n\x1a\n":                false,
		"\xff\xd8\xff\xe0":                 false,
		"":                                 false,
	}
	for head, want := range cases {
		if got := looksLikeHTML([]byte(head)); got != want {
			t.Errorf("looksLikeHTML(%q) = %v, want %v", head, got, want)
		}
	}
}

func TestRenderGlowPNG(t *testing.T) {
	p, err := renderGlowPNG(t.TempDir(), "#38BDF8", 7)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "glow_007.png" {
		t.Errorf("unexpected glow path %s", p)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("glow PNG does not decode: %v", err)
	}
	if img.Bounds().Dx() != 512 {
		t.Errorf("glow size = %d, want 512", img.Bounds().Dx())
	}
}

// TestRenderDims pins the output-resolution scaling: 720p means the short
// side renders at 720 (portrait 720×1280, landscape 1280×720), an output
// at or above the canvas never upscales, and dimensions stay even for
// yuv420p.
func TestRenderDims(t *testing.T) {
	cases := []struct {
		name             string
		canvasW, canvasH int
		outHeight        int
		wantW, wantH     int
	}{
		{"portrait 720p", 1080, 1920, 720, 720, 1280},
		{"landscape 720p", 1920, 1080, 720, 1280, 720},
		{"square 480p", 1080, 1080, 480, 480, 480},
		{"no downscale at 1080p", 1080, 1920, 1080, 1080, 1920},
		{"upscale refused", 720, 1280, 1080, 720, 1280},
		{"unset output defaults to 720 short side", 1080, 1920, 0, 720, 1280},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sb := &contract.Storyboard{
				Canvas: contract.Canvas{Width: c.canvasW, Height: c.canvasH, FPS: 30},
			}
			if c.outHeight > 0 {
				sb.Output = contract.Output{Height: c.outHeight}
			}
			w, h, fps := renderDims(sb)
			if w != c.wantW || h != c.wantH || fps != 30 {
				t.Errorf("renderDims() = %dx%d@%d, want %dx%d@30", w, h, fps, c.wantW, c.wantH)
			}
			if w%2 != 0 || h%2 != 0 {
				t.Errorf("renderDims() = %dx%d — dimensions must stay even", w, h)
			}
		})
	}
}

// TestBuildXfadeChainArgs pins that xfade offsets stay absolute — the old
// absOffset accumulation double-counted them, so the 2nd join landed past
// the first input's end and ffmpeg silently truncated every scene after
// the second (audio kept the full length: video 8.77s vs audio 11.26s).
func TestBuildXfadeChainArgs(t *testing.T) {
	args := buildXfadeChainArgs(
		[]string{"s0.mp4", "s1.mp4", "s2.mp4"},
		[]string{"none", "fade", "fade"},
		[]float64{4.262, 8.262},
		30, "/tmp/x.mp4")
	fc := filterArg(args, "-filter_complex")
	for _, want := range []string{
		"xfade=transition=fadeblack:duration=0.50:offset=4.262[v1]",
		"[v1][2:v]xfade=transition=fadeblack:duration=0.50:offset=8.262[v2]",
	} {
		if !strings.Contains(fc, want) {
			t.Errorf("filtergraph missing %q in: %s", want, fc)
		}
	}
	if strings.Contains(fc, "offset=12.524") {
		t.Errorf("offset was accumulated — must stay absolute: %s", fc)
	}
}
