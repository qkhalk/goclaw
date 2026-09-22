package store

import (
	"encoding/json"
	"testing"
)

// withTestAutoResolver installs a capability-map stand-in mirroring the
// providerresolve defaults (anthropic -> medium, reasoning-model prefixes ->
// low, unknown -> off) and restores the previous resolver on cleanup.
func withTestAutoResolver(t *testing.T) {
	t.Helper()
	prev := RegisterAutoEffortResolver(func(providerType, model string) string {
		switch providerType {
		case "anthropic", "anthropic_native":
			return "medium"
		case "openai", "openai_compat":
			if model == "gpt-5.2" || model == "o3-mini" {
				return "low"
			}
			return "off"
		default:
			return "off"
		}
	})
	t.Cleanup(func() { autoEffortResolver = prev })
}

// TestParseReasoningConfig_LegacyNilStaysOff is the Phase 7 back-compat gate:
// agents with nil/empty reasoning resolve to "off" exactly as before — the
// "auto" sentinel only applies to explicitly configured agents.
func TestParseReasoningConfig_LegacyNilStaysOff(t *testing.T) {
	withTestAutoResolver(t)

	cases := []struct {
		name         string
		agent        *AgentData
		wantOverride string
	}{
		{"nil column", &AgentData{Provider: "anthropic", Model: "claude-sonnet-4-5"}, ReasoningOverrideInherit},
		{"nil raw json", &AgentData{Provider: "anthropic", Model: "claude-sonnet-4-5", ReasoningConfig: nil}, ReasoningOverrideInherit},
		// Literal "null" JSONB is a pre-Phase-7 parse quirk: unmarshal of null
		// leaves the struct zeroed, so the code takes the custom branch with
		// effort "off". Behaviour is unchanged — only the mode label differs.
		{"literal null json", &AgentData{Provider: "anthropic", Model: "claude-sonnet-4-5", ReasoningConfig: json.RawMessage(`null`)}, ReasoningOverrideCustom},
		{"empty object json", &AgentData{Provider: "anthropic", Model: "claude-sonnet-4-5", ReasoningConfig: json.RawMessage(`{}`)}, ReasoningOverrideInherit},
	}
	for i, tt := range cases {
		cfg := tt.agent.ParseReasoningConfig()
		if cfg.Effort != "off" {
			t.Errorf("%s (case %d): legacy effort = %q, want off", tt.name, i, cfg.Effort)
		}
		if cfg.OverrideMode != tt.wantOverride {
			t.Errorf("%s (case %d): override_mode = %q, want %q", tt.name, i, cfg.OverrideMode, tt.wantOverride)
		}
	}
}

// TestParseReasoningConfig_AutoResolvesViaCapabilityMap proves explicit "auto"
// (via reasoning_config OR legacy thinking_level) resolves through the
// provider capability map using the agent row's provider/model.
func TestParseReasoningConfig_AutoResolvesViaCapabilityMap(t *testing.T) {
	withTestAutoResolver(t)

	cases := []struct {
		name       string
		agent      *AgentData
		wantEffort string
		wantCustom bool
	}{
		{
			name:       "auto + anthropic -> medium",
			agent:      &AgentData{Provider: "anthropic", Model: "claude-sonnet-4-5", ReasoningConfig: json.RawMessage(`{"effort":"auto"}`)},
			wantEffort: "medium",
			wantCustom: true,
		},
		{
			name:       "auto + unknown provider/model -> off",
			agent:      &AgentData{Provider: "my-gateway", Model: "mystery-model", ReasoningConfig: json.RawMessage(`{"effort":"auto"}`)},
			wantEffort: "off",
			wantCustom: true,
		},
		{
			name:       "auto via legacy thinking_level column + anthropic -> medium",
			agent:      &AgentData{Provider: "anthropic", Model: "claude-sonnet-4-5", ThinkingLevel: "auto"},
			wantEffort: "medium",
			wantCustom: true,
		},
		{
			name:       "explicit low stays low",
			agent:      &AgentData{Provider: "anthropic", Model: "claude-sonnet-4-5", ReasoningConfig: json.RawMessage(`{"effort":"low"}`)},
			wantEffort: "low",
			wantCustom: true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.agent.ParseReasoningConfig()
			if cfg.Effort != tt.wantEffort {
				t.Errorf("effort = %q, want %q", cfg.Effort, tt.wantEffort)
			}
			if tt.wantCustom && cfg.OverrideMode != ReasoningOverrideCustom {
				t.Errorf("override_mode = %q, want custom", cfg.OverrideMode)
			}
		})
	}
}

