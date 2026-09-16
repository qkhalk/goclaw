package video

import (
	"encoding/json"
	"testing"
)

func TestKenBurnsUnmarshalPanString(t *testing.T) {
	var kb KenBurns
	if err := json.Unmarshal([]byte(`{"zoom_from":1,"zoom_to":1.12,"pan":"left"}`), &kb); err != nil {
		t.Fatalf("string pan: %v", err)
	}
	if kb.Pan != "left" || kb.ZoomTo != 1.12 {
		t.Fatalf("got %+v", kb)
	}
}

func TestKenBurnsUnmarshalPanObject(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"dominant_x_right", `{"pan":{"from_x":0,"from_y":0,"to_x":0.03,"to_y":0.02}}`, "right"},
		{"dominant_x_left", `{"pan":{"from_x":0.02,"to_x":-0.01}}`, "left"},
		{"dominant_y_up", `{"pan":{"from_y":0.03,"to_y":0.01}}`, "up"},
		{"dominant_y_down", `{"pan":{"from_y":-0.01,"to_y":0.04}}`, "down"},
		{"tie_favors_x", `{"pan":{"from_x":0,"from_y":0,"to_x":-0.02,"to_y":0.02}}`, "left"},
		{"sub_threshold", `{"pan":{"from_x":0,"from_y":0,"to_x":0.001,"to_y":-0.001}}`, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var kb KenBurns
			if err := json.Unmarshal([]byte(tc.json), &kb); err != nil {
				t.Fatalf("object pan: %v", err)
			}
			if kb.Pan != tc.want {
				t.Fatalf("pan = %q, want %q", kb.Pan, tc.want)
			}
		})
	}
}

func TestKenBurnsUnmarshalPanRejectsGarbage(t *testing.T) {
	var kb KenBurns
	err := json.Unmarshal([]byte(`{"pan":{"from_x":"left"}}`), &kb)
	if err == nil {
		t.Fatal("expected error for non-numeric pan object")
	}
}
