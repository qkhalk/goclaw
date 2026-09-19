// Package contract is a copy-shape of the gateway's internal/video types.
// The worker binary intentionally does NOT import internal/video; this keeps
// the sidecar independent (separate binary, separate compile graph). A
// golden-file test (golden_test.go) guards the two parsers against drift.
package contract

import (
	"encoding/json"
	"fmt"
	"math"
)

// Storyboard is the render contract (JSON) between the gateway and worker.
type Storyboard struct {
	Version int      `json:"version"`
	Canvas  Canvas   `json:"canvas"`
	Engine  string   `json:"engine,omitempty"` // default "ffmpeg"
	Scenes  []Scene  `json:"scenes"`
	Audio   AudioMix `json:"audio"`
	Output  Output   `json:"output"`
}

// Canvas is the render frame. Defaults: 1080x1920 (9:16) @30fps.
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
	Source      string     `json:"source,omitempty"` // workspace-relative path or URL
	Color       string     `json:"color,omitempty"`  // "#RRGGBB" for color scenes
	// Color2 is the optional second gradient stop of a color scene; empty
	// falls back to a darker shade of Color. Grid overlays a faint blueprint
	// grid; Glow/Vignette/Grain are the visual-v2 extras (all mirror
	// internal/video — keep the copy-shapes in sync).
	Color2      string     `json:"color2,omitempty"`
	Grid        bool       `json:"grid,omitempty"`
	Glow        string     `json:"glow,omitempty"`
	Vignette    bool       `json:"vignette,omitempty"`
	Grain       bool       `json:"grain,omitempty"`
	DurationSec float64    `json:"duration_sec"`
	Fit         string     `json:"fit,omitempty"` // cover|contain
	Mute        bool       `json:"mute,omitempty"`
	KenBurns    *KenBurns  `json:"ken_burns,omitempty"`
	Caption     *Caption   `json:"caption,omitempty"`
	Narration   *Narration `json:"narration,omitempty"`
	// Transition is the scene's enter transition (applied at the junction
	// with the previous scene): none|fade|crossfade|slide_left|slide_up.
	Transition string `json:"transition,omitempty"`
	// StylePack is a scene-level look shorthand: it expands to color/color2/
	// grid/glow (color scenes) and the default text/accent colors when the
	// scene doesn't set them individually. Enum: tech_dark (the current
	// default look), neon_lab, paper_light, bold_red. Empty = no pack.
	StylePack string `json:"style_pack,omitempty"`
	// Layers are timed overlays drawn on top of the base visual (and under
	// the caption), in array order. Optional.
	Layers []Layer `json:"layers,omitempty"`
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
	// scenes without these kinds render exactly as before.
	LayerCounter     LayerKind = "counter"      // number count-up (e.g. "20 tỷ")
	LayerToggleGrid  LayerKind = "toggle_grid"  // grid of switches flipping on/off
	LayerCompareBars LayerKind = "compare_bars" // two labeled bars growing to widths
	LayerStack       LayerKind = "stack"        // stacked slabs sliding in vertically
	LayerStamp       LayerKind = "stamp"        // rotated bordered stamp text
	LayerCTA         LayerKind = "cta"          // gradient pill with centered text at the bottom
)

// ValidStylePacks enumerates the scene-level look presets ("": none).
var ValidStylePacks = map[string]bool{
	"": true, "tech_dark": true, "neon_lab": true, "paper_light": true, "bold_red": true,
}

// ValidIcons is the canonical set of embedded icon names (Feather-style
// stroke glyphs, MIT). The worker embeds the SVG bodies under the same
// keys — cross-checked by a vworker test.
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

// ValidAnims enumerates layer entrance animations ("": instant).
var ValidAnims = map[string]bool{
	"": true, "fade": true, "up": true, "down": true,
	"left": true, "right": true, "pop": true,
}

// ValidFonts enumerates text-layer font styles ("": the default body font).
var ValidFonts = map[string]bool{
	"": true, "body": true, "display": true, "mono": true,
}

// TextHighlight colors every case-sensitive occurrence of Word inside a text
// layer's text with Color.
type TextHighlight struct {
	Word  string `json:"word"`
	Color string `json:"color"` // #RRGGBB
}

