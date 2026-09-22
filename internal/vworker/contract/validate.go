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
// hostile or buggy storyboard cannot drive the filtergraph into giant pad
// allocations or Inf parameters. Zero counts as unset (omitempty) and always
// passes.
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
