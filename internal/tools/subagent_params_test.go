package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// paramsCaptureProvider captures the full ChatRequest (including Options) of
// every call so tests can assert the derived LLM parameters.
type paramsCaptureProvider struct {
	requests []providers.ChatRequest
}

func (p *paramsCaptureProvider) Name() string         { return "capture" }
func (p *paramsCaptureProvider) DefaultModel() string { return "provider-default" }
func (p *paramsCaptureProvider) Chat(_ context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	p.requests = append(p.requests, req)
	return &providers.ChatResponse{Content: "done", FinishReason: "stop"}, nil
}
func (p *paramsCaptureProvider) ChatStream(_ context.Context, req providers.ChatRequest, _ func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	p.requests = append(p.requests, req)
	return &providers.ChatResponse{Content: "done", FinishReason: "stop"}, nil
}

func (p *paramsCaptureProvider) firstOptions(t *testing.T) map[string]any {
	t.Helper()
	if len(p.requests) == 0 {
		t.Fatal("provider received no ChatRequest")
	}
	return p.requests[0].Options
}

func subagentParamsTestContext(parentMaxTokens int) context.Context {
	ctx := context.Background()
	ctx = store.WithTenantID(ctx, uuid.New())
	ctx = store.WithAgentID(ctx, uuid.New())
	ctx = WithToolAgentKey(ctx, "parent")
	if parentMaxTokens > 0 {
		ctx = store.WithAgentMaxTokens(ctx, parentMaxTokens)
	}
	return ctx
}

// TestSubagentLLMOptions_InheritsParentMaxTokens proves the subagent request
// carries the PARENT agent's effective max_tokens instead of the old hardcoded
// 4096, and the parent loop's effective temperature (config.DefaultTemperature)
// instead of the old 0.5 literal.
func TestSubagentLLMOptions_InheritsParentMaxTokens(t *testing.T) {
	prov := &paramsCaptureProvider{}
	manager := NewSubagentManager(prov, nil, "manager-default", nil, NewRegistry, SubagentConfig{
		MaxConcurrent: 4, MaxSpawnDepth: 3, MaxChildrenPerAgent: 8,
	})
	manager.SetAgentBudget(200_000, 32_000)

	ctx := subagentParamsTestContext(12_000)
	if _, _, _, err := manager.RunSync(ctx, "parent", 0, "task", "label", "", "chan", "chat"); err != nil {
		t.Fatalf("RunSync error: %v", err)
	}

	opts := prov.firstOptions(t)
	if got := opts[providers.OptMaxTokens]; got != 12_000 {
		t.Errorf("max_tokens = %v, want 12000 (parent effective)", got)
	}
	if got := opts[providers.OptTemperature]; got != config.DefaultTemperature {
		t.Errorf("temperature = %v, want %v (parent loop effective)", got, config.DefaultTemperature)
	}
	if _, ok := opts[providers.OptThinkingLevel]; ok {
		t.Errorf("thinking_level must not be set without a definition override, got %v", opts[providers.OptThinkingLevel])
	}
}

// TestSubagentLLMOptions_DefaultsWhenParentAbsent proves the fallback when the
// spawning context carries no parent budget: config defaults (8192 / 0.7),
// never the old 4096 / 0.5 literals.
func TestSubagentLLMOptions_DefaultsWhenParentAbsent(t *testing.T) {
	prov := &paramsCaptureProvider{}
	manager := NewSubagentManager(prov, nil, "manager-default", nil, NewRegistry, SubagentConfig{
		MaxConcurrent: 4, MaxSpawnDepth: 3, MaxChildrenPerAgent: 8,
	})
	// Manager-level budget so the usage-cap guard is wired; the important part
	// is that NO parent max-tokens is present in the spawn context.
	manager.SetAgentBudget(200_000, 32_000)

	ctx := subagentParamsTestContext(0) // no parent max tokens in ctx
	if _, _, _, err := manager.RunSync(ctx, "parent", 0, "task", "label", "", "chan", "chat"); err != nil {
		t.Fatalf("RunSync error: %v", err)
	}

	opts := prov.firstOptions(t)
	if got := opts[providers.OptMaxTokens]; got != config.DefaultMaxTokens {
		t.Errorf("max_tokens = %v, want %d (config default)", got, config.DefaultMaxTokens)
	}
	if got := opts[providers.OptTemperature]; got != config.DefaultTemperature {
		t.Errorf("temperature = %v, want %v (config default)", got, config.DefaultTemperature)
	}
}

