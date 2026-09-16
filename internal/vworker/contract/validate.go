package contract

import (
	"fmt"
	"strings"
)

const (
	maxScenes      = 60
	maxSceneSec    = 30.0
	maxTotalSec    = 600.0
	maxCanvasEdge  = 1920
	defaultFPS     = 30
	defaultHeight  = 720
	allowedHeights = "480, 720 or 1080"
	maxLayers      = 8
)

// Validate checks the storyboard against v1 constraints.
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

// validate checks one layer against its host scene's duration — mirror of
// internal/video Layer.validate.
func (l *Layer) validate(sceneSec float64) error {
	switch l.Kind {
	case LayerText:
		if strings.TrimSpace(l.Text) == "" {
			return fmt.Errorf("text layers need text")
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
	default:
		return fmt.Errorf("unknown layer kind %q (text, shape, image)", l.Kind)
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
