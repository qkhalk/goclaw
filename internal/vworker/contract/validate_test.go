package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

// parseMotion builds a one-scene storyboard hosting the given layers and
// parses it.
func parseMotion(t *testing.T, layers, stylePack string) *Storyboard {
	t.Helper()
	pack := ""
	if stylePack != "" {
		pack = `,"style_pack":"` + stylePack + `"`
	}
	raw := `{"version":1,"scenes":[{"type":"color","color":"#0f172a","duration_sec":6` + pack +
		`,"layers":[` + layers + `]}]}`
	var sb Storyboard
	if err := json.Unmarshal([]byte(raw), &sb); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return &sb
}

// TestValidateMotionKindsAccepted — happy paths for every new primitive.
func TestValidateMotionKindsAccepted(t *testing.T) {
	cases := []string{
		`{"kind":"counter","to":20,"suffix":" tỷ","w":0.6,"y":0.3}`,
		`{"kind":"counter","text":"▲ ","from":5,"to":99.5,"decimals":1,"w":0.6}`,
		`{"kind":"toggle_grid","cols":4,"rows":2,"cadence":1.5,"w":0.6}`,
		`{"kind":"toggle_grid","w":0.8}`, // all defaults
		`{"kind":"compare_bars","label_a":"CPU","label_b":"GPU","width_a":0.7,"width_b":0.4,"w":0.8}`,
		`{"kind":"stack","n":5,"labels":["a","b","c","d","e"],"w":0.7}`,
		`{"kind":"stamp","text":"MỚI","angle":-30,"w":0.4}`,
		`{"kind":"cta","text":"Xem ngay","fill":"#38BDF8","fill_b":"#8B5CF6","w":0.7}`,
		`{"kind":"text","text":"Chip giảm 30 giá","w":0.8,"highlights":[{"word":"30","color":"#F97316"}]}`,
	}
	for _, layer := range cases {
		sb := parseMotion(t, layer, "")
		if err := sb.Validate(); err != nil {
			t.Errorf("Validate(%s) = %v, want nil", layer, err)
		}
	}
}

// TestValidateMotionKindsRejected — sad paths: ranges and required fields.
func TestValidateMotionKindsRejected(t *testing.T) {
	cases := []struct {
		layer string
		want  string
	}{
		{`{"kind":"counter","to":0}`, "counter layers need to > 0"},
		{`{"kind":"counter","from":10,"to":5}`, "0 <= from < to"},
		{`{"kind":"counter","to":5,"decimals":3}`, "decimals 3 out of range"},
		{`{"kind":"toggle_grid","cols":5}`, "cols/rows"},
		{`{"kind":"toggle_grid","cadence":5}`, "cadence"},
		{`{"kind":"toggle_grid","fill":"red"}`, "fill must be #RRGGBB"},
		{`{"kind":"compare_bars","width_a":1.4}`, "width_a"},
		{`{"kind":"stack","n":7}`, "stack n"},
		{`{"kind":"stack","labels":["1","2","3","4","5","6","7"]}`, "at most 6 stack labels"},
		{`{"kind":"stamp","text":"MỚI","angle":45}`, "angle 45.0 out of range"},
		{`{"kind":"stamp"}`, "stamp layers need text"},
		{`{"kind":"cta"}`, "cta layers need text"},
		{`{"kind":"cta","text":"ok","fill":"#12345"}`, "fill must be #RRGGBB"},
		{`{"kind":"text","text":"a","highlights":[{"word":"","color":"#FF0000"}]}`, "needs a word"},
		{`{"kind":"text","text":"a","highlights":[{"word":"a","color":"nope"}]}`, "color must be #RRGGBB"},
		{`{"kind":"text","text":"a","highlights":[{"word":"1","color":"#FF0000"},{"word":"2","color":"#FF0000"},{"word":"3","color":"#FF0000"},{"word":"4","color":"#FF0000"},{"word":"5","color":"#FF0000"},{"word":"6","color":"#FF0000"},{"word":"7","color":"#FF0000"}]}`, "at most 6 highlights"},
		{`{"kind":"hologram"}`, "unknown layer kind"},
	}
	for _, tc := range cases {
		sb := parseMotion(t, tc.layer, "")
		err := sb.Validate()
		if err == nil {
			t.Errorf("Validate(%s) = nil, want error containing %q", tc.layer, tc.want)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Validate(%s) = %q, want containing %q", tc.layer, err, tc.want)
		}
	}
}