// TestSubagentLLMOptions_DefinitionOverridesWin proves the precedence chain
// definition > parent effective > fallbacks: a definition carrying explicit
// maxTokens/temperature/thinkingLevel beats the parent's inherited values.
func TestSubagentLLMOptions_DefinitionOverridesWin(t *testing.T) {
	prov := &paramsCaptureProvider{}
	manager := NewSubagentManager(prov, nil, "manager-default", nil, NewRegistry, SubagentConfig{
		MaxConcurrent: 4, MaxSpawnDepth: 3, MaxChildrenPerAgent: 8,
	})
	manager.SetAgentBudget(200_000, 32_000)

	defTemp := 0.2
	defTokens := 3_000
	ctx := WithSubagentDefinition(subagentParamsTestContext(12_000), &config.SubagentDefinition{
		Name:          "researcher",
		SystemPrompt:  "You research things.",
		MaxTokens:     &defTokens,
		Temperature:   &defTemp,
		ThinkingLevel: "high",
	})
	if _, _, _, err := manager.RunSync(ctx, "parent", 0, "task", "label", "", "chan", "chat"); err != nil {
		t.Fatalf("RunSync error: %v", err)
	}

	opts := prov.firstOptions(t)
	if got := opts[providers.OptMaxTokens]; got != 3_000 {
		t.Errorf("max_tokens = %v, want 3000 (definition override)", got)
	}
	if got := opts[providers.OptTemperature]; got != 0.2 {
		t.Errorf("temperature = %v, want 0.2 (definition override)", got)
	}
	if got := opts[providers.OptThinkingLevel]; got != "high" {
		t.Errorf("thinking_level = %v, want high (definition override)", got)
	}
}

// TestSubagentLLMOptions_DefinitionPartialOverride proves per-field precedence:
// a definition overriding only temperature still inherits the parent's
// max_tokens, and vice versa.
func TestSubagentLLMOptions_DefinitionPartialOverride(t *testing.T) {
	prov := &paramsCaptureProvider{}
	manager := NewSubagentManager(prov, nil, "manager-default", nil, NewRegistry, SubagentConfig{
		MaxConcurrent: 4, MaxSpawnDepth: 3, MaxChildrenPerAgent: 8,
	})
	manager.SetAgentBudget(200_000, 32_000)

	defTemp := 0.9
	ctx := WithSubagentDefinition(subagentParamsTestContext(12_000), &config.SubagentDefinition{
		Name:         "creative",
		SystemPrompt: "You write things.",
		Temperature:  &defTemp,
	})
	if _, _, _, err := manager.RunSync(ctx, "parent", 0, "task", "label", "", "chan", "chat"); err != nil {
		t.Fatalf("RunSync error: %v", err)
	}

	opts := prov.firstOptions(t)
	if got := opts[providers.OptMaxTokens]; got != 12_000 {
		t.Errorf("max_tokens = %v, want 12000 (parent effective, no definition override)", got)
	}
	if got := opts[providers.OptTemperature]; got != 0.9 {
		t.Errorf("temperature = %v, want 0.9 (definition override)", got)
	}
}

// TestSubagentLLMOptions_InvalidDefinitionValuesIgnored proves garbage or
// non-positive definition values fall through to the parent/config chain
// instead of producing an invalid request.
func TestSubagentLLMOptions_InvalidDefinitionValuesIgnored(t *testing.T) {
	zero := 0
	task := &SubagentTask{
		ID:              "t-invalid",
		OriginMaxTokens: 10_000,
		definition: &config.SubagentDefinition{
			Name:          "broken",
			SystemPrompt:  "prompt",
			MaxTokens:     &zero,
			Temperature:   nil,
			ThinkingLevel: "banana",
		},
	}

	opts := subagentLLMOptions(task)
	if got := opts[providers.OptMaxTokens]; got != 10_000 {
		t.Errorf("max_tokens = %v, want 10000 (zero override ignored, parent value used)", got)
	}
	if got := opts[providers.OptTemperature]; got != config.DefaultTemperature {
		t.Errorf("temperature = %v, want %v (default)", got, config.DefaultTemperature)
	}
	if _, ok := opts[providers.OptThinkingLevel]; ok {
		t.Errorf("invalid thinkingLevel must be dropped, got %v", opts[providers.OptThinkingLevel])
	}
}
