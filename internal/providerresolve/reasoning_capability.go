package providerresolve

import (
	"regexp"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Reasoning capability map for the agent-level "auto" reasoning sentinel
// (Phase 7 chat quality). When an agent is explicitly configured with effort
// "auto", the concrete effort is resolved from the provider/model pair via
// ReasoningEffortForModel:
//
//   - Anthropic native providers  -> "medium" (extended thinking is broadly
//     available on Claude 4+; legacy claude-3 models resolve to "off")
//   - OpenAI-compat providers (or any provider) running an o*/gpt-5* reasoning
//     model -> "low" (cheap reasoning tier for delegated/child work)
//   - anything else               -> "off" (safe default: no paid thinking)
//
// The map intentionally keys on coarse provider families + model prefixes —
// the fine-grained per-model registry lives in providers.LookupReasoningCapability
// and is consulted per-request by providers.ResolveReasoningDecision. This map
// only decides the AGENT-LEVEL default for "auto".
//
// RegisterAutoEffortResolver injects this function into the store package
// (store cannot import providerresolve — providerresolve imports store).

// anthropicLikeReasoning lists provider types/names whose native reasoning is
// Anthropic extended thinking.
var anthropicLikeReasoning = map[string]bool{
	store.ProviderAnthropicNative: true,
	"anthropic":                   true, // default provider name / agent.Provider value
	store.ProviderClaudeOAuth:     true,
	store.ProviderClaudeCLI:       true,
	"claude":                      true,
}

// openAICompatReasoning lists provider types/names that speak the OpenAI
// reasoning-effort API. Model-prefix matching still applies for unknown names.
var openAICompatReasoning = map[string]bool{
	store.ProviderOpenAICompat: true,
	"openai":                   true,
	"openai-compat":            true,
	"openai_compatible":        true,
	store.ProviderChatGPTOAuth: true,
	"chatgpt":                  true,
	"codex":                    true,
}

// reasoningModelPrefix matches OpenAI reasoning model families: o1/o3/o4/...
// mini/max variants and gpt-5*.
var reasoningModelPrefix = regexp.MustCompile(`^(o[0-9][a-z0-9.-]*|gpt-5)`)

// ReasoningEffortForModel returns the concrete reasoning effort for the "auto"
// sentinel given a provider type (or provider name) and model. Unknown
// provider/model combinations return "off" — the safe no-cost default.
func ReasoningEffortForModel(providerType, model string) string {
	provider := strings.ToLower(strings.TrimSpace(providerType))
	// Strip any namespace prefix ("openai/gpt-5.2" -> "gpt-5.2") and normalize.
	modelID := strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(modelID, "/"); idx >= 0 {
		modelID = modelID[idx+1:]
	}

	if anthropicLikeReasoning[provider] {
		// Legacy Claude 3 models have no extended thinking — treat as unknown.
		if modelID != "" && strings.HasPrefix(modelID, "claude-3") {
			return "off"
		}
		return "medium"
	}

	if modelID != "" && reasoningModelPrefix.MatchString(modelID) {
		// Reasoning-model prefixes imply reasoning support on any
		// OpenAI-compat endpoint (and on unknown providers too).
		return "low"
	}
	if openAICompatReasoning[provider] {
		// OpenAI-compat provider without a recognized reasoning model: the
		// model cannot be vouched for, so stay off.
		return "off"
	}
	return "off"
}

func init() {
	store.RegisterAutoEffortResolver(ReasoningEffortForModel)
}
