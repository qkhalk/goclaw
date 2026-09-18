package contract

import "testing"

func TestValidateFxRanges(t *testing.T) {
	base := Scene{Type: SceneColor, Color: "#000000", DurationSec: 5}

	tests := []struct {
		name    string
		mutate  func(s *Scene)
		wantErr bool
	}{
		{"identity_unset", func(s *Scene) { s.Transform = &Transform{}; s.Filter = &Filter{} }, false},
		{"nil_fx", func(s *Scene) {}, false},
		{"scale_ok", func(s *Scene) { s.Transform = &Transform{Scale: 1.2} }, false},
		{"scale_min", func(s *Scene) { s.Transform = &Transform{Scale: 0.01} }, false},
		{"scale_too_small", func(s *Scene) { s.Transform = &Transform{Scale: 0.001} }, true},
		{"scale_huge", func(s *Scene) { s.Transform = &Transform{Scale: 1e6} }, true},
		{"offset_ok", func(s *Scene) { s.Transform = &Transform{X: 100, Y: -100} }, false},
		{"offset_huge", func(s *Scene) { s.Transform = &Transform{X: 100000} }, true},
		{"rotate_ok", func(s *Scene) { s.Transform = &Transform{Rotate: 359} }, false},
		{"rotate_over", func(s *Scene) { s.Transform = &Transform{Rotate: 361} }, true},
		{"opacity_range", func(s *Scene) { s.Transform = &Transform{Opacity: 0.5} }, false},
		{"opacity_over", func(s *Scene) { s.Transform = &Transform{Opacity: 1.5} }, true},
		{"brightness_ok", func(s *Scene) { s.Filter = &Filter{Brightness: 1.5} }, false},
		{"brightness_over", func(s *Scene) { s.Filter = &Filter{Brightness: 3} }, true},
		{"contrast_over", func(s *Scene) { s.Filter = &Filter{Contrast: 5} }, true},
		{"saturate_negative", func(s *Scene) { s.Filter = &Filter{Saturate: -1} }, true},
		{"blur_over", func(s *Scene) { s.Filter = &Filter{Blur: 500} }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := base
			tt.mutate(&sc)
			err := sc.validateFx()
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFx() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
