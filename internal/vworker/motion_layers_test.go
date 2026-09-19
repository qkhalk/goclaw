package vworker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// TestCounterFilterArgs pins the counter drawtext: eif expansion, count-up
// expression, enable window, pack-aware default color.
func TestCounterFilterArgs(t *testing.T) {
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#0f172a",
		StylePack:   "tech_dark",
		DurationSec: 6,
		Layers: []contract.Layer{
			{Kind: contract.LayerCounter, Text: "▲ ", From: 0.5, To: 20, Suffix: " tỷ", Start: 0.5, Duration: 3, Y: 0.3, FontSize: 96},
		},
	}
	f, err := layerInlineFilter(sc, sc.Layers[0], 1080, 1920, "", 0, 0, FontSet{}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%{eif",                        // time-based expansion (colon is option-escaped inline)
		"0.5000+(20.0000-0.5000)",      // from + (to-from)*…
		"(t-0.500)/3.000",              // linear progress over the window
		"fontcolor=0x38BDF8@1.00",      // tech_dark accent as the default color
		"fontsize=96",
		"enable='between(t,0.500,3.500)'",
	} {
		if !strings.Contains(f, want) {
			t.Errorf("counter filter missing %q:\n%s", want, f)
		}
	}
	// File mode writes the expansion to a textfile — read it back.
	tmp := t.TempDir()
	f, err = layerInlineFilter(sc, sc.Layers[0], 1080, 1920, tmp, 0, 0, FontSet{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f, "textfile='") {
		t.Errorf("file-mode counter missing textfile: %s", f)
	}
	start := strings.Index(f, "textfile='") + len("textfile='")
	end := strings.Index(f[start:], "'")
	data, err := os.ReadFile(f[start : start+end])
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"▲ ",                       // prefix
		"%{eif:",                   // expansion function
		":d}",                      // eif decimal format (d/u/x/X are the only valid formats)
		" tỷ",                      // suffix
		"min(1,max(0,(t-0.500)/3.000))", // linear count window
	} {
		if !strings.Contains(content, want) {
			t.Errorf("counter textfile missing %q in %q", want, content)
		}
	}
}

// TestCounterValueExpansionDecimalFormats pins the eif digit-splitting: the
// eif format only accepts d/u/x/X on every ffmpeg we deploy (no decimal
// count), so decimals come from scale+split expansions.
func TestCounterValueExpansionDecimalFormats(t *testing.T) {
	expr := "0.5000+(20.0000-0.5000)*min(1,max(0,(t-0.500)/3.000))"
	cases := []struct {
		decimals int
		count    int // number of eif expansions in the output
		want     string
	}{
		{0, 1, "%{eif:round(" + expr + "):d}"},
		{1, 2, ""},
		{2, 3, ""},
		{5, 3, ""}, // clamped to 2
	}
	for _, tc := range cases {
		got := counterValueExpansion(expr, tc.decimals)
		if n := strings.Count(got, "%{eif:"); n != tc.count {
			t.Errorf("decimals=%d: %d eif expansions, want %d: %s", tc.decimals, n, tc.count, got)
		}
		if strings.Count(got, ":d}") != tc.count {
			t.Errorf("decimals=%d: every expansion must use :d — %s", tc.decimals, got)
		}
		if tc.want != "" && got != tc.want {
			t.Errorf("decimals=0 mismatch: %s", got)
		}
		if tc.decimals >= 1 && !strings.Contains(got, "*10") {
			t.Errorf("decimals=%d must scale by a power of ten: %s", tc.decimals, got)
		}
	}
}

