// Package video defines the Storyboard v1 contract shared by the gateway and
// the videoworker sidecar, plus the worker job API shapes. Types only — no
// rendering. The worker intentionally does NOT import this package (it keeps
// a copy-shaped parser in internal/vworker/contract so the sidecar binary
// stays independent); a golden-file test guards the two parsers against
// drift.
package video

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Storyboard is the render contract (JSON) between the agent/user, the
// gateway and the worker. Version 1 is deliberately minimal: scene-level
// composition only — no absolute positioning, no template system.
type Storyboard struct {
	Version int      `json:"version"`
	Canvas  Canvas   `json:"canvas"`
	Engine  string   `json:"engine,omitempty"` // default "ffmpeg"
	Scenes  []Scene  `json:"scenes"`
	Audio   AudioMix `json:"audio"`
	Output  Output   `json:"output"`
}

// Canvas is the render frame. Defaults: 1080x1920 (9:16) @30fps; capped at
// 1920 on the long edge by Validate to keep 1 vCPU renders feasible.
type Canvas struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	FPS    int `json:"fps,omitempty"`
}

// SceneKind enumerates the supported scene sources.
type SceneKind string

const (
	SceneImage SceneKind = "image"
	SceneVideo SceneKind = "video"
	SceneColor SceneKind = "color"
)

// Scene is one clip in the timeline.
type Scene struct {
	Type        SceneKind  `json:"type"`
	Source      string     `json:"source,omitempty"` // workspace-relative path or http(s) URL; empty for color
	Color       string     `json:"color,omitempty"`  // "#RRGGBB" for color scenes
	Color2      string     `json:"color2,omitempty"` // color scenes: second gradient stop; empty = darker shade of Color
	Grid        bool       `json:"grid,omitempty"`   // color scenes: overlay a faint blueprint grid
	// Visual v2 (color scenes): Glow adds one or two drifting radial glow
	// orbs tinted with this "#RRGGBB"; Vignette darkens the frame edges;
	// Grain adds subtle animated film grain. All optional — scenes without
	// them render exactly as before.
	Glow     string `json:"glow,omitempty"`
	Vignette bool   `json:"vignette,omitempty"`
	Grain    bool   `json:"grain,omitempty"`
	DurationSec float64    `json:"duration_sec"`
	Fit         string     `json:"fit,omitempty"` // cover|contain (default cover)
	Transition  string     `json:"transition,omitempty"` // enter transition: none|fade|crossfade|slide_left|slide_up
	Mute        bool       `json:"mute,omitempty"`
	KenBurns    *KenBurns  `json:"ken_burns,omitempty"`
	Caption     *Caption   `json:"caption,omitempty"`
	Narration   *Narration `json:"narration,omitempty"`
	// Layers are timed overlays drawn on top of the base visual (and under
	// the caption), in array order. Optional — scenes without layers render
	// exactly as before.
	Layers []Layer `json:"layers,omitempty"`
	// StylePack is a scene-level look shorthand: it expands to color/color2/
	// grid/glow (color scenes) and the default text/accent colors when the
	// scene doesn't set them individually. Enum: tech_dark (the current
	// default look), neon_lab, paper_light, bold_red. Empty = no pack.
	// Keep in sync with internal/vworker/contract (drift-guarded by the
	// golden fixtures).
	StylePack string `json:"style_pack,omitempty"`
}

// LayerKind enumerates the overlay layer kinds.
type LayerKind string

const (
	LayerText  LayerKind = "text"
	LayerShape LayerKind = "shape"
	LayerImage LayerKind = "image"
	LayerIcon  LayerKind = "icon"
	LayerCard  LayerKind = "card"
	// Motion-layer primitives (multi-form engine): each renders as a small
	// composed animation. All fields are optional with documented defaults;
	// scenes without these kinds render exactly as before. Mirrored in
	// internal/vworker/contract — the golden fixture test cross-checks.
	LayerCounter     LayerKind = "counter"      // number count-up (e.g. "20 tỷ")
	LayerToggleGrid  LayerKind = "toggle_grid"  // grid of switches flipping on/off
	LayerCompareBars LayerKind = "compare_bars" // two labeled bars growing to widths
	LayerStack       LayerKind = "stack"        // stacked slabs sliding in vertically
	LayerStamp       LayerKind = "stamp"        // rotated bordered stamp text
	LayerCTA         LayerKind = "cta"          // gradient pill with centered text at the bottom
)

