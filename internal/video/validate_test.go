package video

import (
	"encoding/json"
	"testing"
)

func TestStoryboardValidMinimal(t *testing.T) {
	sb := Storyboard{
		Version: 1,
		Scenes: []Scene{
			{Type: SceneColor, Color: "#000000", DurationSec: 5},
		},
	}
	if err := sb.Validate(); err != nil {
		t.Fatalf("minimal valid storyboard: %v", err)
	}
}

func TestStoryboardValidFull(t *testing.T) {
	raw := `{
		"version": 1,
		"canvas": { "width": 1080, "height": 1920, "fps": 30 },
		"engine": "ffmpeg",
		"scenes": [
			{
				"type": "image",
				"source": "media/banner.png",
				"duration_sec": 5,
				"fit": "cover",
				"ken_burns": { "zoom_from": 1.0, "zoom_to": 1.15, "pan": "none" },
				"caption": { "text": "Xin chào", "position": "bottom", "font_size": 48 },
				"narration": { "text": "Chào mừng các bạn", "provider": "edge", "voice": "vi-VN-HoaiMyNeural" }
			},
			{ "type": "video", "source": "media/clip.mp4", "duration_sec": 8, "mute": true },
			{ "type": "color", "color": "#101820", "duration_sec": 2,
				"caption": { "text": "Kết thúc" } }
		],
		"audio": { "bgm_path": "", "bgm_volume": 0.2, "narration_volume": 1.0 },
		"output": { "format": "mp4", "height": 1080, "video_bitrate": "1500k" }
	}`
	var sb Storyboard
	if err := json.Unmarshal([]byte(raw), &sb); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sb.Validate(); err != nil {
		t.Fatalf("full valid storyboard: %v", err)
	}
}

func TestStoryboardInvalidVersion(t *testing.T) {
	sb := Storyboard{Version: 2, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for version != 1")
	}
}

func TestStoryboardNoScenes(t *testing.T) {
	sb := Storyboard{Version: 1, Scenes: []Scene{}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for 0 scenes")
	}
}

func TestStoryboardTooManyScenes(t *testing.T) {
	scenes := make([]Scene, 61)
	for i := range scenes {
		scenes[i] = Scene{Type: SceneColor, Color: "#000000", DurationSec: 1}
	}
	sb := Storyboard{Version: 1, Scenes: scenes}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for 61 scenes")
	}
}

func TestStoryboardTotalDurationCap(t *testing.T) {
	scenes := make([]Scene, 21)
	for i := range scenes {
		scenes[i] = Scene{Type: SceneColor, Color: "#000000", DurationSec: 30}
	}
	// 21 * 30 = 630 > 600
	sb := Storyboard{Version: 1, Scenes: scenes}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for total duration > 600s")
	}
}

func TestStoryboardExactDurationCap(t *testing.T) {
	scenes := make([]Scene, 20)
	for i := range scenes {
		scenes[i] = Scene{Type: SceneColor, Color: "#000000", DurationSec: 30}
	}
	// 20 * 30 = 600 = exactly allowed
	sb := Storyboard{Version: 1, Scenes: scenes}
	if err := sb.Validate(); err != nil {
		t.Fatalf("exact 600s total should be valid: %v", err)
	}
}

func TestStoryboardCanvasEdgeCap(t *testing.T) {
	sb := Storyboard{Version: 1, Canvas: Canvas{Width: 2000, Height: 1080}, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for canvas width > 1920")
	}
}

func TestStoryboardFpsOutOfRange(t *testing.T) {
	sb := Storyboard{Version: 1, Canvas: Canvas{FPS: 0}, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 3}}}
	// fps=0 → default 30, should be ok
	if err := sb.Validate(); err != nil {
		t.Fatalf("fps 0 should default to 30: %v", err)
	}
	sb2 := Storyboard{Version: 1, Canvas: Canvas{FPS: 61}, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 3}}}
	if err := sb2.Validate(); err == nil {
		t.Fatal("expected error for fps > 60")
	}
}

func TestStoryboardOutputHeightInvalid(t *testing.T) {
	sb := Storyboard{Version: 1, Output: Output{Height: 1440}, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for unsupported output height")
	}
}

func TestStoryboardOutputFormatInvalid(t *testing.T) {
	sb := Storyboard{Version: 1, Output: Output{Format: "webm"}, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestSceneImageMissingSource(t *testing.T) {
	sb := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneImage, DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for image scene without source")
	}
}

func TestSceneVideoMissingSource(t *testing.T) {
	sb := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneVideo, DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for video scene without source")
	}
}

func TestSceneColorMissingColor(t *testing.T) {
	sb := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneColor, DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for color scene without color")
	}
}

func TestSceneColorBadHex(t *testing.T) {
	sb := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneColor, Color: "red", DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for non-hex color")
	}
}

func TestSceneUnknownType(t *testing.T) {
	sb := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneKind("text"), DurationSec: 3}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for unknown scene type")
	}
}

func TestSceneDurationRange(t *testing.T) {
	sb := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 0.5}}}
	if err := sb.Validate(); err == nil {
		t.Fatal("expected error for duration < 1")
	}
	sb2 := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 31}}}
	if err := sb2.Validate(); err == nil {
		t.Fatal("expected error for duration > 30")
	}
}

func TestEffectiveOutputDefaults(t *testing.T) {
	sb := Storyboard{}
	f, h, b := sb.EffectiveOutput()
	if f != "mp4" || h != 720 || b != "1500k" {
		t.Fatalf("defaults wrong: format=%s height=%d bitrate=%s", f, h, b)
	}
}

func TestEffectiveOutputCustom(t *testing.T) {
	sb := Storyboard{Output: Output{Format: "mp4", Height: 1080, VideoBitrate: "3000k"}}
	f, h, b := sb.EffectiveOutput()
	if f != "mp4" || h != 1080 || b != "3000k" {
		t.Fatalf("custom wrong: format=%s height=%d bitrate=%s", f, h, b)
	}
}

func TestDimensionsFor(t *testing.T) {
	tests := []struct {
		aspect string
		height int
		wantW  int
		wantH  int
		wantOk bool
	}{
		{AspectVertical, 1920, 1080, 1920, true},
		{AspectVertical, 720, 405, 720, true},
		{AspectWide, 1080, 1920, 1080, true},
		{AspectSquare, 1080, 1080, 1080, true},
		{"4:3", 720, 0, 0, false},
		{AspectVertical, 0, 405, 720, true}, // height=0 → default 720
	}
	for _, tt := range tests {
		w, h, ok := DimensionsFor(tt.aspect, tt.height)
		if w != tt.wantW || h != tt.wantH || ok != tt.wantOk {
			t.Errorf("DimensionsFor(%q, %d) = (%d,%d,%v), want (%d,%d,%v)",
				tt.aspect, tt.height, w, h, ok, tt.wantW, tt.wantH, tt.wantOk)
		}
	}
}

func TestSceneDurationBounds(t *testing.T) {
	// exactly 1s = valid
	sb := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 1}}}
	if err := sb.Validate(); err != nil {
		t.Fatalf("1s duration should be valid: %v", err)
	}
	// exactly 30s = valid
	sb2 := Storyboard{Version: 1, Scenes: []Scene{{Type: SceneColor, Color: "#000000", DurationSec: 30}}}
	if err := sb2.Validate(); err != nil {
		t.Fatalf("30s duration should be valid: %v", err)
	}
}
