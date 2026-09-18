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
)

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

// Layer is one timed overlay inside a scene — copy-shape mirror of
// internal/video.Layer (drift-guarded by the golden fixture "layers").
type Layer struct {
	Kind     LayerKind `json:"kind"`
	Text     string    `json:"text,omitempty"`
	Source   string    `json:"source,omitempty"`
	Shape    string    `json:"shape,omitempty"`
	Icon     string    `json:"icon,omitempty"`
	Anim     string    `json:"anim,omitempty"`
	Start    float64   `json:"start,omitempty"`
	Duration float64   `json:"duration,omitempty"`
	X        float64   `json:"x,omitempty"`
	Y        float64   `json:"y,omitempty"`
	W        float64   `json:"w,omitempty"`
	H        float64   `json:"h,omitempty"`
	Fill     string    `json:"fill,omitempty"`
	Opacity  float64   `json:"opacity,omitempty"`
	Radius   float64   `json:"radius,omitempty"`
	FontSize int       `json:"font_size,omitempty"`
	Align    string    `json:"align,omitempty"`
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
func (l *Layer) EffectiveBox() (x, y, w, h float64) {
	x, y, w, h = l.X, l.Y, l.W, l.H
	if x == 0 {
		x = 0.1
	}
	if y == 0 {
		y = 0.1
	}
	if w == 0 {
		w = 0.8
	}
	if h == 0 && (l.Kind == LayerShape || l.Kind == LayerCard) {
		h = 0.3
	}
	// Icons default to a square box — their SVG source is square.
	if h == 0 && l.Kind == LayerIcon {
		h = w
	}
	return x, y, w, h
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
