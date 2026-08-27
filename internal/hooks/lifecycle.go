package hooks

// ─── Extended lifecycle events ──────────────────────────────────────────────
//
// These extend the core events in types.go to cover agent-run, LLM, checkpoint,
// completion, error, and rate-limit lifecycle points defined in Phase 15.

const (
	// EventBeforeRun fires before an agent run starts. BLOCKING.
	EventBeforeRun HookEvent = "before_run"
	// EventAfterRun fires after an agent run completes (success or failure).
	EventAfterRun HookEvent = "after_run"
	// EventBeforeLLM fires before an LLM API call. BLOCKING.
	EventBeforeLLM HookEvent = "before_llm"
	// EventAfterLLM fires after an LLM API call completes.
	EventAfterLLM HookEvent = "after_llm"
	// EventBeforeCheckpoint fires before a checkpoint is persisted. BLOCKING.
	EventBeforeCheckpoint HookEvent = "before_checkpoint"
	// EventAfterCheckpoint fires after a checkpoint is persisted.
	EventAfterCheckpoint HookEvent = "after_checkpoint"
	// EventBeforeComplete fires before the final response is delivered. BLOCKING.
	EventBeforeComplete HookEvent = "before_complete"
	// EventAfterComplete fires after the final response is delivered.
	EventAfterComplete HookEvent = "after_complete"
	// EventOnError fires when a run-level error occurs. Non-blocking.
	EventOnError HookEvent = "on_error"
	// EventOnRateLimit fires when a rate-limit is hit. Non-blocking.
	EventOnRateLimit HookEvent = "on_rate_limit"
)

// IsBlocking returns true for extended events that require a synchronous
// allow/block decision before the pipeline continues.
func isBlockingExtended(e HookEvent) bool {
	switch e {
	case EventBeforeRun, EventBeforeLLM, EventBeforeCheckpoint, EventBeforeComplete:
		return true
	default:
		return false
	}
}

// ─── Per-hook failure policy ────────────────────────────────────────────────

// FailurePolicy controls what happens when a hook handler fails.
type FailurePolicy string

const (
	// FailurePolicyContinue logs the error and continues pipeline execution.
	FailurePolicyContinue FailurePolicy = "continue"
	// FailurePolicyBlock blocks the pipeline (fail-closed).
	FailurePolicyBlock FailurePolicy = "block"
	// FailurePolicyAbort cancels the entire run.
	FailurePolicyAbort FailurePolicy = "abort"
)

// ─── Per-hook permissions ───────────────────────────────────────────────────

// HookPermissions restricts which tools/capabilities a hook handler may use.
type HookPermissions struct {
	// AllowedTools limits the handler to these tools (empty = all allowed).
	AllowedTools []string `json:"allowed_tools,omitempty"`
	// DeniedTools blocks these tools regardless of AllowedTools.
	DeniedTools []string `json:"denied_tools,omitempty"`
	// MaxDurationMS caps handler execution time (overrides TimeoutMS if smaller).
	MaxDurationMS int `json:"max_duration_ms,omitempty"`
	// RequireApproval gates the hook on human approval before execution.
	RequireApproval bool `json:"require_approval,omitempty"`
}

// ─── Extended HookConfig fields ─────────────────────────────────────────────

// HookConfigExtended holds Phase 15 extensions to HookConfig.
// These fields are optional and default to zero values for backward compat.
type HookConfigExtended struct {
	// FailurePolicy controls pipeline behavior on handler failure.
	// Zero value = "continue" (safe default).
	FailurePolicy FailurePolicy `json:"failure_policy,omitempty"`
	// Permissions restricts handler capabilities.
	Permissions *HookPermissions `json:"permissions,omitempty"`
}

// GetFailurePolicy returns the failure policy, defaulting to "continue".
func GetFailurePolicy(cfg HookConfig) FailurePolicy {
	if ext, ok := cfg.Metadata["failure_policy"].(string); ok && ext != "" {
		return FailurePolicy(ext)
	}
	return FailurePolicyContinue
}

// GetPermissions returns the per-hook permissions, or nil (no restrictions).
func GetPermissions(cfg HookConfig) *HookPermissions {
	if raw, ok := cfg.Metadata["permissions"]; ok {
		if m, ok := raw.(map[string]any); ok {
			p := &HookPermissions{}
			if tools, ok := m["allowed_tools"].([]any); ok {
				for _, t := range tools {
					if s, ok := t.(string); ok {
						p.AllowedTools = append(p.AllowedTools, s)
					}
				}
			}
			if tools, ok := m["denied_tools"].([]any); ok {
				for _, t := range tools {
					if s, ok := t.(string); ok {
						p.DeniedTools = append(p.DeniedTools, s)
					}
				}
			}
			if v, ok := m["max_duration_ms"].(float64); ok {
				p.MaxDurationMS = int(v)
			}
			if v, ok := m["require_approval"].(bool); ok {
				p.RequireApproval = v
			}
			return p
		}
	}
	return nil
}

// IsToolAllowedByHook checks if a tool is permitted by hook permissions.
func IsToolAllowedByHook(p *HookPermissions, tool string) bool {
	if p == nil {
		return true
	}
	for _, d := range p.DeniedTools {
		if d == tool {
			return false
		}
	}
	if len(p.AllowedTools) == 0 {
		return true
	}
	for _, a := range p.AllowedTools {
		if a == tool {
			return true
		}
	}
	return false
}
