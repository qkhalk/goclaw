package vworker

import (
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

func TestEscapeDrawText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello", "Hello"},
		{"It's", `It'\''s`},
		{"a:b", `a\\:b`},
		{`\n`, `\\n`},
		{`[bold]`, `\\[bold\\]`},
		{`He said "hello" and 'bye'`, `He said "hello" and '\''bye'\''`},
	}
	for _, tt := range tests {
		got := escapeDrawText(tt.input)
		if got != tt.want {
			t.Errorf("escapeDrawText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildImageSceneArgs_KenBurnsCaption(t *testing.T) {
	cfg := FFmpegConfig{FontFile: "/usr/share/fonts/NotoSans-Regular.ttf"}
	sc := contract.Scene{
		Type:        contract.SceneImage,
		Source:      "media/banner.png",
		DurationSec: 5,
		KenBurns:    &contract.KenBurns{ZoomFrom: 1.0, ZoomTo: 1.15, Pan: "none"},
		Caption:     &contract.Caption{Text: "Xin chào!", Position: "bottom", FontSize: 48},
	}

	args := buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_001.mp4")

	// Check that input is specified
	if !containsArg(args, "-loop") {
		t.Error("expected -loop flag for image input")
	}
	if !containsArg(args, "media/banner.png") {
		t.Error("expected source file in args")
	}

	// Check zoompan in filtergraph
	vfIdx := indexOfArg(args, "-vf")
	if vfIdx < 0 {
		t.Fatal("no -vf flag found")
	}
	vf := args[vfIdx+1]
	if !strings.Contains(vf, "zoompan") {
		t.Errorf("expected zoompan in filtergraph, got: %s", vf)
	}
	if !strings.Contains(vf, "drawtext") {
		t.Errorf("expected drawtext in filtergraph, got: %s", vf)
	}
	// Check font file is included
	if !strings.Contains(vf, "fontfile=") {
		t.Errorf("expected fontfile in filtergraph, got: %s", vf)
	}
	// Check text is escaped
	if !strings.Contains(vf, "Xin chào!") {
		t.Errorf("expected caption text in filtergraph, got: %s", vf)
	}

	// Check output file
	last := args[len(args)-1]
	if last != "/tmp/scene_001.mp4" {
		t.Errorf("expected output path, got: %s", last)
	}
}

func TestBuildImageSceneArgs_NoCaptionNoFont(t *testing.T) {
	cfg := FFmpegConfig{} // no font
	sc := contract.Scene{
		Type:        contract.SceneImage,
		Source:      "img.png",
		DurationSec: 3,
	}

	args := buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4")

	vfIdx := indexOfArg(args, "-vf")
	if vfIdx < 0 {
		t.Fatal("no -vf flag found")
	}
	vf := args[vfIdx+1]
	if strings.Contains(vf, "drawtext") {
		t.Errorf("drawtext should NOT be present without font, got: %s", vf)
	}
}

func TestBuildVideoSceneArgs(t *testing.T) {
	cfg := FFmpegConfig{}
	sc := contract.Scene{
		Type:        contract.SceneVideo,
		Source:      "media/clip.mp4",
		DurationSec: 8,
		Mute:        true,
	}

	args := buildVideoSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_002.mp4")

	if !containsArg(args, "-i") {
		t.Error("expected -i flag")
	}
	if !containsArg(args, "-an") {
		t.Error("expected -an for mute=true")
	}
	// Check scale filter
	vfIdx := indexOfArg(args, "-vf")
	if vfIdx < 0 {
		t.Fatal("no -vf flag found")
	}
	vf := args[vfIdx+1]
	if !strings.Contains(vf, "scale=") {
		t.Errorf("expected scale in filtergraph, got: %s", vf)
	}
	if !strings.Contains(vf, "format=yuv420p") {
		t.Errorf("expected format=yuv420p in filtergraph, got: %s", vf)
	}

	// Check -t for duration
	if !containsArg(args, "-t") {
		t.Error("expected -t flag for trim")
	}
}

func TestBuildVideoSceneArgs_NoMute(t *testing.T) {
	cfg := FFmpegConfig{}
	sc := contract.Scene{
		Type:        contract.SceneVideo,
		Source:      "clip.mp4",
		DurationSec: 5,
		Mute:        false,
	}

	args := buildVideoSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4")
	if containsArg(args, "-an") {
		t.Error("should NOT have -an when mute=false")
	}
}

func TestBuildColorSceneArgs_WithCaption(t *testing.T) {
	cfg := FFmpegConfig{FontFile: "/usr/share/fonts/NotoSans.ttf"}
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#101820",
		DurationSec: 2,
		Caption:     &contract.Caption{Text: "Ket thuc", Position: "center", FontSize: 32},
	}

	args := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_003.mp4")

	// Should use lavfi color source
	if !containsArg(args, "-f") {
		t.Error("expected -f lavfi")
	}
	lavfiIdx := indexOfArg(args, "-i")
	if lavfiIdx < 0 {
		t.Fatal("no -i flag")
	}
	lavfi := args[lavfiIdx+1]
	if !strings.Contains(lavfi, "color=c=0x101820") {
		t.Errorf("expected color=0x101820 in lavfi, got: %s", lavfi)
	}

	// Check drawtext in filtergraph
	vfIdx := indexOfArg(args, "-vf")
	if vfIdx < 0 {
		t.Fatal("no -vf flag")
	}
	vf := args[vfIdx+1]
	if !strings.Contains(vf, "drawtext") {
		t.Errorf("expected drawtext, got: %s", vf)
	}
	// Check center positioning
	if !strings.Contains(vf, "(h-th)/2") {
		t.Errorf("expected center position (h-th)/2, got: %s", vf)
	}
}

func TestBuildColorSceneArgs_NoCaption(t *testing.T) {
	cfg := FFmpegConfig{} // no font
	sc := contract.Scene{
		Type:        contract.SceneColor,
		Color:       "#FF0000",
		DurationSec: 1,
	}

	args := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4")

	vfIdx := indexOfArg(args, "-vf")
	if vfIdx < 0 {
		t.Fatal("no -vf flag")
	}
	vf := args[vfIdx+1]
	if strings.Contains(vf, "drawtext") {
		t.Errorf("drawtext should NOT be present without font, got: %s", vf)
	}
}

func TestBuildConcatArgs(t *testing.T) {
	cfg := FFmpegConfig{}
	files := []string{"/tmp/scene_001.mp4", "/tmp/scene_002.mp4", "/tmp/scene_003.mp4"}
	concatFile := "/tmp/concat.txt"
	outPath := "/tmp/concat.mp4"

	args := buildConcatArgs(cfg, files, concatFile, outPath)

	if !containsArg(args, "-f") {
		t.Error("expected -f concat")
	}
	if !containsArg(args, concatFile) {
		t.Error("expected concat file path")
	}
	if !containsArg(args, "-c") {
		t.Error("expected -c copy")
	}
}

func TestBuildMixArgs_NarrationAndBGM(t *testing.T) {
	cfg := FFmpegConfig{}
	narrFiles := []string{"/tmp/narr_001.mp3", "/tmp/narr_002.mp3"}
	bgmPath := "/tmp/bgm.mp3"
	mix := contract.AudioMix{BGMVolume: 0.3, NarrationVolume: 0.8}

	args := buildMixArgs(cfg, "/tmp/video.mp4", "/tmp/final.mp4",
		narrFiles, bgmPath, mix, 30)

	// Should have filter_complex
	if !containsArg(args, "-filter_complex") {
		t.Error("expected -filter_complex for audio mixing")
	}
	// Should have aac codec
	if !containsArg(args, "aac") {
		t.Error("expected aac codec")
	}
	// Should have 3 inputs (video + 2 narr + 1 bgm = 4)
	inputCount := 0
	for _, a := range args {
		if a == "-i" {
			inputCount++
		}
	}
	if inputCount != 4 {
		t.Errorf("expected 4 inputs (video + 2 narr + 1 bgm), got %d", inputCount)
	}
}

func TestBuildMixArgs_NoAudio(t *testing.T) {
	cfg := FFmpegConfig{}
	args := buildMixArgs(cfg, "/tmp/video.mp4", "/tmp/final.mp4",
		nil, "", contract.AudioMix{}, 30)

	if containsArg(args, "-filter_complex") {
		t.Error("no filter_complex expected with no audio")
	}
}

func TestBuildMixArgs_NarrationOnly(t *testing.T) {
	cfg := FFmpegConfig{}
	narrFiles := []string{"/tmp/narr_001.mp3"}

	args := buildMixArgs(cfg, "/tmp/video.mp4", "/tmp/final.mp4",
		narrFiles, "", contract.AudioMix{}, 30)

	if !containsArg(args, "-filter_complex") {
		t.Error("expected filter_complex for narration")
	}
}

func TestPanExprNone(t *testing.T) {
	expr := buildPanExpr("none")
	if !strings.Contains(expr, "iw/2-(iw/zoom/2)") {
		t.Errorf("unexpected pan expr for 'none': %s", expr)
	}
}

func TestPanExprLeft(t *testing.T) {
	expr := buildPanExpr("left")
	if !strings.Contains(expr, "on*1") {
		t.Errorf("expected animation in left pan expr: %s", expr)
	}
}

func TestSceneOutputPath(t *testing.T) {
	got := sceneOutputPath("/tmp/job123", 0)
	if got != "/tmp/job123/scene_000.mp4" {
		t.Errorf("unexpected scene path: %s", got)
	}
	got = sceneOutputPath("/tmp/job123", 5)
	if got != "/tmp/job123/scene_005.mp4" {
		t.Errorf("unexpected scene path: %s", got)
	}
}

// --- helpers ---

func containsArg(args []string, val string) bool {
	for _, a := range args {
		if a == val {
			return true
		}
	}
	return false
}

func indexOfArg(args []string, val string) int {
	for i, a := range args {
		if a == val {
			return i
		}
	}
	return -1
}
