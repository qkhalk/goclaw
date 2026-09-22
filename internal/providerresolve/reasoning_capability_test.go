package providerresolve

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestReasoningEffortForModel locks the agent-level "auto" capability map
// (Phase 7): anthropic native -> medium, OpenAI-style reasoning-model
// prefixes -> low, anything else -> off (safe no-cost default).
func TestReasoningEffortForModel(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		want     string
	}{
		{"anthropic native -> medium", "anthropic_native", "claude-sonnet-4-5", "medium"},
		{"anthropic default provider name -> medium", "anthropic", "claude-sonnet-4-5-20250929", "medium"},
		{"anthropic case-insensitive -> medium", "Anthropic", "claude-opus-4-8", "medium"},
		{"claude oauth -> medium", "claude_oauth", "claude-sonnet-4-5", "medium"},
		{"anthropic legacy claude-3 -> off", "anthropic", "claude-3-5-haiku-20241022", "off"},
		{"openai-compat o-prefix reasoning model -> low", "openai_compat", "o3-mini", "low"},
		{"openai-compat gpt-5 prefix -> low", "openai_compat", "gpt-5.2", "low"},
		{"openai-compat namespaced reasoning model -> low", "openai_compat", "openai/o4-mini", "low"},
		{"openai-compat non-reasoning model -> off", "openai_compat", "gpt-4.1", "off"},
		{"unknown provider with reasoning-model prefix -> low", "my-gateway", "o3-mini", "low"},
		{"unknown provider/model -> off", "my-gateway", "mystery-model", "off"},
		{"empty provider/model -> off", "", "", "off"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReasoningEffortForModel(tt.provider, tt.model); got != tt.want {
				t.Errorf("ReasoningEffortForModel(%q, %q) = %q, want %q", tt.provider, tt.model, got, tt.want)
			}
		})
	}
}

// TestInitRegistersAutoEffortResolver proves the package init wires the
// capability map into the store (the store cannot import providerresolve, so
// registration is the only link). Grabs the registered resolver, exercises it,
// and restores it.
func TestInitRegistersAutoEffortResolver(t *testing.T) {
	registered := store.RegisterAutoEffortResolver(nil)
	t.Cleanup(func() { store.RegisterAutoEffortResolver(registered) })

	if registered == nil {
		t.Fatal("providerresolve init must register the auto-effort resolver into store")
	}
	if got := registered("anthropic", "claude-sonnet-4-5"); got != "medium" {
		t.Errorf("registered resolver anthropic = %q, want medium", got)
	}
	if got := registered("openai_compat", "o3-mini"); got != "low" {
		t.Errorf("registered resolver openai_compat/o3-mini = %q, want low", got)
	}
	if got := registered("mystery", "mystery-model"); got != "off" {
		t.Errorf("registered resolver unknown = %q, want off", got)
	}
}
