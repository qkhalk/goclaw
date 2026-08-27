package skills

import (
	"time"
)

// SkillSpec is the structured specification for a skill, parsed from SKILL.md
// frontmatter and enhanced with dependency/policy metadata. It extends the
// existing Metadata type with execution-related fields.
type SkillSpec struct {
	// Core identity (from Metadata)
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Version     string `json:"version,omitempty"`
	BaseDir     string `json:"baseDir"` // skill directory (parent of SKILL.md)

	// I/O contract
	Inputs    []string `json:"inputs,omitempty"`
	Outputs   []string `json:"outputs,omitempty"`
	DependsOn []string `json:"dependsOn,omitempty"` // skill slugs this skill depends on
	Provides  []string `json:"provides,omitempty"`  // capabilities this skill provides

	// Policy
	AllowedTools []string `json:"allowedTools,omitempty"` // empty = all tools allowed
	QualityGates []string `json:"qualityGates,omitempty"` // e.g. "tests_pass", "build_pass"

	// Execution
	MaxDuration time.Duration `json:"maxDuration,omitempty"` // 0 = no limit
	MaxRetries  int           `json:"maxRetries,omitempty"`  // 0 = default (3)
	MaxCost     float64       `json:"maxCost,omitempty"`     // 0 = no limit (USD)
	Priority    int           `json:"priority,omitempty"`    // lower = higher priority
	Timeout     time.Duration `json:"timeout,omitempty"`     // per-step timeout

	// Content
	Content string `json:"-"` // raw SKILL.md body (frontmatter stripped)
	Raw     string `json:"-"` // full raw file content
}

// SkillStatus represents the outcome of a skill execution.
type SkillStatus string

const (
	SkillStatusPending   SkillStatus = "pending"
	SkillStatusRunning   SkillStatus = "running"
	SkillStatusCompleted SkillStatus = "completed"
	SkillStatusFailed    SkillStatus = "failed"
	SkillStatusSkipped   SkillStatus = "skipped"   // dependency not met
	SkillStatusBlocked   SkillStatus = "blocked"   // quality gate failed
	SkillStatusCancelled SkillStatus = "cancelled" // timeout or user cancel
)

// GateResult records the outcome of a single quality gate check.
type GateResult struct {
	Gate    string `json:"gate"` // e.g. "tests_pass"
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
	Value   string `json:"value,omitempty"` // actual observed value
}

// ArtifactRef references an artifact produced by a skill execution.
type ArtifactRef struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "plan", "patch", "code", "test-report", etc.
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Checksum string `json:"checksum,omitempty"`
	Version  int    `json:"version,omitempty"`
}

// SkillResult is the output of a skill execution.
type SkillResult struct {
	Status    SkillStatus   `json:"status"`
	Outputs   []string      `json:"outputs,omitempty"`   // output values
	Artifacts []ArtifactRef `json:"artifacts,omitempty"` // produced artifacts
	Gates     []GateResult  `json:"gates,omitempty"`     // quality gate results
	Error     string        `json:"error,omitempty"`     // error message if failed
	Duration  time.Duration `json:"duration,omitempty"`  // execution duration
	Retries   int           `json:"retries,omitempty"`   // number of retries attempted
}

// IsTerminal returns true if the skill execution is in a final state.
func (s SkillStatus) IsTerminal() bool {
	switch s {
	case SkillStatusCompleted, SkillStatusFailed, SkillStatusSkipped, SkillStatusCancelled:
		return true
	}
	return false
}

// SkillContext is the runtime context passed to a skill executor.
// It provides access to all external systems the skill needs.
type SkillContext struct {
	// Run identity
	RunID     string `json:"runId"`
	TenantID  string `json:"tenantId"`
	UserID    string `json:"userId"`
	AgentID   string `json:"agentId"`
	AgentUUID string `json:"agentUuid"`

	// State
	WorkflowStep int    `json:"workflowStep"` // current step in workflow
	StepName     string `json:"stepName"`     // name of current step

	// Tool policy
	ToolPolicy ToolPolicy `json:"toolPolicy"`

	// Budget tracking
	BudgetUsed float64 `json:"budgetUsed"` // USD spent so far
	BudgetCap  float64 `json:"budgetCap"`  // max budget (0 = unlimited)

	// Quality gate state
	GateResults []GateResult `json:"gateResults,omitempty"`

	// Memory context
	MemoryEnabled bool `json:"memoryEnabled"`

	// Locale
	Locale string `json:"locale,omitempty"`
}

// ToolPolicy controls which tools a skill is allowed to use.
type ToolPolicy struct {
	// Allowed is an explicit allowlist. Empty = all tools allowed.
	Allowed []string `json:"allowed,omitempty"`

	// Denied is a denylist (takes precedence over Allowed).
	Denied []string `json:"denied,omitempty"`

	// RequireApproval tools that need user approval before execution.
	RequireApproval []string `json:"requireApproval,omitempty"`
}

// IsToolAllowed reports whether the given tool is permitted by this policy.
func (tp ToolPolicy) IsToolAllowed(tool string) bool {
	// Check denylist first
	for _, d := range tp.Denied {
		if d == tool {
			return false
		}
	}
	// If allowlist is empty, all tools allowed (except denied)
	if len(tp.Allowed) == 0 {
		return true
	}
	// Check allowlist
	for _, a := range tp.Allowed {
		if a == tool {
			return true
		}
	}
	return false
}

// NeedsApproval reports whether the tool requires user approval.
func (tp ToolPolicy) NeedsApproval(tool string) bool {
	for _, a := range tp.RequireApproval {
		if a == tool {
			return true
		}
	}
	return false
}
