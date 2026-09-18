package vworker

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

func mustChip(t *testing.T, name, hex string, size int) []byte {
	t.Helper()
	b, err := renderChipPNG(name, hex, size)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustCardBordered(t *testing.T, w, h, r int, hex string, opacity float64) []byte {
	t.Helper()
	b, err := renderCardPNG(w, h, r, hex, opacity, true)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestLayerAnimFilterGraph pins the entrance-animation wiring: PNG layers
// (card/icon) get looped inputs, per-anim fade/scale chains and eased overlay
// expressions; text layers get the eased drawtext y.
func TestLayerAnimFilterGraph(t *testing.T) {
	fs := fontSetForTest(t)
	cfg := FFmpegConfig{Fonts: fs}
	sc := contract.Scene{
		Type: contract.SceneColor, Color: "#0D1117", Color2: "#1E293B",
		DurationSec: 5,
		Layers: []contract.Layer{
			{Kind: contract.LayerCard, X: 0.1, Y: 0.3, W: 0.8, H: 0.3,
				Fill: "#1E293B", Opacity: 0.45, Radius: 0.03, Anim: "up", Start: 0.6},
			{Kind: contract.LayerIcon, Icon: "zap", X: 0.14, Y: 0.34, W: 0.1,
				Fill: "#FACC15", Anim: "pop", Start: 0.9},
			{Kind: contract.LayerText, Text: "Cộng đồng cùng xây", X: 0.28, Y: 0.38, W: 0.6,
				FontSize: 56, Fill: "#FFFFFF", Anim: "left", Start: 1.1},
		},
	}
	tmp := t.TempDir()
	args, err := buildColorSceneArgs(cfg, sc, 480, 852, 30, "/tmp/anim.mp4", tmp, 0, true, 3.0)
	if err != nil {
		t.Fatal(err)
	}

	// Three PNG-ish inputs total: lavfi base + card + icon (text is inline
	// drawtext, not an input).
	if got := countImageLayers(sc); got != 2 {
		t.Fatalf("countImageLayers = %d, want 2 (card + icon)", got)
	}
	joined := strings.Join(args, " ")
	for _, in := range []string{"-loop", "1", "-framerate", "30"} {
		if !strings.Contains(joined, in) {
			t.Fatalf("PNG layer inputs must be looped; missing %q in %s", in, joined)
		}
	}
	for _, src := range []string{"layer_card_0_0.png", "layer_icon_0_1.png"} {
		if !strings.Contains(joined, src) {
			t.Fatalf("missing materialized asset %q in args", src)
		}
	}

	fc := filterArg(args, "-filter_complex")
	for _, want := range []string{
		// card: slide-up eased overlay + constant alpha
		"[li0]",
		"overlay=x='48+(384-w)/2':y='256+51*pow(1-clip((t-0.600)/0.45,0,1),2)'",
		// icon: pop = quick fade + per-frame punch-in scale
		"[li1]",
		"fade=t=in:st=0.900:d=0.20:alpha=1",
		"scale=w='ceil(iw*(1+0.35*pow(1-clip((t-0.900)/0.45,0,1),2)))':h=-2:eval=frame",
		// text: eased drawtext y (left slide moves x)
		"drawtext=",
		"pow(1-clip((t-1.100)/0.45,0,1),2)",
	} {
		if !strings.Contains(fc, want) {
			t.Fatalf("filter_complex missing %q:\n%s", want, fc)
		}
	}
}

// TestKenBurnsSupersample pins the jitter fix: small canvases upscale the
// composited frame before zoompan (crop steps become sub-pixel).
func TestKenBurnsSupersample(t *testing.T) {
	cfg := FFmpegConfig{}
	sc := contract.Scene{
		Type: contract.SceneImage, Source: "media/a.jpg", DurationSec: 4,
		KenBurns: &contract.KenBurns{ZoomFrom: 1.0, ZoomTo: 1.12, Pan: "none"},
	}
	tmp := t.TempDir()
	args, err := buildImageSceneArgs(cfg, sc, 480, 852, 30, "/tmp/kb.mp4", tmp, 0, 3.0)
	if err != nil {
		t.Fatal(err)
	}
	fc := filterArg(args, "-filter_complex")
	if !strings.Contains(fc, "[comp]scale=960:1704:flags=lanczos[css];") {
		t.Fatalf("expected 2x supersample before zoompan:\n%s", fc)
	}
	if !strings.Contains(fc, "[css]zoompan=") {
		t.Fatalf("zoompan must consume the supersampled stream:\n%s", fc)
	}
	if !strings.Contains(fc, "s=480x852") {
		t.Fatalf("zoompan output must stay at delivery size:\n%s", fc)
	}

	// 1080p canvas is too memory-heavy to supersample — keep the direct path.
	args, err = buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/kb.mp4", tmp, 0, 3.0)
	if err != nil {
		t.Fatal(err)
	}
	fc = filterArg(args, "-filter_complex")
	if strings.Contains(fc, "flags=lanczos[css]") {
		t.Fatalf("1080p must not supersample:\n%s", fc)
	}
	if !strings.Contains(fc, "[comp]zoompan=") {
		t.Fatalf("1080p zoompan should read [comp] directly:\n%s", fc)
	}
}

// TestLayerAnimValidation rejects unknown anims and icon names.
func TestLayerAnimValidation(t *testing.T) {
	validate := func(sc contract.Scene) error {
		sb := contract.Storyboard{Version: 1, Scenes: []contract.Scene{sc}}
		return sb.Validate()
	}

	sc := contract.Scene{Type: contract.SceneColor, Color: "#000000", DurationSec: 4}

	sc1 := sc
	sc1.Layers = []contract.Layer{{Kind: contract.LayerIcon, Icon: "rocket", W: 0.2}}
	if err := validate(sc1); err == nil {
		t.Fatal("unknown icon must fail validation")
	}

	sc2 := sc
	sc2.Layers = []contract.Layer{{Kind: contract.LayerIcon, Icon: "zap", W: 0.2, Anim: "explode"}}
	if err := validate(sc2); err == nil {
		t.Fatal("unknown anim must fail validation")
	}

	sc3 := sc
	sc3.Layers = []contract.Layer{{Kind: contract.LayerCard, Fill: "#112233", W: 0.5, H: 0.2, Radius: 0.5}}
	if err := validate(sc3); err == nil {
		t.Fatal("radius > 0.2 must fail validation")
	}

	sc4 := sc
	sc4.Layers = []contract.Layer{{Kind: contract.LayerText, Text: "x", Font: "comic"}}
	if err := validate(sc4); err == nil {
		t.Fatal("unknown font must fail validation")
	}
}

// TestLayerPolishPins covers the visual-polish pass: icon chips, card
// borders and per-layer font selection.
func TestLayerPolishPins(t *testing.T) {
	t.Run("chip tile composes icon over tint", func(t *testing.T) {
		img, err := png.Decode(bytes.NewReader(mustChip(t, "zap", "FACC15", 96)))
		if err != nil {
			t.Fatal(err)
		}
		// Corner stays transparent (outside the rounded tile).
		if c := colorAt(t, img, 1, 1); c[3] != 0 {
			t.Fatalf("chip corner alpha = %d, want 0", c[3])
		}
		// The tinted tile is visible mid-edge (accent hue at low alpha) and
		// the glyph stroke stacks to near-full opacity somewhere inside.
		midEdge := colorAt(t, img, 48, 6)
		if midEdge[3] == 0 {
			t.Fatal("tile tint missing at mid-edge")
		}
		// Premultiplied channels keep the warm accent ordering (r > g > b).
		if !(midEdge[0] > midEdge[2] && midEdge[1] > midEdge[2]) {
			t.Fatalf("tile tint should carry the accent hue, got %v", midEdge[:3])
		}
		stroke := false
		for y := 20; y < 76 && !stroke; y++ {
			for x := 20; x < 76 && !stroke; x++ {
				if colorAt(t, img, x, y)[3] >= 200 {
					stroke = true
				}
			}
		}
		if !stroke {
			t.Fatal("icon stroke pixel (alpha >= 200) missing inside the tile")
		}
	})

	t.Run("card border boosts edge alpha", func(t *testing.T) {
		plain, err := png.Decode(bytes.NewReader(mustCard(t, 200, 100, 12, "1E293B", 0.2)))
		if err != nil {
			t.Fatal(err)
		}
		bordered, err := png.Decode(bytes.NewReader(mustCardBordered(t, 200, 100, 12, "1E293B", 0.2)))
		if err != nil {
			t.Fatal(err)
		}
		// The ring lives in the top 2px band; 30px deep is plain fill.
		if a1, a2 := colorAt(t, plain, 100, 1)[3], colorAt(t, bordered, 100, 1)[3]; a2 <= a1 {
			t.Fatalf("bordered edge alpha %d should exceed plain %d", a2, a1)
		}
		if colorAt(t, bordered, 100, 50)[3] == 0 {
			t.Fatal("bordered card center should stay filled")
		}
	})

	t.Run("display font selects the bold face", func(t *testing.T) {
		fs := fontSetForTest(t)
		cfg := FFmpegConfig{Fonts: fs}
		sc := contract.Scene{
			Type: contract.SceneColor, Color: "#0D1117", DurationSec: 4,
			// A caption forces the filter_complex path (plain scenes take -vf).
			Caption: &contract.Caption{Text: "x"},
			Layers: []contract.Layer{
				{Kind: contract.LayerText, Text: "Big", W: 0.8, Font: "display", FontSize: 56},
				{Kind: contract.LayerText, Text: "// eyebrow", W: 0.8, Y: 0.3, Font: "mono", FontSize: 30},
			},
		}
		tmp := t.TempDir()
		args, err := buildColorSceneArgs(cfg, sc, 480, 852, 30, "/tmp/p.mp4", tmp, 0, true, 3.0)
		if err != nil {
			t.Fatal(err)
		}
		fc := filterArg(args, "-filter_complex")
		if !strings.Contains(fc, fs.BodyBold) {
			t.Fatalf("display layer must use the bold face:\n%s", fc)
		}
		if !strings.Contains(fc, fs.Mono) {
			t.Fatalf("mono layer must use the monospace face:\n%s", fc)
		}
		// Soft halo instead of the hard black border.
		if !strings.Contains(fc, "bordercolor=black@0.5:shadowcolor=black@0.35") {
			t.Fatalf("text layers must carry the soft halo:\n%s", fc)
		}
	})
}