// ValidStylePacks enumerates the scene-level look presets ("": none).
// Mirrored in internal/vworker/contract.
var ValidStylePacks = map[string]bool{
	"": true, "tech_dark": true, "neon_lab": true, "paper_light": true, "bold_red": true,
}

// ValidIcons is the canonical set of embedded icon names for icon layers
// (Feather-style stroke glyphs, MIT). The worker embeds the SVG bodies
// under the same keys. Mirrored in internal/vworker/contract — the golden
// fixture test cross-checks the two sets.
var ValidIcons = map[string]bool{
	"check": true, "zap": true, "users": true, "user": true,
	"cpu": true, "database": true, "git-branch": true, "globe": true,
	"heart": true, "star": true, "trending-up": true, "shield": true,
	"layers": true, "code": true, "terminal": true, "book-open": true,
	"message-circle": true, "clock": true, "eye": true, "lock": true,
	"package": true, "settings": true, "bar-chart-2": true,
	"arrow-right": true, "download": true, "play": true, "target": true,
	"search": true, "calendar": true, "camera": true, "music": true,
	"wifi": true, "cloud": true, "coffee": true,
}

// ValidAnims enumerates layer entrance animations ("": instant). Mirrored in
// internal/vworker/contract.
var ValidAnims = map[string]bool{
	"": true, "fade": true, "up": true, "down": true,
	"left": true, "right": true, "pop": true,
}

// ValidFonts enumerates text-layer font styles ("": the default body font).
// Mirrored in internal/vworker/contract.
var ValidFonts = map[string]bool{
	"": true, "body": true, "display": true, "mono": true,
}

// TextHighlight colors every case-sensitive occurrence of Word inside a text
// layer's text with Color. Mirrored in internal/vworker/contract.
type TextHighlight struct {
	Word  string `json:"word"`
	Color string `json:"color"` // #RRGGBB
}

