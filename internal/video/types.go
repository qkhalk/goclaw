// Package video defines the Storyboard v1 contract shared by the gateway and
// the videoworker sidecar, plus the worker job API shapes. Types only — no
// rendering. The worker intentionally does NOT import this package (it keeps
// a copy-shaped parser in internal/vworker/contract so the sidecar binary
// stays independent); a golden-file test guards the two parsers against
// drift.
package video

import (
	"fmt"
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

// Scene enter transitions — exact values of the web editor's SceneTransition
// union ("none"|"fade"|"crossfade"|"slide_left"|"slide_up"). A transition
// decorates the START of a scene: for TRANSITION_SEC seconds the incoming
// scene blends in over the tail of the previous one. Honored by the browser
// player, client export and the ffmpeg worker (chained xfade).
const (
	TransitionNone      = "none"
	TransitionFade      = "fade"
	TransitionCrossfade = "crossfade"
	TransitionSlideLeft = "slide_left"
	TransitionSlideUp   = "slide_up"
)

// Scene is one clip in the timeline.
type Scene struct {
	Type        SceneKind  `json:"type"`
	Source      string     `json:"source,omitempty"` // workspace-relative path or http(s) URL; empty for color
	Color       string     `json:"color,omitempty"`  // "#RRGGBB" for color scenes
	DurationSec float64    `json:"duration_sec"`
	Fit         string     `json:"fit,omitempty"` // cover|contain (default cover)
	Mute        bool       `json:"mute,omitempty"`
	KenBurns    *KenBurns  `json:"ken_burns,omitempty"`
	Caption     *Caption   `json:"caption,omitempty"`
	Narration   *Narration `json:"narration,omitempty"`
	Transform   *Transform `json:"transform,omitempty"`  // per-scene geometry (image/video scenes)
	Filter      *Filter    `json:"filter,omitempty"`     // per-scene color grade (image/video scenes)
	Transition  string     `json:"transition,omitempty"` // enter transition: none|fade|crossfade|slide_left|slide_up
}

// Transform is an OpenCut-style per-scene geometric transform. The web editor
// pivots at the frame center: scale is a multiple around that center, x/y are
// offsets in % of frame size, rotate is degrees, opacity is 0-1 (0/unset =
// fully opaque).
type Transform struct {
	Scale   float64 `json:"scale,omitempty"`   // multiple around frame center; 1 = identity
	X       float64 `json:"x,omitempty"`       // offset, % of frame width
	Y       float64 `json:"y,omitempty"`       // offset, % of frame height
	Rotate  float64 `json:"rotate,omitempty"`  // degrees, clockwise
	Opacity float64 `json:"opacity,omitempty"` // 0-1 flattening over black; unset = opaque
}

// Filter is a CSS-style color grade applied to the scene's media.
type Filter struct {
	Brightness float64 `json:"brightness,omitempty"` // CSS brightness(k); 1 = normal
	Contrast   float64 `json:"contrast,omitempty"`   // CSS contrast(k); 1 = normal
	Saturate   float64 `json:"saturate,omitempty"`   // CSS saturate(k); 1 = normal
	Blur       float64 `json:"blur,omitempty"`       // Gaussian radius in px; 0 = none
}

// KenBurns animates a slow zoom/pan on image scenes.
type KenBurns struct {
	ZoomFrom float64 `json:"zoom_from,omitempty"` // default 1.0
	ZoomTo   float64 `json:"zoom_to,omitempty"`   // default 1.0 (no zoom)
	Pan      string  `json:"pan,omitempty"`       // none|left|right|up|down
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
	switch sc.Transition {
	case "", TransitionNone, TransitionFade, TransitionCrossfade, TransitionSlideLeft, TransitionSlideUp:
	default:
		return fmt.Errorf("unknown transition type %q (want none, fade, crossfade, slide_left or slide_up)", sc.Transition)
	}
	if err := sc.validateFx(); err != nil {
		return err
	}
	return nil
}

// validateFx bounds the optional per-scene transform and color grade so a
// hostile or buggy storyboard cannot drive the worker filtergraph into giant
// pad allocations or Inf parameters. Zero counts as unset (omitempty) and
// always passes. Mirror of internal/vworker/contract validateFx.
func (sc *Scene) validateFx() error {
	if tr := sc.Transform; tr != nil {
		if s := tr.Scale; s != 0 && (s < 0.01 || s > 16) {
			return fmt.Errorf("transform scale %.3f out of range 0.01..16", s)
		}
		if tr.X < -100 || tr.X > 100 || tr.Y < -100 || tr.Y > 100 {
			return fmt.Errorf("transform x/y %.1f/%.1f out of range -100..100 (%% of frame)", tr.X, tr.Y)
		}
		if tr.Rotate < -360 || tr.Rotate > 360 {
			return fmt.Errorf("transform rotate %.1f out of range -360..360 degrees", tr.Rotate)
		}
		if tr.Opacity < 0 || tr.Opacity > 1 {
			return fmt.Errorf("transform opacity %.3f out of range 0..1", tr.Opacity)
		}
	}
	if f := sc.Filter; f != nil {
		switch {
		case f.Brightness < 0 || f.Brightness > 2:
			return fmt.Errorf("filter brightness %.2f out of range 0..2", f.Brightness)
		case f.Contrast < 0 || f.Contrast > 3:
			return fmt.Errorf("filter contrast %.2f out of range 0..3", f.Contrast)
		case f.Saturate < 0 || f.Saturate > 3:
			return fmt.Errorf("filter saturate %.2f out of range 0..3", f.Saturate)
		case f.Blur < 0 || f.Blur > 100:
			return fmt.Errorf("filter blur %.1f out of range 0..100 px", f.Blur)
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
