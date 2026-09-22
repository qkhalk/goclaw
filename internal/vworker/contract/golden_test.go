package contract

import (
	"encoding/json"
	"testing"

	gwvideo "github.com/nextlevelbuilder/goclaw/internal/video"
)

// goldenFixture is a JSON storyboard that both the gateway and worker parsers
// must handle identically. These are taken from internal/video/validate_test.go.
var goldenFixtures = []struct {
	name string
	json string
}{
	{
		name: "minimal",
		json: `{
			"version": 1,
			"scenes": [{ "type": "color", "color": "#000000", "duration_sec": 5 }]
		}`,
	},
	{
		name: "full_3scene",
		json: `{
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
		}`,
	},
	{
		name: "caption_with_quotes",
		json: `{
			"version": 1,
			"scenes": [{
				"type": "color", "color": "#FF0000", "duration_sec": 3,
				"caption": { "text": "He said \"hello\" and 'bye'", "position": "center", "font_size": 32 }
			}]
		}`,
	},
		{
			name: "empty_audio",
			json: `{
				"version": 1,
				"scenes": [{ "type": "color", "color": "#ABCDEF", "duration_sec": 2 }],
				"audio": {}
			}`,
		},
		{
			name: "fx_and_transitions",
			json: `{
				"version": 1,
				"canvas": { "width": 1080, "height": 1920, "fps": 30 },
				"scenes": [
					{
						"type": "image",
						"source": "media/a.png",
						"duration_sec": 5,
						"transform": { "scale": 1.15, "x": 4, "y": -2, "rotate": 8, "opacity": 0.9 },
						"filter": { "brightness": 1.1, "contrast": 1.2, "saturate": 0.8, "blur": 1.5 }
					},
					{
						"type": "video",
						"source": "media/b.mp4",
						"duration_sec": 4,
						"transition": "crossfade",
						"transform": { "scale": 0.9 },
						"filter": { "blur": 2 }
					},
					{
						"type": "color",
						"color": "#101820",
						"duration_sec": 3,
						"transition": "slide_left"
					}
				]
			}`,
		},
}

// TestGoldenParseMatchGateway verifies that the worker contract parser
// produces the same Storyboard struct as the gateway's internal/video parser
// for every golden fixture. This is the drift guard.
func TestGoldenParseMatchGateway(t *testing.T) {
	for _, fix := range goldenFixtures {
		t.Run(fix.name, func(t *testing.T) {
			// Parse with gateway parser
			var gw gwvideo.Storyboard
			if err := json.Unmarshal([]byte(fix.json), &gw); err != nil {
				t.Fatalf("gateway parse failed: %v", err)
			}

			// Parse with worker parser
			var w Storyboard
			if err := json.Unmarshal([]byte(fix.json), &w); err != nil {
				t.Fatalf("worker parse failed: %v", err)
			}

			// Re-marshal both and compare JSON
			gwJSON, _ := json.Marshal(gw)
			wJSON, _ := json.Marshal(w)

			// Re-parse to normalize and compare structurally
			var gwNorm, wNorm Storyboard
			if err := json.Unmarshal(gwJSON, &gwNorm); err != nil {
				t.Fatalf("gateway re-parse: %v", err)
			}
			if err := json.Unmarshal(wJSON, &wNorm); err != nil {
				t.Fatalf("worker re-parse: %v", err)
			}

			// Deep compare via JSON roundtrip
			gwNormJSON, _ := json.Marshal(gwNorm)
			wNormJSON, _ := json.Marshal(wNorm)

			if string(gwNormJSON) != string(wNormJSON) {
				t.Errorf("parser drift detected for fixture %q:\n  gateway: %s\n  worker:  %s",
					fix.name, gwNormJSON, wNormJSON)
			}
		})
	}
}

// TestGoldenValidateMatchGateway ensures both Validate() implementations
// reject the same invalid inputs.
func TestGoldenValidateMatchGateway(t *testing.T) {
	invalidCases := []struct {
		name string
		json string
	}{
		{"wrong_version", `{"version":2,"scenes":[{"type":"color","color":"#000","duration_sec":5}]}`},
		{"no_scenes", `{"version":1,"scenes":[]}`},
		{"bad_height", `{"version":1,"output":{"height":1440},"scenes":[{"type":"color","color":"#000","duration_sec":5}]}`},
		{"bad_format", `{"version":1,"output":{"format":"webm"},"scenes":[{"type":"color","color":"#000","duration_sec":5}]}`},
		{"bad_transition", `{"version":1,"scenes":[{"type":"color","color":"#000000","duration_sec":5,"transition":"wipe"}]}`},
		{"good_transitions", `{"version":1,"scenes":[
			{"type":"color","color":"#000000","duration_sec":5},
			{"type":"color","color":"#111111","duration_sec":5,"transition":"fade"},
			{"type":"color","color":"#222222","duration_sec":5,"transition":"slide_up"}]}`},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			var gw gwvideo.Storyboard
			gwErr := json.Unmarshal([]byte(tc.json), &gw)
			if gwErr == nil {
				gwErr = gw.Validate()
			}

			var w Storyboard
			wErr := json.Unmarshal([]byte(tc.json), &w)
			if wErr == nil {
				wErr = w.Validate()
			}

			// Both must agree: either both error or both pass
			gwFail := gwErr != nil
			wFail := wErr != nil
			if gwFail != wFail {
				t.Errorf("validate drift: gw=%v worker=%v for %q", gwErr, wErr, tc.name)
			}
		})
	}
}