// Layer is one timed overlay inside a scene. Geometry is normalized to the
// canvas (0..1, top-left origin) so a storyboard is resolution-independent;
// the worker and the browser preview resolve x/y/w/h against the same
// effective canvas. A layer is visible while start <= t < start+duration
// (duration 0 = until the scene ends).
type Layer struct {
	Kind     LayerKind `json:"kind"`
	Text     string    `json:"text,omitempty"`      // text layers
	Source   string    `json:"source,omitempty"`    // image layers: workspace-relative path or http(s) URL
	Shape    string    `json:"shape,omitempty"`     // shape layers: rect
	Icon     string    `json:"icon,omitempty"`      // icon layers: one of ValidIcons
	Anim     string    `json:"anim,omitempty"`      // entrance animation: fade|up|down|left|right|pop (default none)
	Font     string    `json:"font,omitempty"`      // text layers: body (default) | display (bold) | mono
	Chip     bool      `json:"chip,omitempty"`      // icon layers: tinted tile behind the glyph
	Start    float64   `json:"start,omitempty"`     // seconds into the scene (default 0)
	Duration float64   `json:"duration,omitempty"`  // seconds (0 = to scene end)
	X        float64   `json:"x,omitempty"`         // 0..1 (default 0.1)
	Y        float64   `json:"y,omitempty"`         // 0..1 (default 0.1)
	W        float64   `json:"w,omitempty"`         // 0..1 width (default 0.8)
	H        float64   `json:"h,omitempty"`         // 0..1 height, shape/card layers (default 0.3; icons default square)
	Fill     string    `json:"fill,omitempty"`      // #RRGGBB — text color / shape fill / icon stroke / card fill (default white)
	Opacity  float64   `json:"opacity,omitempty"`   // 0..1 (default 1; cards usually 0.08..0.25)
	Radius   float64   `json:"radius,omitempty"`    // card corner radius, 0..0.2 of canvas width (default 0.018)
	Border   bool      `json:"border,omitempty"`    // card layers: contrast ring on the edge
	FontSize int       `json:"font_size,omitempty"` // text layers (default 48)
	Align    string    `json:"align,omitempty"`     // left|center|right within the box (default center)

	// ── Motion-layer primitives (multi-form engine), all optional.
	// Mirrored in internal/vworker/contract — keep the copy-shapes in sync.

	// Highlights color specific words of a text layer (the colored-keyword
	// headline). Rendered as one PNG overlay server-side, segment drawing in
	// the browser painter.
	Highlights []TextHighlight `json:"highlights,omitempty"`
	// counter: counts from From up to To over the layer window. Text is the
	// optional prefix, Suffix the optional suffix (e.g. text "▲ ", to 20,
	// suffix " tỷ"). Decimals 0..2.
	From     float64 `json:"from,omitempty"`
	To       float64 `json:"to,omitempty"`
	Suffix   string  `json:"suffix,omitempty"`
	Decimals int     `json:"decimals,omitempty"`
	// toggle_grid: Cols×Rows switches (1..4 each, default 3×3) flipping on/off
	// every Cadence seconds (0.2..2, default 0.6). Fill = on color, FillB =
	// off color.
	Cols    int     `json:"cols,omitempty"`
	Rows    int     `json:"rows,omitempty"`
	Cadence float64 `json:"cadence,omitempty"`
	// compare_bars: two labeled bars growing to WidthA/WidthB (0..1 of the
	// box width, defaults 0.62/0.38). Fill = bar A, FillB = bar B.
	LabelA string   `json:"label_a,omitempty"`
	LabelB string   `json:"label_b,omitempty"`
	WidthA float64  `json:"width_a,omitempty"`
	WidthB float64  `json:"width_b,omitempty"`
	FillB  string   `json:"fill_b,omitempty"` // secondary color (off/bar B/gradient end)
	// stack: N slabs (1..6, default 3) sliding in vertically, staggered;
	// Labels are optional per-slab strings. Fill → FillB is the slab gradient.
	N      int      `json:"n,omitempty"`
	Labels []string `json:"labels,omitempty"`
	// stamp: rotation angle in degrees, -30..30 (default -8).
	Angle float64 `json:"angle,omitempty"`
}

// KenBurns animates a slow zoom/pan on image scenes.
type KenBurns struct {
	ZoomFrom float64 `json:"zoom_from,omitempty"` // default 1.0
	ZoomTo   float64 `json:"zoom_to,omitempty"`   // default 1.0 (no zoom)
	Pan      string  `json:"pan,omitempty"`       // none|left|right|up|down
}

