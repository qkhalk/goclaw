package methods

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// stampCreateAgent runs handleCreate with the given params against the capture
// stub and returns the agent row that would have been persisted.
func stampCreateAgent(t *testing.T, m *AgentsMethods, params map[string]any) *store.AgentData {
	t.Helper()
	stub := &createCaptureStore{}
	m.agentStore = stub
	m.handleCreate(context.Background(), nullClient(), buildCreateRequest(t, params))
	if stub.created == nil {
		t.Fatal("agentStore.Create was not called")
	}
	return stub.created
}

// TestHandleCreate_StampsReasoningDefaultAuto is the Phase 7 gate: a new agent
// whose request omits both reasoning_config and thinking_level inherits
// agents.reasoning_default — empty config resolves to "auto".
func TestHandleCreate_StampsReasoningDefaultAuto(t *testing.T) {
	m := newManagedMethods(t, nil) // cfg.ReasoningDefault unset -> "auto"

	created := stampCreateAgent(t, m, map[string]any{"name": "Auto Agent"})
	if created.ThinkingLevel != "auto" {
		t.Errorf("ThinkingLevel = %q, want auto (agents.reasoning_default default)", created.ThinkingLevel)
	}
	// reasoning_config must stay unstamped so ParseReasoningConfig reads the
	// legacy thinking_level column as the single source of truth.
	if len(created.ReasoningConfig) != 0 {
		t.Errorf("ReasoningConfig = %q, want empty (no double-stamp)", created.ReasoningConfig)
	}
}

// TestHandleCreate_ReasoningDefaultConfigValue proves the configured value is
// stamped verbatim for new agents.
func TestHandleCreate_ReasoningDefaultConfigValue(t *testing.T) {
	m := newManagedMethods(t, nil)
	m.cfg.Agents.ReasoningDefault = "medium"

	created := stampCreateAgent(t, m, map[string]any{"name": "Medium Agent"})
	if created.ThinkingLevel != "medium" {
		t.Errorf("ThinkingLevel = %q, want medium (configured reasoning_default)", created.ThinkingLevel)
	}
}

// TestHandleCreate_ReasoningDefaultInheritStampsNothing proves the escape
// hatch: reasoning_default "inherit" (or a config typo, which normalizes to
// inherit) preserves the pre-Phase-7 creation behaviour — no stamp.
func TestHandleCreate_ReasoningDefaultInheritStampsNothing(t *testing.T) {
	for _, value := range []string{"inherit", "banana"} {
		m := newManagedMethods(t, nil)
		m.cfg.Agents.ReasoningDefault = value

		created := stampCreateAgent(t, m, map[string]any{"name": "Legacy Agent " + value})
		if created.ThinkingLevel != "" {
			t.Errorf("reasoning_default %q: ThinkingLevel = %q, want empty (no stamp)", value, created.ThinkingLevel)
		}
	}
}

// TestHandleCreate_ExplicitThinkingLevelWins proves an explicit request value
// always beats agents.reasoning_default.
func TestHandleCreate_ExplicitThinkingLevelWins(t *testing.T) {
	m := newManagedMethods(t, nil) // default would stamp "auto"

	created := stampCreateAgent(t, m, map[string]any{"name": "Explicit Agent", "thinking_level": "high"})
	if created.ThinkingLevel != "high" {
		t.Errorf("ThinkingLevel = %q, want high (explicit request wins)", created.ThinkingLevel)
	}
}

// TestHandleCreate_ExplicitReasoningConfigNotDoubleStamped proves an advanced
// reasoning_config in the request suppresses the thinking_level stamp — the
// JSONB stays the single source of truth for the agent.
func TestHandleCreate_ExplicitReasoningConfigNotDoubleStamped(t *testing.T) {
	m := newManagedMethods(t, nil) // default would stamp "auto"

	rawConfig := json.RawMessage(`{"override_mode":"custom","effort":"low"}`)
	created := stampCreateAgent(t, m, map[string]any{
		"name":             "Advanced Agent",
		"reasoning_config": rawConfig,
	})
	if created.ThinkingLevel != "" {
		t.Errorf("ThinkingLevel = %q, want empty (reasoning_config wins, no double-stamp)", created.ThinkingLevel)
	}
	if string(created.ReasoningConfig) != string(rawConfig) {
		t.Errorf("ReasoningConfig = %s, want %s preserved", created.ReasoningConfig, rawConfig)
	}
}

// TestDefaultThinkingLevelForNewAgent covers the helper edges directly:
// whitespace/"null" JSONB payloads still stamp; real configs never do.
func TestDefaultThinkingLevelForNewAgent(t *testing.T) {
	m := newManagedMethods(t, nil)

	if got := defaultThinkingLevelForNewAgent(m.cfg, nil); got != "auto" {
		t.Errorf("nil reasoning_config = %q, want auto", got)
	}
	if got := defaultThinkingLevelForNewAgent(m.cfg, json.RawMessage("null")); got != "auto" {
		t.Errorf("null reasoning_config = %q, want auto", got)
	}
	if got := defaultThinkingLevelForNewAgent(m.cfg, json.RawMessage(`{}`)); got != "auto" {
		t.Errorf("empty-object reasoning_config = %q, want auto", got)
	}
	if got := defaultThinkingLevelForNewAgent(m.cfg, json.RawMessage(`{"effort":"low"}`)); got != "" {
		t.Errorf("real reasoning_config = %q, want empty (no double-stamp)", got)
	}

	m.cfg.Agents.ReasoningDefault = "inherit"
	if got := defaultThinkingLevelForNewAgent(m.cfg, nil); got != "" {
		t.Errorf("inherit default = %q, want empty", got)
	}
}