// TestToggleGridFiltersArgs pins the per-cell drawbox pairs and the
// arithmetic on/off enable expressions.
func TestToggleGridFiltersArgs(t *testing.T) {
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#0f172a",
		DurationSec: 6,
		Layers: []contract.Layer{
			{Kind: contract.LayerToggleGrid, Cols: 3, Rows: 3, Cadence: 0.6, Y: 0.55, W: 0.5, Start: 0.8, Duration: 4, Fill: "#22C55E"},
		},
	}
	f, err := layerInlineFilter(sc, sc.Layers[0], 1080, 1920, "", 0, 0, FontSet{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(f, "drawbox="); got != 18 { // 9 cells × (off + on)
		t.Errorf("toggle_grid drawbox count = %d, want 18:\n%s", got, f)
	}
	for _, want := range []string{
		"0x22C55E@1.00", // on color from fill
		"0x334155@",     // default off color
		"mod(0*31+0*17+floor((t-0.800)/0.600)*7,5)",
		"enable='lt(", // on-state expression
		"enable='gte(", // off-state expression
	} {
		if !strings.Contains(f, want) {
			t.Errorf("toggle_grid filter missing %q:\n%s", want, f)
		}
	}
	// Shared schedule sanity: the Go-side mirror matches the expression —
	// (c*31+r*17+step*7) mod 5: (0,0,0)→0 on, (0,0,2)→14 mod 5 = 4 off.
	if !toggleOn(0, 0, 0) || toggleOn(0, 0, 2) {
		t.Error("toggleOn schedule drifted from its documented formula")
	}
}

// TestCompareBarsFiltersArgs pins the tracks, stepped growth and labels.
func TestCompareBarsFiltersArgs(t *testing.T) {
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#0f172a",
		DurationSec: 6,
		Layers: []contract.Layer{
			{Kind: contract.LayerCompareBars, LabelA: "CPU", LabelB: "GPU", WidthA: 0.7, WidthB: 0.4, Y: 0.5, Start: 1, Duration: 3},
		},
	}
	f, err := layerInlineFilter(sc, sc.Layers[0], 1080, 1920, "", 0, 0, FontSet{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(f, "drawbox="); got != 18 { // 2 tracks + 2×8 growth steps
		t.Errorf("compare_bars drawbox count = %d, want 18:\n%s", got, f)
	}
	for _, want := range []string{
		"0x38BDF8@1.00", // bar A default (accent fallback)
		"0xF97316@1.00", // bar B default
		"0x94A3B8@0.25", // dim track
		"drawtext=",     // labels
		"enable='between(t,1.000,4.000)'",
	} {
		if !strings.Contains(f, want) {
			t.Errorf("compare_bars filter missing %q:\n%s", want, f)
		}
	}
}

// TestStackFiltersArgs pins the slab count, staggered enable windows and
// label drawtexts.
func TestStackFiltersArgs(t *testing.T) {
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#0f172a",
		DurationSec: 6,
		Layers: []contract.Layer{
			{Kind: contract.LayerStack, N: 3, Labels: []string{"L1", "L2", "L3"}, Fill: "#38BDF8", FillB: "#8B5CF6", Y: 0.3, Start: 0.4},
		},
	}
	f, err := layerInlineFilter(sc, sc.Layers[0], 1080, 1920, "", 0, 0, FontSet{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(f, "drawbox="); got != 9 { // 3 slabs × 3 slide steps
		t.Errorf("stack drawbox count = %d, want 9:\n%s", got, f)
	}
	if got := strings.Count(f, "drawtext="); got != 3 {
		t.Errorf("stack label count = %d, want 3:\n%s", got, f)
	}
	for _, want := range []string{
		"0x38BDF8@1.00", // slab 0 = gradient start
		"0x8B5CF6@1.00", // slab n-1 = gradient end
	} {
		if !strings.Contains(f, want) {
			t.Errorf("stack filter missing %q:\n%s", want, f)
		}
	}
	// Stack timing contract: stagger fits inside the window.
	slide, stag := stackTiming(3, 5.6)
	if slide <= 0 || stag <= 0 || stag*(3-1)+slide > 5.6 {
		t.Errorf("stackTiming(3, 5.6) = %.2f/%.2f does not fit", slide, stag)
	}
}

// TestPrepareMotionAssets_StampCTAHighlights exercises the generated-PNG
// overlays: Source is rewritten to an existing PNG file per layer.
func TestPrepareMotionAssets_StampCTAHighlights(t *testing.T) {
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#0f172a",
		DurationSec: 6,
		Layers: []contract.Layer{
			{Kind: contract.LayerStamp, Text: "MỚI", Angle: -8, Y: 0.15, W: 0.4, Fill: "#F87171"},
			{Kind: contract.LayerCTA, Text: "Xem ngay", Y: 0.82, Fill: "#38BDF8", FillB: "#8B5CF6"},
			{Kind: contract.LayerText, Text: "Chip giảm 30 giá", Y: 0.2, FontSize: 64,
				Highlights: []contract.TextHighlight{{Word: "30", Color: "#F97316"}}},
			{Kind: contract.LayerText, Text: "plain stays drawtext", Y: 0.7},
		},
	}
	tmp := t.TempDir()
	bodyFont := extractTestFont(t, "Inter-Regular.ttf")
	prepareMotionAssets(&sc, 720, 1280, tmp, 0, FontSet{
		Body:     bodyFont,
		BodyBold: extractTestFont(t, "Inter-Bold.ttf"),
	}, bodyFont)
	if !strings.HasSuffix(sc.Layers[0].Source, "motion_stamp_0_0.png") {
		t.Errorf("stamp source = %q, want generated png", sc.Layers[0].Source)
	}
	if !strings.HasSuffix(sc.Layers[1].Source, "motion_cta_0_1.png") {
		t.Errorf("cta source = %q, want generated png", sc.Layers[1].Source)
	}
	if !strings.HasSuffix(sc.Layers[2].Source, "motion_text_0_2.png") {
		t.Errorf("highlight text source = %q, want generated png", sc.Layers[2].Source)
	}
	if sc.Layers[3].Source != "" {
		t.Errorf("plain text layer must stay drawtext, got %q", sc.Layers[3].Source)
	}
	for _, l := range sc.Layers[:3] {
		data, err := os.ReadFile(l.Source)
		if err != nil {
			t.Fatalf("read %s: %v", l.Source, err)
		}
		if len(data) < 8 || string(data[:4]) != "\x89PNG" {
			t.Errorf("%s is not a PNG", l.Source)
		}
	}
}

// TestBuildColorSceneArgs_MotionPlainVf covers the -vf path with the
// filter-graph primitives and the filter_complex switch for PNG kinds.
func TestBuildColorSceneArgs_MotionPlainVf(t *testing.T) {
	cfg := FFmpegConfig{FontFile: "/usr/share/fonts/NotoSans-Regular.ttf"}
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#0f172a",
		DurationSec: 6,
		Layers: []contract.Layer{
			{Kind: contract.LayerCounter, To: 20, Y: 0.3},
			{Kind: contract.LayerToggleGrid, Y: 0.55, W: 0.5},
		},
	}
	args, err := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_0.mp4", t.TempDir(), 0, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	vf := indexOfArg(args, "-vf")
	if vf < 0 {
		t.Fatal("expected -vf chain for filter-graph primitives")
	}
	chain := args[vf+1]
	// File mode: the %{eif:} expansion lives inside the textfile (same
	// arbitrary-text lesson as captions) — the chain carries the textfile ref.
	if !strings.Contains(chain, "textfile='") || !strings.Contains(chain, "drawbox=") {
		t.Errorf("expected counter textfile ref and toggle drawboxes in %s", chain)
	}
	if strings.Contains(chain, "overlay=") {
		t.Errorf("no PNG layers — overlay must not appear: %s", chain)
	}
}

// TestBuildColorSceneArgs_StampSwitchesToComplex covers the PNG-overlay
// path: the stamp becomes an extra 12fps input and filter_complex graph.
func TestBuildColorSceneArgs_StampSwitchesToComplex(t *testing.T) {
	cfg := FFmpegConfig{Fonts: FontSet{BodyBold: extractTestFont(t, "Inter-Bold.ttf"), Body: extractTestFont(t, "Inter-Regular.ttf")}}
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#101820",
		DurationSec: 4,
		Layers: []contract.Layer{
			{Kind: contract.LayerStamp, Text: "MỚI", Angle: -8, Y: 0.15, W: 0.4},
		},
	}
	tmp := t.TempDir()
	args, err := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_0.mp4", tmp, 0, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	stampInput := false
	for _, a := range args {
		if strings.Contains(a, "motion_stamp_0_0.png") {
			stampInput = true
		}
	}
	if !stampInput {
		t.Error("stamp png input missing from args")
	}
	fcIdx := indexOfArg(args, "-filter_complex")
	if fcIdx < 0 {
		t.Fatal("expected -filter_complex for stamp layers")
	}
	if !strings.Contains(args[fcIdx+1], "overlay=") {
		t.Errorf("expected overlay compositing in %s", args[fcIdx+1])
	}
}

