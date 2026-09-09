package agent

import (
	"strconv"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// RunSupervisor — proactive per-run resource enforcement (the resource-manager
// half of the supervisor design; the reactive half is the event-stream
// watchdog in watchdog.go, which classifies stuck runs from behavior).
//
// Coverage note: total tool calls per run are ALREADY capped by the pipeline
// (PipelineConfig.MaxToolCalls -> tool_stage.go); the supervisor deliberately
// does not duplicate that counter. It enforces the caps nothing else covers:
//
//	max LLM calls                — the next think-stage call is answered with
//	                               a stop-finish blocker instead of a provider
//	                               call (same pattern as the team-work
//	                               directive blocker)
//	consecutive tool failures    — the tool loop breaks exactly like a critical
//	                               tool-loop detection (final content +
//	                               loopKilled); toolloop.go detects behavioral
//	                               loops, not pure failure streaks
//	wall-clock run deadline      — checked on both paths; the watchdog is
//	                               reactive and the stale sweep terminal-fails
//	                               only after a much longer window
//
// A warning fires once per budget at warnPercent so the model gets a chance
// to wrap up before the hard stop. All methods are nil-safe (a nil supervisor
// allows everything) and safe for concurrent use.
type RunSupervisor struct {
	limits    SupervisorLimits
	startedAt time.Time

	mu                        sync.Mutex
	llmCalls                  int
	consecutiveToolFailures   int
	warnedLLMCalls            bool
	warnedDeadline            bool
	warnedConsecutiveFailures bool
}

// SupervisorLimits are the per-run hard caps. Zero values fall back to
// DefaultSupervisorLimits(); negative values disable that specific cap.
type SupervisorLimits struct {
	// MaxLLMCalls caps think-stage LLM calls per run (guard retries inside one
	// call are counted once — they share the bracketing event). Default 60.
	MaxLLMCalls int
	// MaxRunTime caps wall-clock duration per run. Default 30m.
	MaxRunTime time.Duration
	// MaxConsecutiveToolFailures caps back-to-back failed tool results.
	// Default 6.
	MaxConsecutiveToolFailures int
	// WarnAtPercent is the budget percentage at which a one-shot wrap-up
	// warning fires. Default 80.
	WarnAtPercent int
}

// DefaultSupervisorLimits returns the enforced-when-unconfigured defaults.
func DefaultSupervisorLimits() SupervisorLimits {
	return SupervisorLimits{
		MaxLLMCalls:                60,
		MaxRunTime:                 30 * time.Minute,
		MaxConsecutiveToolFailures: 6,
		WarnAtPercent:              80,
	}
}

// withDefaults returns a copy with zero values replaced by defaults.
func (l SupervisorLimits) withDefaults() SupervisorLimits {
	d := DefaultSupervisorLimits()
	if l.MaxLLMCalls == 0 {
		l.MaxLLMCalls = d.MaxLLMCalls
	}
	if l.MaxRunTime == 0 {
		l.MaxRunTime = d.MaxRunTime
	}
	if l.MaxConsecutiveToolFailures == 0 {
		l.MaxConsecutiveToolFailures = d.MaxConsecutiveToolFailures
	}
	if l.WarnAtPercent == 0 || l.WarnAtPercent > 100 {
		l.WarnAtPercent = d.WarnAtPercent
	}
	return l
}

// withDefaultsSafe resolves a nil pointer to the package defaults.
func (l *SupervisorLimits) withDefaultsSafe() SupervisorLimits {
	if l == nil {
		return DefaultSupervisorLimits()
	}
	return l.withDefaults()
}

// SupervisorLimitsFromConfig converts reliability.supervisor config into
// runtime limits. Lives in agent (not config) because config cannot import
// agent without an import cycle; cmd calls this at gateway wiring time.
func SupervisorLimitsFromConfig(cfg config.SupervisorConfig) *SupervisorLimits {
	return &SupervisorLimits{
		MaxLLMCalls:                cfg.MaxLLMCalls,
		MaxRunTime:                 time.Duration(cfg.MaxRunTimeMs) * time.Millisecond,
		MaxConsecutiveToolFailures: cfg.MaxConsecutiveToolFailures,
		WarnAtPercent:              cfg.WarnAtPercent,
	}
}

// SupervisorVerdict is the outcome of a supervisor check.
type SupervisorVerdict struct {
	// Allowed is false when a hard cap is exceeded: the caller must stop.
	Allowed bool
	// StopReason explains the stop (surfaced as the run's final content).
	StopReason string
	// Warning is a one-shot message at the warn threshold (empty after the
	// first fire).
	Warning string
}

func allowedVerdict() SupervisorVerdict { return SupervisorVerdict{Allowed: true} }

func stoppedVerdict(reason string) SupervisorVerdict {
	return SupervisorVerdict{Allowed: false, StopReason: reason}
}

// NewRunSupervisor creates the per-run supervisor. startedAt anchors the
// wall-clock deadline (pass the run's start time so the deadline reflects the
// whole run, not the supervisor's construction moment).
func NewRunSupervisor(limits SupervisorLimits, startedAt time.Time) *RunSupervisor {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &RunSupervisor{limits: limits.withDefaults(), startedAt: startedAt}
}

// RecordLLMCall counts one think-stage LLM call and returns the verdict: the
// caller skips the provider call and answers with the StopReason blocker when
// not allowed.
func (s *RunSupervisor) RecordLLMCall() SupervisorVerdict {
	if s == nil {
		return allowedVerdict()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llmCalls++

	var warning string
	if w := budgetWarning(s.warnedLLMCalls, s.limits.WarnAtPercent, s.llmCalls, s.limits.MaxLLMCalls, "LLM call"); w != "" {
		warning = w
		s.warnedLLMCalls = true
	}
	if s.limits.MaxLLMCalls > 0 && s.llmCalls > s.limits.MaxLLMCalls {
		return stoppedVerdict(supervisorStopMessage("LLM call", s.llmCalls, s.limits.MaxLLMCalls))
	}
	if v := checkDeadlineLocked(s.limits, s.startedAt, &s.warnedDeadline); !v.Allowed {
		return v
	}
	return SupervisorVerdict{Allowed: true, Warning: warning}
}

// RecordToolResult counts one completed tool execution and checks the
// consecutive-failure cap plus the run deadline. When not allowed, the caller
// breaks the run exactly like a critical tool-loop detection (final content +
// loopKilled + break).
func (s *RunSupervisor) RecordToolResult(isError bool) SupervisorVerdict {
	if s == nil {
		return allowedVerdict()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if isError {
		s.consecutiveToolFailures++
	} else {
		s.consecutiveToolFailures = 0
	}

	var warning string
	if s.limits.MaxConsecutiveToolFailures > 0 &&
		s.consecutiveToolFailures >= s.limits.MaxConsecutiveToolFailures {
		return stoppedVerdict(supervisorStopMessage(
			"consecutive failed tool call", s.consecutiveToolFailures, s.limits.MaxConsecutiveToolFailures))
	}
	if w := budgetWarning(s.warnedConsecutiveFailures, s.limits.WarnAtPercent,
		s.consecutiveToolFailures, s.limits.MaxConsecutiveToolFailures, "failed tool call"); w != "" {
		warning = w
		s.warnedConsecutiveFailures = true
	}
	if v := checkDeadlineLocked(s.limits, s.startedAt, &s.warnedDeadline); !v.Allowed {
		return v
	}
	return SupervisorVerdict{Allowed: true, Warning: warning}
}

// CheckDeadline reports whether the run has outlived its wall-clock budget.
// Checked on both the tool and LLM paths so runs made only of LLM calls still
// respect it.
func (s *RunSupervisor) CheckDeadline() SupervisorVerdict {
	if s == nil {
		return allowedVerdict()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return checkDeadlineLocked(s.limits, s.startedAt, &s.warnedDeadline)
}

func checkDeadlineLocked(limits SupervisorLimits, startedAt time.Time, warned *bool) SupervisorVerdict {
	if limits.MaxRunTime <= 0 {
		return allowedVerdict()
	}
	elapsed := time.Since(startedAt)
	if elapsed > limits.MaxRunTime {
		return stoppedVerdict(supervisorStopMessage(
			"minutes of run time", int(elapsed.Minutes()), int(limits.MaxRunTime.Minutes())))
	}
	if !*warned && limits.WarnAtPercent > 0 &&
		elapsed > limits.MaxRunTime*time.Duration(limits.WarnAtPercent)/100 {
		*warned = true
		return SupervisorVerdict{Allowed: true, Warning: supervisorWarningMessage("run time", limits.WarnAtPercent)}
	}
	return allowedVerdict()
}

// budgetWarning returns the one-shot warning when count crosses warnPct of
// max; alreadyWarned suppresses repeats.
func budgetWarning(alreadyWarned bool, warnPct, count, max int, what string) string {
	if alreadyWarned || max <= 0 || count <= max*warnPct/100 {
		return ""
	}
	return supervisorWarningMessage(what, warnPct)
}

func supervisorWarningMessage(what string, pct int) string {
	return "resource notice: this run has used " +
		strconv.Itoa(pct) + "% of its " + what + " budget — wrap up and deliver what you have."
}

func supervisorStopMessage(what string, used, max int) string {
	return "I stopped because this run exhausted its resource budget: " +
		strconv.Itoa(used) + " " + what + "(s) against a limit of " + strconv.Itoa(max) +
		". Partial work is preserved; start a new run to continue."
}