// TestValidateStylePacks — enum enforced, pack supplies the color.
func TestValidateStylePacks(t *testing.T) {
	for _, pack := range []string{"tech_dark", "neon_lab", "paper_light", "bold_red"} {
		sb := parseMotion(t, "", pack)
		if err := sb.Validate(); err != nil {
			t.Errorf("style_pack %q: %v", pack, err)
		}
	}
	// Unknown pack rejected.
	sb := parseMotion(t, "", "cyberpunk")
	if err := sb.Validate(); err == nil || !strings.Contains(err.Error(), "unknown style_pack") {
		t.Errorf("Validate(style_pack=cyberpunk) = %v, want unknown style_pack error", err)
	}
	// Color scene without color or pack still rejected (missing-color error).
	var raw Storyboard
	if err := json.Unmarshal([]byte(`{"version":1,"scenes":[{"type":"color","duration_sec":3}]}`), &raw); err != nil {
		t.Fatal(err)
	}
	if err := raw.Validate(); err == nil || !strings.Contains(err.Error(), "#RRGGBB color") {
		t.Errorf("Validate(colorless scene) = %v, want missing-color error", err)
	}
}

// TestEffectiveBoxMotionDefaults pins the kind-specific geometry defaults the
// browser painter mirrors.
func TestEffectiveBoxMotionDefaults(t *testing.T) {
	tg := Layer{Kind: LayerToggleGrid}
	x, y, w, h := tg.EffectiveBox()
	if x != 0.1 || y != 0.1 || w != 0.8 || h != 0.8 { // 3×3 square cells
		t.Errorf("toggle_grid box = %v %v %v %v, want 0.1 0.1 0.8 0.8", x, y, w, h)
	}
	cta := Layer{Kind: LayerCTA}
	_, y, _, h = cta.EffectiveBox()
	if y != 0.8 || h != 0.12 {
		t.Errorf("cta y/h = %v %v, want 0.8 0.12", y, h)
	}
	st := Layer{Kind: LayerStack}
	_, _, _, h = st.EffectiveBox()
	if h != 0.44 {
		t.Errorf("stack h = %v, want 0.44", h)
	}
	cb := Layer{Kind: LayerCompareBars}
	_, _, _, h = cb.EffectiveBox()
	if h != 0.24 {
		t.Errorf("compare_bars h = %v, want 0.24", h)
	}
	sp := Layer{Kind: LayerStamp}
	_, _, _, h = sp.EffectiveBox()
	if h != 0.8*0.42 {
		t.Errorf("stamp h = %v, want %v", h, 0.8*0.42)
	}
	// Resolver clamps.
	tg2 := Layer{Kind: LayerToggleGrid, Cols: 9, Rows: 0, Cadence: 99}
	if got := tg2.EffectiveCols(); got != 4 {
		t.Errorf("EffectiveCols(9) = %d, want 4", got)
	}
	if got := tg2.EffectiveRows(); got != 3 {
		t.Errorf("EffectiveRows(0) = %d, want 3", got)
	}
	if got := tg2.EffectiveCadence(); got != 2 {
		t.Errorf("EffectiveCadence(99) = %v, want 2", got)
	}
	st2 := Layer{Kind: LayerStack, N: 99}
	if got := st2.EffectiveN(); got != 6 {
		t.Errorf("EffectiveN(99) = %d, want 6", got)
	}
	cb2 := Layer{Kind: LayerCompareBars, WidthA: 2, WidthB: 0}
	if got := cb2.EffectiveWidthA(); got != 1 {
		t.Errorf("EffectiveWidthA(2) = %v, want 1", got)
	}
	if got := cb2.EffectiveWidthB(); got != 0.38 {
		t.Errorf("EffectiveWidthB(0) = %v, want 0.38", got)
	}
}
