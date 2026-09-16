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
	DurationSec float64    `json:"duration_sec"`
	Fit         string     `json:"fit,omitempty"` // cover|contain (default cover)
	Transition  string     `json:"transition,omitempty"` // enter transition: none|fade|crossfade|slide_left|slide_up
	Mute        bool       `json:"mute,omitempty"`
	KenBurns    *KenBurns  `json:"ken_burns,omitempty"`
	Caption     *Caption   `json:"caption,omitempty"`
	Narration   *Narration `json:"narration,omitempty"`
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

// Caption draws text over the scene via drawtext (needs a font file on the
// worker; without one captions are skipped with a warning).
type Caption struct {
	Text     string `json:"text"`
	Position string `json:"position,omitempty"` // top|center|bottom (default bottom)
	FontSize int    `json:"font_size,omitempty"`
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
		if !hexColor(sc.Color) {
			return fmt.Errorf("color scenes need a #RRGGBB color, got %q", sc.Color)
		}
	default:
		return fmt.Errorf("unknown scene type %q", sc.Type)
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
	return nil
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
