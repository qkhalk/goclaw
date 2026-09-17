package vworker

import (
	"os"
	"path/filepath"
	"slices"
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

	tmp := t.TempDir()
	args, err := buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_001.mp4", tmp, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Check that input is specified
	if !containsArg(args, "-loop") {
		t.Error("expected -loop flag for image input")
	}
	if !containsArg(args, "media/banner.png") {
		t.Error("expected source file in args")
	}

	// Check zoompan in filtergraph (image scenes composite blur backdrop +
	// contain-fit photo, so they use -filter_complex, not -vf)
	vfIdx := indexOfArg(args, "-filter_complex")
	if vfIdx < 0 {
		t.Fatal("no -filter_complex flag found")
	}
	vf := args[vfIdx+1]
	if !strings.Contains(vf, "zoompan") {
		t.Errorf("expected zoompan in filtergraph, got: %s", vf)
	}
	if !strings.Contains(vf, "drawtext") {
		t.Errorf("expected drawtext in filtergraph, got: %s", vf)
	}
	if !strings.Contains(vf, "fontfile=") {
		t.Errorf("expected fontfile in filtergraph, got: %s", vf)
	}
	// Caption text goes through a textfile (arbitrary text survives ffmpeg
	// filtergraph escaping) — the filtergraph references it, the file holds it.
	if !strings.Contains(vf, "textfile=") {
		t.Errorf("expected textfile in filtergraph, got: %s", vf)
	}
	capPath := filepath.Join(tmp, "caption_001.txt")
	if _, err := os.Stat(capPath); err != nil {
		t.Errorf("expected caption file at %s: %v", capPath, err)
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

	args, err := buildImageSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4", "", 0)
	if err != nil {
		t.Fatal(err)
	}

	vfIdx := indexOfArg(args, "-filter_complex")
	if vfIdx < 0 {
		t.Fatal("no -filter_complex flag found")
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

	args, err := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/scene_003.mp4", t.TempDir(), 3, true)
	if err != nil {
		t.Fatal(err)
	}

	// Should use lavfi color source
	if !containsArg(args, "-f") {
		t.Error("expected -f lavfi")
	}
	lavfiIdx := indexOfArg(args, "-i")
	if lavfiIdx < 0 {
		t.Fatal("no -i flag")
	}
	lavfi := args[lavfiIdx+1]
	if !strings.Contains(lavfi, "0x101820") {
		t.Errorf("expected color 0x101820 in lavfi, got: %s", lavfi)
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

	args, err := buildColorSceneArgs(cfg, sc, 1080, 1920, 30, "/tmp/out.mp4", "", 0, true)
	if err != nil {
		t.Fatal(err)
	}

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
	narr := []NarrTrack{
		{Path: "/tmp/narr_001.mp3", StartSec: 0},
		{Path: "/tmp/narr_002.mp3", StartSec: 5.5},
	}
	bgmPath := "/tmp/bgm.mp3"
	mix := contract.AudioMix{BGMVolume: 0.3, NarrationVolume: 0.8}

	args := buildMixArgs(cfg, "/tmp/video.mp4", "/tmp/final.mp4",
		narr, bgmPath, mix, 30, 12.0)

	// Should have filter_complex
	if !containsArg(args, "-filter_complex") {
		t.Error("expected -filter_complex for audio mixing")
	}
	// Should have aac codec
	if !containsArg(args, "aac") {
		t.Error("expected aac codec")
	}
	// Should have 5 inputs (video + silence base + 2 narr + 1 bgm)
	inputCount := 0
	for _, a := range args {
		if a == "-i" {
			inputCount++
		}
	}
	if inputCount != 5 {
		t.Errorf("expected 5 inputs (video + silence + 2 narr + 1 bgm), got %d", inputCount)
	}
	// Narration clips are delayed to their scene starts, not concatenated
	fc := filterArg(args, "-filter_complex")
	if !strings.Contains(fc, "adelay=0:all=1") {
		t.Errorf("expected adelay=0 for the first track, got %s", fc)
	}
	if !strings.Contains(fc, "adelay=5500:all=1") {
		t.Errorf("expected adelay=5500 for the second track, got %s", fc)
	}
	if strings.Contains(fc, "concat=") {
		t.Errorf("narration must not be concatenated: %s", fc)
	}
}

func TestBuildMixArgs_NoAudio(t *testing.T) {
	cfg := FFmpegConfig{}
	args := buildMixArgs(cfg, "/tmp/video.mp4", "/tmp/final.mp4",
		nil, "", contract.AudioMix{}, 30, 10.0)

	if containsArg(args, "-filter_complex") {
		t.Error("no filter_complex expected with no audio")
	}
}

func TestBuildMixArgs_NarrationOnly(t *testing.T) {
	cfg := FFmpegConfig{}
	narr := []NarrTrack{{Path: "/tmp/narr_001.mp3", StartSec: 2.0}}

	args := buildMixArgs(cfg, "/tmp/video.mp4", "/tmp/final.mp4",
		narr, "", contract.AudioMix{}, 30, 10.0)

	if !containsArg(args, "-filter_complex") {
		t.Error("expected filter_complex for narration")
	}
	fc := filterArg(args, "-filter_complex")
	if !strings.Contains(fc, "adelay=2000:all=1") {
		t.Errorf("expected adelay=2000, got %s", fc)
	}
}

func TestPanExprsNone(t *testing.T) {
	x, y := buildPanExprs("none", 120)
	if x != "(iw-iw/zoom)/2" || y != "(ih-ih/zoom)/2" {
		t.Errorf("unexpected centered pan exprs: x=%s y=%s", x, y)
	}
}

func TestPanExprsLeftEased(t *testing.T) {
	x, y := buildPanExprs("left", 120)
	if !strings.Contains(x, "(iw-iw/zoom)*0.45") {
		t.Errorf("expected margin-scaled travel in left pan x: %s", x)
	}
	if !strings.Contains(x, "(3-2*on/120)") {
		t.Errorf("expected smoothstep ease in left pan x: %s", x)
	}
	if y != "(ih-ih/zoom)/2" {
		t.Errorf("left pan must not move y: %s", y)
	}
}

func TestPanExprsUpMovesY(t *testing.T) {
	x, y := buildPanExprs("up", 90)
	if x != "(iw-iw/zoom)/2" {
		t.Errorf("up pan must not move x: %s", x)
	}
	if !strings.Contains(y, "(ih-ih/zoom)*0.45") {
		t.Errorf("expected vertical travel in up pan y: %s", y)
	}
}

func TestXfadeTransitionMapping(t *testing.T) {
	cases := map[string]string{
		"fade":       "fadeblack",
		"crossfade":  "fade",
		"slide_left": "slideleft",
		"slide_up":   "slideup",
		"":           "fade",
		"none":       "fade",
	}
	for in, want := range cases {
		if got := xfadeTransition(in); got != want {
			t.Errorf("xfadeTransition(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildXfadeArgs(t *testing.T) {
	files := []string{"/tmp/s0.mp4", "/tmp/s1.mp4", "/tmp/s2.mp4"}
	trans := []string{"", "crossfade", "slide_up"}
	offsets := []float64{4.0, 8.5}
	args := buildXfadeArgs(FFmpegConfig{}, files, trans, offsets, 30, "/tmp/out.mp4")

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-i /tmp/s0.mp4", "-i /tmp/s1.mp4", "-i /tmp/s2.mp4",
		"xfade=transition=fade:duration=0.50:offset=4.000",
		"xfade=transition=slideup:duration=0.50:offset=8.500",
		"-map [v2]",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("xfade args missing %q in: %s", want, joined)
		}
	}
}

func TestDarkerHex(t *testing.T) {
	if got := darkerHex("ff8800"); got != "7f4400" {
		t.Errorf("darkerHex(ff8800) = %s, want 7f4400", got)
	}
}

func TestImageSceneBlurBackdrop(t *testing.T) {
	sc := contract.Scene{Type: contract.SceneImage, Source: "media/a.jpg", DurationSec: 4}
	args, err := buildImageSceneArgs(FFmpegConfig{}, sc, 1080, 1920, 30, "/tmp/out.mp4", t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"split=2", "overlay=0:0", "eq=brightness=-0.12", "zoompan", "-map [vout]"} {
		if !strings.Contains(joined, want) {
			t.Errorf("image scene args missing %q", want)
		}
	}
}

func TestColorSceneGradientVsFlat(t *testing.T) {
	sc := contract.Scene{Type: contract.SceneColor, Color: "#0f172a", DurationSec: 3}
	gradArgs, err := buildColorSceneArgs(FFmpegConfig{}, sc, 1080, 1920, 30, "/tmp/g.mp4", t.TempDir(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(gradArgs, " "), "gradients=s=1080x1920") {
		t.Error("animated color scene should use gradients source")
	}
	flatArgs, err := buildColorSceneArgs(FFmpegConfig{}, sc, 1080, 1920, 30, "/tmp/f.mp4", t.TempDir(), 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(flatArgs, " "), "color=c=0x0f172a") {
		t.Error("flat color scene should use color source")
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
	return slices.Contains(args, val)
}

func indexOfArg(args []string, val string) int {
	for i, a := range args {
		if a == val {
			return i
		}
	}
	return -1
}

// filterArg returns the value following the named flag, or "" when absent.
func filterArg(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