// TestResolveStylePack pins the expansion rules: pack fills only what the
// scene left unset, non-color scenes take only the glow.
func TestResolveStylePack(t *testing.T) {
	sc := contract.Scene{Type: contract.SceneColor, StylePack: "tech_dark"}
	got := resolveStylePack(sc)
	if got.Color != "#0D1117" || got.Color2 != "#1E293B" || got.Glow != "#38BDF8" || !got.Grid {
		t.Errorf("tech_dark expansion = %+v", got)
	}
	// Explicit scene fields win.
	sc = contract.Scene{Type: contract.SceneColor, StylePack: "tech_dark",
		Color: "#123456", Color2: "#654321", Glow: "#111111", Grid: true}
	got = resolveStylePack(sc)
	if got.Color != "#123456" || got.Color2 != "#654321" || got.Glow != "#111111" {
		t.Errorf("scene fields must win over the pack: %+v", got)
	}
	// paper_light: no glow.
	sc = contract.Scene{Type: contract.SceneColor, StylePack: "paper_light"}
	got = resolveStylePack(sc)
	if got.Glow != "" || got.Color != "#F8FAFC" {
		t.Errorf("paper_light expansion = %+v", got)
	}
	// Image scenes: glow only, backdrop untouched.
	sc = contract.Scene{Type: contract.SceneImage, Source: "a.png", StylePack: "neon_lab"}
	got = resolveStylePack(sc)
	if got.Color != "" || got.Glow != "#22D3EE" {
		t.Errorf("image-scene expansion = %+v", got)
	}
	// Unknown pack: unchanged.
	sc = contract.Scene{Type: contract.SceneColor, Color: "#000000"}
	if got := resolveStylePack(sc); got.Color != "#000000" || got.Grid {
		t.Errorf("plain scene mutated: %+v", got)
	}
}

// extractTestFont materializes one embedded font for PNG rasterization in
// tests (faceFor reads from disk).
func extractTestFont(t *testing.T, name string) string {
	t.Helper()
	data, err := fontFS.ReadFile("fonts/" + name)
	if err != nil {
		t.Fatalf("embedded font %s missing: %v", name, err)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
