package vworker

import (
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// TestBuildColorSceneArgs_LayersMixed covers the plain -vf path (text+shape
// layers, no image inputs): timed drawtext/drawbox with enable windows and
// defaults resolved (fill white, opacity 1, font 48, align center, box 0.1/0.1/0.8).
func TestBuildColorSceneArgs_LayersMixed(t *testing.T) {
	cfg := FFmpegConfig{FontFile: "/usr/share/fonts/NotoSans-Regular.ttf"}
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#0f172a",
		DurationSec: 6,
		Layers: []contract.Layer{
			{Kind: contract.LayerText, Text: "Sale 50%", Y: 0.2, FontSize: 72, Fill: "#FACC15"},
			{Kind: contract.LayerShape, Fill: "#000000", Opacity: 0.55, Start: 0.5, Duration: 3},
		},
	}

	tmp := t.TempDir()
	args, err := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_0.mp4", tmp, 0, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	vf := indexOfArg(args, "-vf")
	if vf < 0 {
		t.Fatal("expected -vf chain for non-image layers")
	}
	chain := args[vf+1]
	if !strings.Contains(chain, "enable='between(t,0.000,6.000)'") {
		t.Errorf("text layer missing full-scene enable window: %s", chain)
	}
	if !strings.Contains(chain, "enable='between(t,0.500,3.500)'") {
		t.Errorf("shape layer missing timed enable window: %s", chain)
	}
	if !strings.Contains(chain, "drawbox=x=108:y=192:w=864:h=576:color=0x000000@0.55:t=fill") {
		t.Errorf("shape defaults (x/y/w/h, opacity) not resolved: %s", chain)
	}
	if !strings.Contains(chain, "fontsize=72:fontcolor=0xFACC15@1.00") {
		t.Errorf("text style not resolved: %s", chain)
	}
	if !strings.Contains(chain, "x='108+(864-tw)/2'") {
		t.Errorf("centered align expr missing: %s", chain)
	}
	if strings.Contains(chain, "overlay=") {
		t.Errorf("no image layers — overlay must not appear: %s", chain)
	}
}

// TestBuildColorSceneArgs_ImageLayerSwitchesToComplex covers the
// filter_complex path: the layer image becomes an extra input, alpha-reduced
// and width-fitted, composited with an enable window.
func TestBuildColorSceneArgs_ImageLayerSwitchesToComplex(t *testing.T) {
	cfg := FFmpegConfig{}
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#101820",
		DurationSec: 4,
		Layers: []contract.Layer{
			{Kind: contract.LayerImage, Source: "/tmp/logo.png", X: 0.4, Y: 0.6, W: 0.2, Start: 1, Duration: 2},
		},
	}
	args, err := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_0.mp4", t.TempDir(), 0, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(args, "/tmp/logo.png") {
		t.Error("image layer input missing")
	}
	fcIdx := indexOfArg(args, "-filter_complex")
	if fcIdx < 0 {
		t.Fatal("expected -filter_complex for image layers")
	}
	fc := args[fcIdx+1]
	for _, want := range []string{
		"[1:v]format=rgba,colorchannelmixer=aa=1.00,scale=216:-1[li0]",
		"overlay=x='432+(216-w)/2':y=1152:enable='between(t,1.000,3.000)'",
		"-map", "[vout]",
	} {
		if !strings.Contains(fc, want) && !containsArg(args, want) {
			t.Errorf("filter_complex missing %q in %s", want, fc)
		}
	}
}

// TestBuildImageSceneArgs_LayersUnderCaption pins the layer ordering on image
// scenes: base → Ken Burns → layers → caption (caption stays topmost).
func TestBuildImageSceneArgs_LayersUnderCaption(t *testing.T) {
	cfg := FFmpegConfig{FontFile: "/fonts/f.ttf"}
	sc := contract.Scene{
		Type:        contract.SceneImage,
		Source:      "media/banner.png",
		DurationSec: 5,
		Caption:     &contract.Caption{Text: "Bottom", Position: "bottom"},
		Layers: []contract.Layer{
			{Kind: contract.LayerText, Text: "Top text"},
		},
	}
	args, err := buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/s.mp4", t.TempDir(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	fc := args[indexOfArg(args, "-filter_complex")+1]
	layerPos := strings.Index(fc, "drawtext")
	captionPos := strings.Index(fc, "textfile=")
	if layerPos < 0 || captionPos < 0 {
		t.Fatalf("expected both layer and caption drawtext in %s", fc)
	}
	if layerPos > captionPos {
		t.Errorf("layer must composite before the caption: %s", fc)
	}
}

// TestBuildVideoSceneArgs_LayersPlainVf covers video scenes with text/shape
// layers staying on the -vf path.
func TestBuildVideoSceneArgs_LayersPlainVf(t *testing.T) {
	sc := contract.Scene{
		Type:        contract.SceneVideo,
		Source:      "media/clip.mp4",
		DurationSec: 4,
		Mute:        true,
		Layers: []contract.Layer{
			{Kind: contract.LayerShape, Fill: "#FFFFFF", Opacity: 0.3, H: 0.1},
		},
	}
	args := buildVideoSceneArgs(FFmpegConfig{}, sc, 1080, 1920, 30, "/tmp/s.mp4", "", 0, 0)
	vf := args[indexOfArg(args, "-vf")+1]
	if !strings.Contains(vf, "drawbox=") || !strings.Contains(vf, "format=yuv420p") {
		t.Errorf("expected drawbox in the -vf chain tail: %s", vf)
	}
	if strings.Contains(vf, "overlay=") {
		t.Errorf("no image layers — overlay must not appear: %s", vf)
	}
}
