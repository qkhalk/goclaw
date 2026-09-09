package config

import (
	"encoding/json"
	"slices"
	"testing"
)

// TestDeferredToolsConfig_DefaultsOff verifies the dark-shipping contract:
// zero-value config means disabled, and Resolve fills safe defaults.
func TestDeferredToolsConfig_DefaultsOff(t *testing.T) {
	var cfg ToolsConfig
	if err := json.Unmarshal([]byte(`{}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Deferred.Enabled {
		t.Error("tools.deferred must default to disabled (ships dark)")
	}

	resolved := cfg.Deferred.Resolve()
	if resolved.Enabled {
		t.Error("Resolve must not enable the feature")
	}
	if resolved.Threshold != DefaultDeferredToolsThreshold {
		t.Errorf("threshold default = %d, want %d", resolved.Threshold, DefaultDeferredToolsThreshold)
	}
	if len(resolved.AlwaysInline) == 0 {
		t.Fatal("Resolve must fill the default always_inline list")
	}
	for _, want := range []string{"group:fs", "group:web", "group:sessions", "group:memory"} {
		if !slices.Contains(resolved.AlwaysInline, want) {
			t.Errorf("default always_inline missing %q: %v", want, resolved.AlwaysInline)
		}
	}
}

// TestDeferredToolsConfig_ExplicitValuesWin verifies overrides are preserved.
func TestDeferredToolsConfig_ExplicitValuesWin(t *testing.T) {
	raw := `{
		"tools": {
			"deferred": {
				"enabled": true,
				"threshold": 25,
				"always_inline": ["group:fs", "web_search"]
			}
		}
	}`
	var cfg struct {
		Tools ToolsConfig `json:"tools"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	resolved := cfg.Tools.Deferred.Resolve()
	if !resolved.Enabled {
		t.Error("enabled=true must be preserved")
	}
	if resolved.Threshold != 25 {
		t.Errorf("threshold = %d, want 25", resolved.Threshold)
	}
	if !slices.Equal(resolved.AlwaysInline, []string{"group:fs", "web_search"}) {
		t.Errorf("always_inline = %v, want explicit list", resolved.AlwaysInline)
	}
}

// TestDeferredToolsConfig_NegativeThresholdFallsBack: a nonsensical threshold
// resolves to the default instead of disabling or panicking.
func TestDeferredToolsConfig_NegativeThresholdFallsBack(t *testing.T) {
	cfg := DeferredToolsConfig{Enabled: true, Threshold: -5}
	resolved := cfg.Resolve()
	if resolved.Threshold != DefaultDeferredToolsThreshold {
		t.Errorf("negative threshold must fall back to default, got %d", resolved.Threshold)
	}
}