// UnmarshalJSON accepts the documented direction string ("left", …) and also
// the coordinate object {from_x, from_y, to_x, to_y} that LLM-authored
// storyboards sometimes emit — the object is reduced to its dominant axis so
// a near-miss storyboard still renders instead of failing validation.
// Keep in sync with internal/vworker/contract (drift-guarded by the golden
// fixture "kenburns_pan_object").
func (k *KenBurns) UnmarshalJSON(data []byte) error {
	var raw struct {
		ZoomFrom *float64        `json:"zoom_from"`
		ZoomTo   *float64        `json:"zoom_to"`
		Pan      json.RawMessage `json:"pan"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ZoomFrom != nil {
		k.ZoomFrom = *raw.ZoomFrom
	}
	if raw.ZoomTo != nil {
		k.ZoomTo = *raw.ZoomTo
	}
	if len(raw.Pan) == 0 || string(raw.Pan) == "null" {
		return nil
	}
	var dir string
	if err := json.Unmarshal(raw.Pan, &dir); err == nil {
		k.Pan = dir
		return nil
	}
	var box struct {
		FromX *float64 `json:"from_x"`
		FromY *float64 `json:"from_y"`
		ToX   *float64 `json:"to_x"`
		ToY   *float64 `json:"to_y"`
	}
	if err := json.Unmarshal(raw.Pan, &box); err != nil {
		return fmt.Errorf(`ken_burns.pan: want a direction string or a {from_x,from_y,to_x,to_y} object: %s`, truncateJSON(raw.Pan))
	}
	k.Pan = panFromBox(box.FromX, box.FromY, box.ToX, box.ToY)
	return nil
}

// panFromBox reduces a pan box to the dominant movement direction. Sub-0.2%
// offsets count as no pan; ties favor the horizontal axis. Mirrored by
// internal/vworker/contract.
func panFromBox(fromX, fromY, toX, toY *float64) string {
	var from, to [2]float64
	if fromX != nil {
		from[0] = *fromX
	}
	if fromY != nil {
		from[1] = *fromY
	}
	if toX != nil {
		to[0] = *toX
	}
	if toY != nil {
		to[1] = *toY
	}
	dx := to[0] - from[0]
	dy := to[1] - from[1]
	ax, ay := math.Abs(dx), math.Abs(dy)
	if ax < 0.002 && ay < 0.002 {
		return "none"
	}
	if ax >= ay {
		if dx < 0 {
			return "left"
		}
		return "right"
	}
	if dy < 0 {
		return "up"
	}
	return "down"
}

func truncateJSON(b json.RawMessage) string {
	s := string(b)
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

// Caption draws text over the scene. The worker renders captions with the
// bundled display font as PNG overlays: with narration the caption becomes a
// karaoke reveal (words light up in sync with the voice); "chip" puts the text
// on a rounded dark chip and "mono" uses the bundled monospace eyebrow font.
type Caption struct {
	Text     string `json:"text"`
	Position string `json:"position,omitempty"` // top|center|bottom (default bottom)
	FontSize int    `json:"font_size,omitempty"`
	Style    string `json:"style,omitempty"` // "" plain | chip | mono
}

// Narration is the spoken text for the scene. Phase 4 wires the actual
// synthesis providers; the contract is fixed here.
type Narration struct {
	Text     string `json:"text"`
	Provider string `json:"provider,omitempty"` // default "edge"
	Voice    string `json:"voice,omitempty"`    // e.g. "vi-VN-HoaiMyNeural"
}

// AudioMix configures the final audio graph.
type AudioMix struct {
	BGMPath         string  `json:"bgm_path,omitempty"`         // workspace-relative or URL
	BGMVolume       float64 `json:"bgm_volume,omitempty"`       // default 0.2
	NarrationVolume float64 `json:"narration_volume,omitempty"` // default 1.0
}

// Output describes the delivered file. Defaults: mp4, 720p height, 1500k.
type Output struct {
	Format       string `json:"format,omitempty"`        // default mp4
	Height       int    `json:"height,omitempty"`        // 480|720|1080 (default 720)
	VideoBitrate string `json:"video_bitrate,omitempty"` // e.g. "1500k"
}

// Aspect presets for callers that think in ratios rather than pixels.
const (
	AspectVertical = "9:16"
	AspectWide     = "16:9"
	AspectSquare   = "1:1"
)

// DimensionsFor returns the canvas size for a named aspect at the given
// (short-edge) height. Used by the gateway when mapping anh's aspect presets
// onto Canvas; unknown aspects return ok=false.
func DimensionsFor(aspect string, height int) (w, h int, ok bool) {
	if height <= 0 {
		height = 720
	}
	switch aspect {
	case AspectVertical:
		return height * 9 / 16, height, true
	case AspectWide:
		return height * 16 / 9, height, true
	case AspectSquare:
		return height, height, true
	}
	return 0, 0, false
}

const (
	maxScenes      = 60
	maxSceneSec    = 30.0
	maxTotalSec    = 600.0
	maxCanvasEdge  = 1920
	defaultFPS     = 30
	defaultHeight  = 720
	allowedHeights = "480, 720 or 1080"
	maxLayers      = 8
	// maxHighlights caps colored-keyword entries on one text layer.
	maxHighlights = 6
)

// Validate checks the storyboard against the v1 constraints. It is shared by
// the gateway (enqueue-time) and mirrored (copy-shape) by the worker.
func (s *Storyboard) Validate() error {
	if s.Version != 1 {
		return fmt.Errorf("video: unsupported storyboard version %d (want 1)", s.Version)
	}
	if len(s.Scenes) < 1 || len(s.Scenes) > maxScenes {
		return fmt.Errorf("video: scenes must be 1..%d, got %d", maxScenes, len(s.Scenes))
	}
	total := 0.0
	for i := range s.Scenes {
		sc := &s.Scenes[i]
		if err := sc.validate(); err != nil {
			return fmt.Errorf("video: scene %d: %w", i, err)
		}
		total += sc.DurationSec
	}
	if total > maxTotalSec {
		return fmt.Errorf("video: total duration %.0fs exceeds the %ds cap", total, int(maxTotalSec))
	}
	w, h, fps := s.Canvas.effective()
	if w <= 0 || h <= 0 || w > maxCanvasEdge || h > maxCanvasEdge {
		return fmt.Errorf("video: canvas %dx%d exceeds the %dpx edge cap", w, h, maxCanvasEdge)
	}
	if fps <= 0 || fps > 60 {
		return fmt.Errorf("video: fps %d out of range 1..60", fps)
	}
	if s.Output.Height != 0 {
		switch s.Output.Height {
		case 480, 720, 1080:
		default:
			return fmt.Errorf("video: output height must be %s", allowedHeights)
		}
	}
	if f := s.Output.Format; f != "" && f != "mp4" {
		return fmt.Errorf("video: output format %q not supported (mp4 only in v1)", f)
	}
	return nil
}

func (sc *Scene) validate() error {
	switch sc.Type {
	case SceneImage, SceneVideo:
		if strings.TrimSpace(sc.Source) == "" {
			return fmt.Errorf("source is required for %s scenes", sc.Type)
		}
	case SceneColor:
		// An explicit color/color2/glow must be #RRGGBB; with a style_pack an
		// unset color is fine — the pack supplies the backdrop. Mirrored by
		// internal/vworker/contract.
		if sc.Color == "" && sc.StylePack == "" {
			return fmt.Errorf("color scenes need a #RRGGBB color (or a style_pack)")
		}
		if sc.Color != "" && !hexColor(sc.Color) {
			return fmt.Errorf("color scenes need a #RRGGBB color, got %q", sc.Color)
		}
		if sc.Color2 != "" && !hexColor(sc.Color2) {
			return fmt.Errorf("color2 must be #RRGGBB, got %q", sc.Color2)
		}
		if sc.Glow != "" && !hexColor(sc.Glow) {
			return fmt.Errorf("glow must be #RRGGBB, got %q", sc.Glow)
		}
	default:
		return fmt.Errorf("unknown scene type %q", sc.Type)
	}
	if !ValidStylePacks[sc.StylePack] {
		return fmt.Errorf("unknown style_pack %q (tech_dark, neon_lab, paper_light, bold_red)", sc.StylePack)
	}
	if sc.Caption != nil {
		switch sc.Caption.Style {
		case "", "chip", "mono":
		default:
			return fmt.Errorf("caption style %q not supported (chip, mono)", sc.Caption.Style)
		}
	}
	if sc.DurationSec < 1 || sc.DurationSec > maxSceneSec {
		return fmt.Errorf("duration_sec %.1f out of range 1..%.0f", sc.DurationSec, maxSceneSec)
	}
	if sc.Transition != "" && sc.Transition != "none" {
		switch sc.Transition {
		case "fade", "crossfade", "slide_left", "slide_up":
		default:
			return fmt.Errorf("transition %q not supported (none, fade, crossfade, slide_left, slide_up)", sc.Transition)
		}
	}
	if len(sc.Layers) > maxLayers {
		return fmt.Errorf("at most %d layers per scene, got %d", maxLayers, len(sc.Layers))
	}
	for j := range sc.Layers {
		if err := sc.Layers[j].validate(sc.DurationSec); err != nil {
			return fmt.Errorf("layer %d: %w", j, err)
		}
	}
	return nil
}

// validate checks one layer against its host scene's duration. Mirrored by
// internal/vworker/contract (drift-guarded by the golden fixture "layers").
func (l *Layer) validate(sceneSec float64) error {
	switch l.Kind {
	case LayerText:
		if strings.TrimSpace(l.Text) == "" {
			return fmt.Errorf("text layers need text")
		}
		if len(l.Highlights) > maxHighlights {
			return fmt.Errorf("at most %d highlights per text layer, got %d", maxHighlights, len(l.Highlights))
		}
		for k, hl := range l.Highlights {
			if strings.TrimSpace(hl.Word) == "" {
				return fmt.Errorf("highlight %d needs a word", k)
			}
			if !hexColor(hl.Color) {
				return fmt.Errorf("highlight %d color must be #RRGGBB, got %q", k, hl.Color)
			}
		}
	case LayerShape:
		if l.Shape == "" {
			l.Shape = "rect"
		}
		if l.Shape != "rect" {
			return fmt.Errorf("shape %q not supported (rect)", l.Shape)
		}
		if !hexColor(l.Fill) {
			return fmt.Errorf("shape layers need a #RRGGBB fill, got %q", l.Fill)
		}
	case LayerImage:
		if strings.TrimSpace(l.Source) == "" {
			return fmt.Errorf("image layers need source")
		}
	case LayerIcon:
		if !ValidIcons[l.Icon] {
			return fmt.Errorf("unknown icon %q (see ValidIcons for the embedded set)", l.Icon)
		}
	case LayerCard:
		if !hexColor(l.Fill) {
			return fmt.Errorf("card layers need a #RRGGBB fill, got %q", l.Fill)
		}
	case LayerCounter:
		if l.To <= 0 {
			return fmt.Errorf("counter layers need to > 0 (count-up target)")
		}
		if l.From < 0 || l.From >= l.To {
			return fmt.Errorf("counter from must satisfy 0 <= from < to (got from=%.3g, to=%.3g)", l.From, l.To)
		}
		if l.Decimals < 0 || l.Decimals > 2 {
			return fmt.Errorf("counter decimals %d out of range 0..2", l.Decimals)
		}
	case LayerToggleGrid:
		if l.Cols < 0 || l.Cols > 4 || l.Rows < 0 || l.Rows > 4 {
			return fmt.Errorf("toggle_grid cols/rows must be 0 (default 3) or 1..4, got %d×%d", l.Cols, l.Rows)
		}
		if l.Cadence < 0 || l.Cadence > 2 {
			return fmt.Errorf("toggle_grid cadence must be 0 (default 0.6) or 0.2..2, got %g", l.Cadence)
		}
		if l.Fill != "" && !hexColor(l.Fill) {
			return fmt.Errorf("toggle_grid fill must be #RRGGBB, got %q", l.Fill)
		}
		if l.FillB != "" && !hexColor(l.FillB) {
			return fmt.Errorf("toggle_grid fill_b must be #RRGGBB, got %q", l.FillB)
		}
	case LayerCompareBars:
		if l.WidthA < 0 || l.WidthA > 1 {
			return fmt.Errorf("compare_bars width_a must be within 0..1 (0 = default), got %g", l.WidthA)
		}
		if l.WidthB < 0 || l.WidthB > 1 {
			return fmt.Errorf("compare_bars width_b must be within 0..1 (0 = default), got %g", l.WidthB)
		}
		if l.Fill != "" && !hexColor(l.Fill) {
			return fmt.Errorf("compare_bars fill must be #RRGGBB, got %q", l.Fill)
		}
		if l.FillB != "" && !hexColor(l.FillB) {
			return fmt.Errorf("compare_bars fill_b must be #RRGGBB, got %q", l.FillB)
		}
	case LayerStack:
		if l.N < 0 || l.N > 6 {
			return fmt.Errorf("stack n must be 0 (default 3) or 1..6, got %d", l.N)
		}
		if len(l.Labels) > 6 {
			return fmt.Errorf("at most 6 stack labels, got %d", len(l.Labels))
		}
		if l.Fill != "" && !hexColor(l.Fill) {
			return fmt.Errorf("stack fill must be #RRGGBB, got %q", l.Fill)
		}
		if l.FillB != "" && !hexColor(l.FillB) {
			return fmt.Errorf("stack fill_b must be #RRGGBB, got %q", l.FillB)
		}
	case LayerStamp:
		if strings.TrimSpace(l.Text) == "" {
			return fmt.Errorf("stamp layers need text")
		}
		if l.Angle < -30 || l.Angle > 30 {
			return fmt.Errorf("stamp angle %.1f out of range -30..30", l.Angle)
		}
	case LayerCTA:
		if strings.TrimSpace(l.Text) == "" {
			return fmt.Errorf("cta layers need text")
		}
		if l.Fill != "" && !hexColor(l.Fill) {
			return fmt.Errorf("cta fill must be #RRGGBB, got %q", l.Fill)
		}
		if l.FillB != "" && !hexColor(l.FillB) {
			return fmt.Errorf("cta fill_b must be #RRGGBB, got %q", l.FillB)
		}
	default:
		return fmt.Errorf("unknown layer kind %q (text, shape, image, icon, card, counter, toggle_grid, compare_bars, stack, stamp, cta)", l.Kind)
	}
	if !ValidAnims[l.Anim] {
		return fmt.Errorf("unknown anim %q (fade, up, down, left, right, pop)", l.Anim)
	}
	if !ValidFonts[l.Font] {
		return fmt.Errorf("unknown font %q (body, display, mono)", l.Font)
	}
	if l.Radius < 0 || l.Radius > 0.2 {
		return fmt.Errorf("radius %.3f out of range 0..0.2 (fraction of canvas width)", l.Radius)
	}
	if l.Start < 0 || l.Start >= sceneSec {
		return fmt.Errorf("start %.2fs out of range 0..%.2f", l.Start, sceneSec)
	}
	dur := l.Duration
	if dur == 0 {
		dur = sceneSec - l.Start
	}
	if dur <= 0 {
		return fmt.Errorf("duration %.2fs must be positive", l.Duration)
	}
	if l.Start+dur > sceneSec+1e-9 {
		return fmt.Errorf("layer ends at %.2fs, past the scene's %.2fs", l.Start+dur, sceneSec)
	}
	if !unitRange(l.X) || !unitRange(l.Y) {
		return fmt.Errorf("x/y must be within 0..1 (got %.2f, %.2f)", l.X, l.Y)
	}
	if !unitRange(l.W) || l.W <= 0 {
		return fmt.Errorf("w must be within 0..1 and positive (got %.2f)", l.W)
	}
	if l.Kind == LayerShape && (!unitRange(l.H) || l.H <= 0) {
		return fmt.Errorf("shape layers need h within 0..1 and positive (got %.2f)", l.H)
	}
	if !unitRange(l.Opacity) {
		return fmt.Errorf("opacity must be within 0..1 (got %.2f)", l.Opacity)
	}
	if l.FontSize < 0 || l.FontSize > 300 {
		return fmt.Errorf("font_size %d out of range 0..300", l.FontSize)
	}
	switch l.Align {
	case "", "left", "center", "right":
	default:
		return fmt.Errorf("align %q not supported (left, center, right)", l.Align)
	}
	return nil
}

func unitRange(v float64) bool { return v >= 0 && v <= 1 }

// EffectiveStart/EffectiveDuration resolve the layer's timing defaults.
func (l *Layer) EffectiveStart() float64 { return l.Start }

func (l *Layer) EffectiveDuration(sceneSec float64) float64 {
	if l.Duration > 0 {
		return l.Duration
	}
	return sceneSec - l.Start
}

// EffectiveStyle resolves the style defaults (fill white, opacity 1,
// font_size 48, align center).
func (l *Layer) EffectiveStyle() (fill string, opacity float64, fontSize int, align string) {
	fill = l.Fill
	if fill == "" {
		fill = "#FFFFFF"
	}
	opacity = l.Opacity
	if opacity == 0 {
		opacity = 1
	}
	fontSize = l.FontSize
	if fontSize == 0 {
		fontSize = 48
	}
	align = l.Align
	if align == "" {
		align = "center"
	}
	return fill, opacity, fontSize, align
}

// EffectiveBox resolves the geometry defaults (x/y 0.1, w 0.8, h 0.3).
// Kind-specific h defaults keep the motion primitives sensible bare:
// toggle_grid derives square cells from cols/rows, compare_bars 0.24,
// stack 0.44, stamp 0.42·w and cta 0.12 (y defaults 0.8 — the bottom pill).
func (l *Layer) EffectiveBox() (x, y, w, h float64) {
	x, y, w, h = l.X, l.Y, l.W, l.H
	if x == 0 {
		x = 0.1
	}
	if y == 0 {
		if l.Kind == LayerCTA {
			y = 0.8
		} else {
			y = 0.1
		}
	}
	if w == 0 {
		w = 0.8
	}
	if h == 0 {
		switch l.Kind {
		case LayerShape, LayerCard:
			h = 0.3
		case LayerToggleGrid:
			h = w * (float64(l.EffectiveRows()) / float64(l.EffectiveCols()))
		case LayerCompareBars:
			h = 0.24
		case LayerStack:
			h = 0.44
		case LayerStamp:
			h = w * 0.42
		case LayerCTA:
			h = 0.12
		}
	}
	// Icons default to a square box — their SVG source is square.
	if h == 0 && l.Kind == LayerIcon {
		h = w
	}
	return x, y, w, h
}

// EffectiveCols/EffectiveRows resolve the toggle_grid grid size (default and
// cap 3×3 .. 4×4). Mirrored by internal/vworker/contract and the browser
// painter.
func (l *Layer) EffectiveCols() int {
	if l.Cols < 1 {
		return 3
	}
	return min(l.Cols, 4)
}

func (l *Layer) EffectiveRows() int {
	if l.Rows < 1 {
		return 3
	}
	return min(l.Rows, 4)
}

// EffectiveCadence resolves the toggle flip period (default 0.6s, clamped
// 0.2..2 so a scene can't machine-gun the cells).
func (l *Layer) EffectiveCadence() float64 {
	if l.Cadence <= 0 {
		return 0.6
	}
	return min(max(l.Cadence, 0.2), 2)
}

// EffectiveN resolves the stack slab count (default 3, cap 6).
func (l *Layer) EffectiveN() int {
	if l.N < 1 {
		return 3
	}
	return min(l.N, 6)
}

// EffectiveWidthA/EffectiveWidthB resolve the compare_bars bar widths as
// fractions of the box width (defaults 0.62 / 0.38).
func (l *Layer) EffectiveWidthA() float64 {
	if l.WidthA <= 0 {
		return 0.62
	}
	return min(l.WidthA, 1)
}

func (l *Layer) EffectiveWidthB() float64 {
	if l.WidthB <= 0 {
		return 0.38
	}
	return min(l.WidthB, 1)
}

// effective fills canvas defaults (1080x1920@30) — receiver semantics so the
// worker parser can mirror the exact same logic.
func (c Canvas) effective() (w, h, fps int) {
	w, h, fps = c.Width, c.Height, c.FPS
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 1920
	}
	if fps <= 0 {
		fps = defaultFPS
	}
	return w, h, fps
}

func hexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// EffectiveOutput returns output defaults applied (mp4/720p/1500k).
func (s *Storyboard) EffectiveOutput() (format string, height int, bitrate string) {
	format = s.Output.Format
	if format == "" {
		format = "mp4"
	}
	height = s.Output.Height
	if height == 0 {
		height = defaultHeight
	}
	bitrate = s.Output.VideoBitrate
	if bitrate == "" {
		bitrate = "1500k"
	}
	return format, height, bitrate
}
