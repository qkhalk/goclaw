package config

// Default agent configuration values.
// These are the single source of truth — all fallback/default logic should reference these
// instead of hardcoding numeric literals.
const (
	DefaultContextWindow   = 200000
	DefaultMaxTokens       = 8192
	DefaultMaxMessageChars = 32000
	DefaultMaxIterations   = 30
	DefaultTemperature     = 0.7
	DefaultHistoryShare    = 0.85
	// DefaultReasoningEffort is the agents.reasoning_default value used when the
	// config key is absent. "auto" lets new agents resolve their reasoning effort
	// from the provider capability map; legacy agents are unaffected (Phase 7).
	DefaultReasoningEffort = "auto"
	// DefaultMaxCompactionsPerSession bounds LLM compactions per session.
	// Zero disables the cap (unlimited legacy behaviour).
	DefaultMaxCompactionsPerSession = 12
)
