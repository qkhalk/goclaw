package skills

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// SkillExecutor orchestrates the execution of a resolved skill.
// It enforces tool permissions, tracks quality gates, and manages artifacts.
// The actual LLM interaction is delegated to the caller (agent loop) —
// the executor provides the structured framework around it.
type SkillExecutor struct {
	resolver *SkillResolver

	mu sync.RWMutex
	// Active runs tracked by runID
	active map[string]*SkillRun
}

// SkillRun tracks the state of an in-progress skill execution.
type SkillRun struct {
	Spec      *SkillSpec    `json:"spec"`
	Context   *SkillContext `json:"context"`
	Result    *SkillResult  `json:"result"`
	StartedAt time.Time     `json:"startedAt"`
	StepIndex int           `json:"stepIndex"` // current step in multi-step skill
}

// NewSkillExecutor creates an executor backed by the given resolver.
func NewSkillExecutor(resolver *SkillResolver) *SkillExecutor {
	return &SkillExecutor{
		resolver: resolver,
		active:   make(map[string]*SkillRun),
	}
}

// Resolver returns the underlying skill resolver.
func (e *SkillExecutor) Resolver() *SkillResolver {
	return e.resolver
}

// BeginRun initializes a new skill execution run. It resolves the skill,
// validates the spec, and sets up the execution context.
func (e *SkillExecutor) BeginRun(ctx context.Context, slug string, sctx *SkillContext) (*SkillRun, error) {
	// Resolve skill and its dependencies
	specs, err := e.resolver.ResolveDependencies(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("resolve dependencies: %w", err)
	}

	// The last spec in the chain is the skill itself
	spec := specs[len(specs)-1]

	// Validate
	if err := ValidateSpec(spec); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	// Build effective tool policy (intersect skill AllowedTools with context policy)
	effectivePolicy := e.buildToolPolicy(spec, &sctx.ToolPolicy)

	run := &SkillRun{
		Spec:      spec,
		Context:   sctx,
		StartedAt: time.Now(),
		Result: &SkillResult{
			Status: SkillStatusPending,
		},
	}
	run.Context.ToolPolicy = effectivePolicy

	// Track active run
	e.mu.Lock()
	e.active[sctx.RunID] = run
	e.mu.Unlock()

	slog.Info("skill.run.started",
		"slug", slug,
		"run_id", sctx.RunID,
		"max_retries", spec.MaxRetries,
		"quality_gates", len(spec.QualityGates),
	)

	return run, nil
}

// CheckToolPermission checks if a tool call is allowed by the current
// skill's policy. Returns (allowed, needsApproval).
func (e *SkillExecutor) CheckToolPermission(runID string, tool string) (bool, bool) {
	e.mu.RLock()
	run, ok := e.active[runID]
	e.mu.RUnlock()
	if !ok {
		return true, false // no active run = no restriction
	}

	policy := &run.Context.ToolPolicy
	return policy.IsToolAllowed(tool), policy.NeedsApproval(tool)
}

// RecordGate records the result of a quality gate check.
func (e *SkillExecutor) RecordGate(runID string, gate GateResult) {
	e.mu.Lock()
	defer e.mu.Unlock()

	run, ok := e.active[runID]
	if !ok {
		return
	}

	run.Result.Gates = append(run.Result.Gates, gate)

	slog.Info("skill.gate.checked",
		"run_id", runID,
		"gate", gate.Gate,
		"passed", gate.Passed,
	)
}

// AllGatesPassed checks if all required quality gates have passed.
func (e *SkillExecutor) AllGatesPassed(runID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	run, ok := e.active[runID]
	if !ok {
		return false
	}

	required := make(map[string]bool)
	for _, g := range run.Spec.QualityGates {
		required[g] = true
	}

	passed := make(map[string]bool)
	for _, g := range run.Result.Gates {
		if g.Passed {
			passed[g.Gate] = true
		}
	}

	for gate := range required {
		if !passed[gate] {
			return false
		}
	}
	return true
}

// AddArtifact records an artifact produced by the skill.
func (e *SkillExecutor) AddArtifact(runID string, artifact ArtifactRef) {
	e.mu.Lock()
	defer e.mu.Unlock()

	run, ok := e.active[runID]
	if !ok {
		return
	}

	run.Result.Artifacts = append(run.Result.Artifacts, artifact)
}

// CompleteRun marks a run as completed and returns the final result.
func (e *SkillExecutor) CompleteRun(runID string, status SkillStatus) *SkillResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	run, ok := e.active[runID]
	if !ok {
		return &SkillResult{Status: SkillStatusFailed, Error: "run not found"}
	}

	run.Result.Status = status
	run.Result.Duration = time.Since(run.StartedAt)

	result := run.Result

	// Cleanup
	delete(e.active, runID)

	slog.Info("skill.run.completed",
		"run_id", runID,
		"status", status,
		"duration", result.Duration,
		"retries", result.Retries,
		"gates_passed", e.countPassedGates(result),
	)

	return result
}

// GetRun returns the current state of a run (read-only snapshot).
func (e *SkillExecutor) GetRun(runID string) (*SkillRun, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	run, ok := e.active[runID]
	return run, ok
}

// ActiveRuns returns the count of active skill runs (for metrics).
func (e *SkillExecutor) ActiveRuns() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.active)
}

// --- internal ---

// buildToolPolicy intersects the skill's AllowedTools with the context policy.
func (e *SkillExecutor) buildToolPolicy(spec *SkillSpec, ctxPolicy *ToolPolicy) ToolPolicy {
	policy := *ctxPolicy // copy

	// If skill declares AllowedTools, intersect with context allowlist
	if len(spec.AllowedTools) > 0 {
		if len(policy.Allowed) == 0 {
			// Context allows all, but skill restricts
			policy.Allowed = spec.AllowedTools
		} else {
			// Intersect: only tools in both lists
			intersect := make(map[string]bool)
			for _, t := range spec.AllowedTools {
				intersect[t] = true
			}
			var filtered []string
			for _, t := range policy.Allowed {
				if intersect[t] {
					filtered = append(filtered, t)
				}
			}
			policy.Allowed = filtered
		}
	}

	return policy
}

func (e *SkillExecutor) countPassedGates(r *SkillResult) int {
	n := 0
	for _, g := range r.Gates {
		if g.Passed {
			n++
		}
	}
	return n
}
