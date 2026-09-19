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
	// maxHighlights caps colored-keyword entries on one text layer.
	maxHighlights = 6
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
		// An explicit color/color2/glow must be #RRGGBB; with a style_pack an
		// unset color is fine — the pack supplies the backdrop.
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

// validate checks one layer against its host scene's duration — mirror of
// internal/video Layer.validate.
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