// TestParseReasoningConfig_AutoPassthroughWithoutResolver proves that when no
// capability map is registered (store-only builds), "auto" passes through
// unchanged for per-request providers.ResolveReasoningDecision downstream.
func TestParseReasoningConfig_AutoPassthroughWithoutResolver(t *testing.T) {
	prev := RegisterAutoEffortResolver(nil)
	t.Cleanup(func() { autoEffortResolver = prev })

	ag := &AgentData{Provider: "anthropic", Model: "claude-sonnet-4-5", ReasoningConfig: json.RawMessage(`{"effort":"auto"}`)}
	if cfg := ag.ParseReasoningConfig(); cfg.Effort != ReasoningEffortAuto {
		t.Errorf("effort = %q, want passthrough %q", cfg.Effort, ReasoningEffortAuto)
	}
}

// TestResolveEffectiveReasoningConfigForModel covers the provider-aware
// variant: explicit provider info resolves the sentinel; empty provider info
// keeps it (safe default, legacy behavior); non-auto efforts pass through.
func TestResolveEffectiveReasoningConfigForModel(t *testing.T) {
	withTestAutoResolver(t)

	customAuto := AgentReasoningConfig{OverrideMode: ReasoningOverrideCustom, Effort: ReasoningEffortAuto}

	if got := ResolveEffectiveReasoningConfigForModel(nil, customAuto, "anthropic", "claude-sonnet-4-5"); got.Effort != "medium" {
		t.Errorf("auto+anthropic effort = %q, want medium", got.Effort)
	}
	if got := ResolveEffectiveReasoningConfigForModel(nil, customAuto, "my-gateway", "mystery"); got.Effort != "off" {
		t.Errorf("auto+unknown effort = %q, want off", got.Effort)
	}
	if got := ResolveEffectiveReasoningConfigForModel(nil, customAuto, "", ""); got.Effort != ReasoningEffortAuto {
		t.Errorf("auto without provider info = %q, want passthrough sentinel (safe default)", got.Effort)
	}

	customLow := AgentReasoningConfig{OverrideMode: ReasoningOverrideCustom, Effort: "low"}
	if got := ResolveEffectiveReasoningConfigForModel(nil, customLow, "anthropic", "claude-sonnet-4-5"); got.Effort != "low" {
		t.Errorf("explicit low effort = %q, want low (untouched)", got.Effort)
	}

	// Legacy inherit agent with provider defaults still resolves provider-owned defaults.
	pd := &ProviderReasoningConfig{Effort: "high"}
	if got := ResolveEffectiveReasoningConfigForModel(pd, AgentReasoningConfig{OverrideMode: ReasoningOverrideInherit}, "anthropic", "claude-sonnet-4-5"); got.Effort != "high" {
		t.Errorf("inherit + provider default effort = %q, want high", got.Effort)
	}
}

// TestResolveEffectiveReasoningConfig_LegacyBehaviorUnchanged re-locks the
// pre-Phase-7 resolution table for the two-arg entry point.
func TestResolveEffectiveReasoningConfig_LegacyBehaviorUnchanged(t *testing.T) {
	// Legacy unset agent, no provider defaults -> off.
	got := ResolveEffectiveReasoningConfig(nil, AgentReasoningConfig{})
	if got.Effort != "off" || got.OverrideMode != ReasoningOverrideInherit {
		t.Errorf("legacy unset = %+v, want inherit/off", got)
	}
	// Explicit custom effort is returned as-is (including "auto").
	got = ResolveEffectiveReasoningConfig(nil, AgentReasoningConfig{OverrideMode: ReasoningOverrideCustom, Effort: ReasoningEffortAuto})
	if got.Effort != ReasoningEffortAuto {
		t.Errorf("custom auto passthrough = %q, want auto", got.Effort)
	}
}
