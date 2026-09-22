package vworker

import (
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// --- per-scene fx filter generation ---

func TestTransformFilters(t *testing.T) {
	tests := []struct {
		name string
		t    *contract.Transform
		want string
	}{
		{"nil", nil, ""},
		{"identity", &contract.Transform{}, ""},
		{"identity_scale", &contract.Transform{Scale: 1}, ""},
		{"rotate_only", &contract.Transform{Rotate: 90}, "rotate=1.570796:c=black"},
		{
			"scale_up",
			&contract.Transform{Scale: 2},
			"scale=2160:3840,pad=2160:3840:0:0:black,crop=1080:1920:540:960",
		},
		{
			// shrink + 10% x offset: content 540x960 centered inside a
			// canvas-sized+bleed frame, window clamps flush left so the
			// content center lands at canvas center +108px
			"shrink_offset",
			&contract.Transform{Scale: 0.5, X: 10},
			"scale=540:960,pad=1296:1920:378:480:black,crop=1080:1920:0:0",
		},
		{
			// rotate and scale combine; offsets land in the bleed margin
			// (x=5% of 1080 = 54px, y=-5% of 1920 = -96px)
			"rotate_scale_offset",
			&contract.Transform{Scale: 1.2, X: 5, Y: -5, Rotate: 15},
			"rotate=0.261799:c=black,scale=1296:2304,pad=1404:2496:54:96:black,crop=1080:1920:108:384",
		},
		{
			// opacity alone is handled by opacityFilters, no geometry here
			"opacity_only",
			&contract.Transform{Opacity: 0.5},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transformFilters(tt.t, 1080, 1920)
			if got != tt.want {
				t.Errorf("transformFilters() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestColorFilterFilters(t *testing.T) {
	tests := []struct {
		name string
		f    *contract.Filter
		want string
	}{
		{"nil", nil, ""},
		{"identity", &contract.Filter{}, ""},
		{"identity_ones", &contract.Filter{Brightness: 1, Contrast: 1, Saturate: 1}, ""},
		{
			"brightness_multiplicative_mapping",
			&contract.Filter{Brightness: 1.2},
			"eq=brightness=0.2000",
		},
		{
			"full",
			&contract.Filter{Brightness: 0.5, Contrast: 1.5, Saturate: 0.5, Blur: 3},
			"eq=brightness=-0.5000:contrast=1.5000:saturation=0.5000,gblur=sigma=3.000",
		},
		{"brightness_clamped_high", &contract.Filter{Brightness: 5}, "eq=brightness=1.0000"},
		{"blur_clamped", &contract.Filter{Blur: 500}, "gblur=sigma=100.000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorFilterFilters(tt.f)
			if got != tt.want {
				t.Errorf("colorFilterFilters() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpacityFilters(t *testing.T) {
	tests := []struct {
		name string
		t    *contract.Transform
		want string
	}{
		{"nil", nil, ""},
		{"opaque", &contract.Transform{Opacity: 1}, ""},
		{"half", &contract.Transform{Opacity: 0.5},
			"format=rgba,colorchannelmixer=rr=0.5000:gg=0.5000:bb=0.5000"},
		{"zero_unset", &contract.Transform{Opacity: 0}, ""},
		{"negative_unset", &contract.Transform{Opacity: -5}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := opacityFilters(tt.t)
			if got != tt.want {
				t.Errorf("opacityFilters() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Identity transform/filter values must produce a clean filtergraph: zoompan
// and the final format node only, zero fx nodes.
func TestBuildImageSceneArgs_IdentityFxNoNodes(t *testing.T) {
	cfg := FFmpegConfig{}
	sc := contract.Scene{
		Type:        contract.SceneImage,
		Source:      "media/banner.png",
		DurationSec: 5,
		Transform:   &contract.Transform{Scale: 1, X: 0, Y: 0, Rotate: 0, Opacity: 1},
		Filter:      &contract.Filter{Brightness: 1, Contrast: 1, Saturate: 1, Blur: 0},
	}
	args, err := buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	vf := args[indexOfArg(args, "-vf")+1]
	for _, node := range []string{"rotate=", "pad=", "crop=", "scale=", "eq=", "gblur=", "colorchannelmixer"} {
		if strings.Contains(vf, node) {
			t.Errorf("identity fx should not emit %q node, got: %s", node, vf)
		}
	}
	if !strings.Contains(vf, "zoompan") || !strings.Contains(vf, "format=yuv420p") {
		t.Errorf("expected zoompan + format nodes, got: %s", vf)
	}
}

// Full transform + filter on an image scene: nodes appear in the same order
// the web editor draws (zoompan → rotate → eq → gblur → opacity → format).
func TestBuildImageSceneArgs_FullFxOrder(t *testing.T) {
	cfg := FFmpegConfig{}
	sc := contract.Scene{
		Type:        contract.SceneImage,
		Source:      "media/banner.png",
		DurationSec: 5,
		Transform:   &contract.Transform{Scale: 1.2, Rotate: 15, Opacity: 0.8},
		Filter:      &contract.Filter{Brightness: 1.1, Blur: 2},
	}
	args, err := buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	vf := args[indexOfArg(args, "-vf")+1]
	order := []string{"zoompan", "rotate=", "eq=", "gblur=", "colorchannelmixer", "format=yuv420p"}
	prev := -1
	for _, node := range order {
		idx := strings.Index(vf, node)
		if idx < 0 {
			t.Fatalf("expected %q in filtergraph, got: %s", node, vf)
		}
		if idx <= prev {
			t.Errorf("node %q out of order (idx %d after %d): %s", node, idx, prev, vf)
		}
		prev = idx
	}
}

func TestBuildVideoSceneArgs_Fx(t *testing.T) {
	cfg := FFmpegConfig{}
	sc := contract.Scene{
		Type:        contract.SceneVideo,
		Source:      "media/clip.mp4",
		DurationSec: 8,
		Transform:   &contract.Transform{X: 5},
		Filter:      &contract.Filter{Saturate: 1.5},
	}
	args := buildVideoSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4")
	vf := args[indexOfArg(args, "-vf")+1]

	fitIdx := strings.Index(vf, "scale=1080:1920:force_original_aspect_ratio=decrease")
	cropIdx := strings.Index(vf, "crop=1080:1920:")
	eqIdx := strings.Index(vf, "eq=saturation=1.5000")
	fmtIdx := strings.Index(vf, "format=yuv420p")
	if fitIdx < 0 || cropIdx < 0 || eqIdx < 0 || fmtIdx < 0 {
		t.Fatalf("expected fit/scale + transform crop + eq + format, got: %s", vf)
	}
	if !(fitIdx < cropIdx && cropIdx < eqIdx && eqIdx < fmtIdx) {
		t.Errorf("fx must sit between the fit stage and format=yuv420p: %s", vf)
	}
}

// Geometry-only transforms (no opacity on the wire) must stay fully opaque:
// omitempty makes 0 indistinguishable from unset, and opacity 0 must NOT
// blacken the scene (regression guard for the colorchannelmixer gate).
func TestBuildVideoSceneArgs_GeometryOnlyStaysOpaque(t *testing.T) {
	cfg := FFmpegConfig{}
	sc := contract.Scene{
		Type:        contract.SceneVideo,
		Source:      "media/clip.mp4",
		DurationSec: 8,
		Transform:   &contract.Transform{Scale: 1.2, X: 5},
	}
	args := buildVideoSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4")
	vf := args[indexOfArg(args, "-vf")+1]
	if strings.Contains(vf, "colorchannelmixer") {
		t.Errorf("geometry-only transform must not emit an opacity node, got: %s", vf)
	}
	if !strings.Contains(vf, "crop=1080:1920:") {
		t.Errorf("expected the transform crop stage, got: %s", vf)
	}
}

// --- transitions ---

func TestXfadeTransitionMapping(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{contract.TransitionFade, "fadeblack", true},
		{contract.TransitionCrossfade, "fade", true},
		{contract.TransitionSlideLeft, "slideleft", true},
		{contract.TransitionSlideUp, "slideup", true},
		{contract.TransitionNone, "", false},
		{"", "", false},
		{"wipe", "", false},
	}
	for _, tt := range tests {
		got, ok := xfadeTransition(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("xfadeTransition(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestClampTransitionSec(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0.05, minTransitionSec},
		{0.5, 0.5},
		{5, maxTransitionSec},
	}
	for _, tt := range tests {
		if got := clampTransitionSec(tt.in); got != tt.want {
			t.Errorf("clampTransitionSec(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestAnyEnterTransition(t *testing.T) {
	none := []contract.Scene{
		{Type: contract.SceneColor, Color: "#000000", DurationSec: 5, Transition: contract.TransitionFade},
		{Type: contract.SceneColor, Color: "#111111", DurationSec: 5},
	}
	if anyEnterTransition(none) {
		t.Error("first scene's transition decorates nothing — must be ignored")
	}
	mixed := []contract.Scene{
		{Type: contract.SceneColor, Color: "#000000", DurationSec: 5},
		{Type: contract.SceneColor, Color: "#111111", DurationSec: 5, Transition: contract.TransitionNone},
		{Type: contract.SceneColor, Color: "#222222", DurationSec: 5, Transition: contract.TransitionSlideUp},
	}
	if !anyEnterTransition(mixed) {
		t.Error("slide_up on scene 2 should trigger the transition path")
	}
}

// 3-scene storyboard, transitions on both edges: offsets are d0-T and
// d0+d1-2T (5-0.5=4.5 then 5+4-1=8.0).
func TestBuildTransitionArgs_ThreeSceneMixedOffsets(t *testing.T) {
	cfg := FFmpegConfig{}
	scenes := []contract.Scene{
		{Type: contract.SceneColor, Color: "#000000", DurationSec: 5},
		{Type: contract.SceneColor, Color: "#111111", DurationSec: 4, Transition: contract.TransitionCrossfade},
		{Type: contract.SceneColor, Color: "#222222", DurationSec: 6, Transition: contract.TransitionSlideLeft},
	}
	files := []string{"/t/scene_000.mp4", "/t/scene_001.mp4", "/t/scene_002.mp4"}

	args := buildTransitionArgs(cfg, scenes, files, "/t/out.mp4", 30)
	graph := args[indexOfArg(args, "-filter_complex")+1]

	wantParts := []string{
		"[0:v]settb=AVTB,fps=30,setsar=1,tpad=stop_mode=clone:stop_duration=5.000[v0];",
		"[2:v]settb=AVTB,fps=30,setsar=1,tpad=stop_mode=clone:stop_duration=6.000[v2];",
		"[v0][v1]xfade=transition=fade:duration=0.500:offset=4.500[vx1];",
		"[vx1][v2]xfade=transition=slideleft:duration=0.500:offset=8.000[vx2];",
		"[vx2]format=yuv420p[vout]",
	}
	for _, want := range wantParts {
		if !strings.Contains(graph, want) {
			t.Errorf("filter_complex missing %q\ngot: %s", want, graph)
		}
	}
	inputs := 0
	for _, a := range args {
		if a == "-i" {
			inputs++
		}
	}
	if inputs != 3 {
		t.Errorf("expected 3 inputs, got %d", inputs)
	}
	if mapIdx := indexOfArg(args, "-map"); mapIdx < 0 || args[mapIdx+1] != "[vout]" {
		t.Errorf("expected -map [vout], got: %v", args)
	}
	if last := args[len(args)-1]; last != "/t/out.mp4" {
		t.Errorf("expected output path last, got %q", last)
	}
}

// Hard cut in the middle: concat joins edge 1 (cursor advances by d0), then
// xfade offset = d0+d1-T = 5+4-0.5 = 8.5.
func TestBuildTransitionArgs_HardCutMiddle(t *testing.T) {
	cfg := FFmpegConfig{}
	scenes := []contract.Scene{
		{Type: contract.SceneColor, Color: "#000000", DurationSec: 5},
		{Type: contract.SceneColor, Color: "#111111", DurationSec: 4, Transition: contract.TransitionNone},
		{Type: contract.SceneColor, Color: "#222222", DurationSec: 6, Transition: contract.TransitionFade},
	}
	files := []string{"/t/scene_000.mp4", "/t/scene_001.mp4", "/t/scene_002.mp4"}

	args := buildTransitionArgs(cfg, scenes, files, "/t/out.mp4", 30)
	graph := args[indexOfArg(args, "-filter_complex")+1]

	wantParts := []string{
		"[v0][v1]concat=n=2:v=1:a=0[vx1];",
		"[vx1][v2]xfade=transition=fadeblack:duration=0.500:offset=8.500[vx2];",
	}
	for _, want := range wantParts {
		if !strings.Contains(graph, want) {
			t.Errorf("filter_complex missing %q\ngot: %s", want, graph)
		}
	}
	if strings.Count(graph, "xfade=") != 1 {
		t.Errorf("expected exactly one xfade node, got: %s", graph)
	}
}

// MaxSceneSec caps the rendered scene files, so offsets must be computed
// against the capped durations.
func TestBuildTransitionArgs_RespectsMaxSceneSec(t *testing.T) {
	cfg := FFmpegConfig{MaxSceneSec: 2}
	scenes := []contract.Scene{
		{Type: contract.SceneColor, Color: "#000000", DurationSec: 10},
		{Type: contract.SceneColor, Color: "#111111", DurationSec: 10, Transition: contract.TransitionCrossfade},
	}
	files := []string{"/t/scene_000.mp4", "/t/scene_001.mp4"}

	args := buildTransitionArgs(cfg, scenes, files, "/t/out.mp4", 30)
	graph := args[indexOfArg(args, "-filter_complex")+1]
	if !strings.Contains(graph, "xfade=transition=fade:duration=0.500:offset=1.500") {
		t.Errorf("offset should use capped duration (2-0.5=1.5), got: %s", graph)
	}
}

// 1s scenes with 0.5s transitions: the 0.8x guard keeps offset >= 0 and the
// transition inside the overlap window.
func TestBuildTransitionArgs_ShortScenes(t *testing.T) {
	cfg := FFmpegConfig{}
	scenes := []contract.Scene{
		{Type: contract.SceneColor, Color: "#000000", DurationSec: 1},
		{Type: contract.SceneColor, Color: "#111111", DurationSec: 1, Transition: contract.TransitionFade},
	}
	files := []string{"/t/scene_000.mp4", "/t/scene_001.mp4"}

	args := buildTransitionArgs(cfg, scenes, files, "/t/out.mp4", 30)
	graph := args[indexOfArg(args, "-filter_complex")+1]
	if !strings.Contains(graph, "duration=0.500:offset=0.500") {
		t.Errorf("expected t=0.5 offset=0.5 for 1s scenes, got: %s", graph)
	}
}