// Layer is one timed overlay inside a scene — copy-shape mirror of
// internal/video.Layer (drift-guarded by the golden fixture "layers").
type Layer struct {
	Kind     LayerKind `json:"kind"`
	Text     string    `json:"text,omitempty"`
	Source   string    `json:"source,omitempty"`
	Shape    string    `json:"shape,omitempty"`
	Icon     string    `json:"icon,omitempty"`
	Anim     string    `json:"anim,omitempty"`
	Font     string    `json:"font,omitempty"` // text layers: body (default) | display (bold) | mono
	Chip     bool      `json:"chip,omitempty"` // icon layers: tinted rounded tile behind the glyph
	Start    float64   `json:"start,omitempty"`
	Duration float64   `json:"duration,omitempty"`
	X        float64   `json:"x,omitempty"`
	Y        float64   `json:"y,omitempty"`
	W        float64   `json:"w,omitempty"`
	H        float64   `json:"h,omitempty"`
	Fill     string    `json:"fill,omitempty"`
	Opacity  float64   `json:"opacity,omitempty"`
	Radius   float64   `json:"radius,omitempty"`
	Border   bool      `json:"border,omitempty"` // card layers: contrast ring on the edge
	FontSize int       `json:"font_size,omitempty"`
	Align    string    `json:"align,omitempty"`

	// ── Motion-layer primitives (multi-form engine), all optional ──

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

// EffectiveStart resolves the layer's start (0 default) — mirror of
// internal/video.
func (l *Layer) EffectiveStart() float64 { return l.Start }

// EffectiveDuration resolves duration 0 = until the scene ends.
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
// cap 3×3 .. 4×4). Mirrored by internal/video and the browser painter.
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

// KenBurns animates a slow zoom/pan on image scenes.
type KenBurns struct {
	ZoomFrom float64 `json:"zoom_from,omitempty"`
	ZoomTo   float64 `json:"zoom_to,omitempty"`
	Pan      string  `json:"pan,omitempty"` // none|left|right|up|down
}

// UnmarshalJSON accepts the direction string and the coordinate object
// {from_x, from_y, to_x, to_y} (reduced to the dominant axis), mirroring
// internal/video.KenBurns — keep the two in sync.
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

// panFromBox reduces a pan box to the dominant movement direction. Mirrored
// by internal/video.
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

// Caption draws text over the scene (rendered as PNG overlays by the worker;
// with narration it becomes a karaoke reveal — mirror of internal/video).
type Caption struct {
	Text     string `json:"text"`
	Position string `json:"position,omitempty"` // top|center|bottom
	FontSize int    `json:"font_size,omitempty"`
	Style    string `json:"style,omitempty"` // "" plain | chip | mono
}

// Narration is the spoken text for the scene.
type Narration struct {
	Text     string `json:"text"`
	Provider string `json:"provider,omitempty"` // default "edge"
	Voice    string `json:"voice,omitempty"`    // e.g. "vi-VN-HoaiMyNeural"
}

// AudioMix configures the final audio graph.
type AudioMix struct {
	BGMPath         string  `json:"bgm_path,omitempty"`
	BGMVolume       float64 `json:"bgm_volume,omitempty"`
	NarrationVolume float64 `json:"narration_volume,omitempty"`
}

// Output describes the delivered file.
type Output struct {
	Format       string `json:"format,omitempty"`        // default mp4
	Height       int    `json:"height,omitempty"`        // 480|720|1080
	VideoBitrate string `json:"video_bitrate,omitempty"` // e.g. "1500k"
}

// Worker job API shapes.

// JobStatus is the worker-side lifecycle state.
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRendering JobStatus = "rendering"
	JobDone      JobStatus = "done"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// SubmitJob is the POST /v1/jobs request body.
type SubmitJob struct {
	JobID      string           `json:"jobId"`
	Storyboard *Storyboard      `json:"storyboard"`
	Narration  []NarrationAudio `json:"narration,omitempty"`
	AssetsDir  string           `json:"assetsDir,omitempty"`
}

// NarrationAudio binds one scene to its synthesized audio file.
type NarrationAudio struct {
	SceneIndex int    `json:"sceneIndex"`
	AudioPath  string `json:"audioPath"`
}

// SubmitJobResponse is the POST /v1/jobs reply.
type SubmitJobResponse struct {
	JobID  string    `json:"jobId"`
	Status JobStatus `json:"status"`
}

// JobState is the GET /v1/jobs/{id} reply.
type JobState struct {
	JobID           string    `json:"jobId"`
	Status          JobStatus `json:"status"`
	Progress        int       `json:"progress"` // 0-100
	Error           string    `json:"error,omitempty"`
	OutputPath      string    `json:"outputPath,omitempty"`
	OutputSizeBytes int64     `json:"outputSizeBytes,omitempty"`
	DurationMS      int64     `json:"durationMs,omitempty"`
}

// CancelResponse is the POST /v1/jobs/{id}/cancel reply.
type CancelResponse struct {
	JobID  string    `json:"jobId"`
	Status JobStatus `json:"status"`
}
