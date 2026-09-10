// Recovery engine (Wave 1 WS-C): a unified classify → policy → action layer
// for ThinkStage failure paths.
//
// Before this file the think stage carried three independent counters
// (TruncRetries=3, EmptyReplyRetries=2, OverflowRetries≤1/3) with no shared
// budget and no canonical error classification. The recovery engine routes
// every weak-model / provider failure through one policy table mapped from the
// reliability taxonomy (internal/reliability errors.go) and one global
// per-run budget (reliability.recovery.max_retry_count / max_retry_time_ms).
//
// Behavior contract: with default config the observable retry semantics are
// UNCHANGED — truncation still retries 3×, empty replies 2×, context overflow
// compacts once (budget-reduction 3×). The policy table documents those counts
// as class defaults and the global budget only engages beyond them.
package pipeline

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/reliability"
)

// ---------------------------------------------------------------------------
// Policy table
// ---------------------------------------------------------------------------

// RecoveryAction is what the engine tells ThinkStage to do after classifying a
// failure.
type RecoveryAction int

const (
	// RecoveryProceed means the failure was handled inline (repair applied /
	// nudge queued) and the iteration should continue normally.
	RecoveryProceed RecoveryAction = iota
	// RecoveryRetry means the caller should re-enter the same failure path
	// (retry the LLM call or reduction step) — budget was granted.
	RecoveryRetry
	// RecoveryGiveUp means the global recovery budget is exhausted or the
	// class is non-retryable: surface the failure to the caller.
	RecoveryGiveUp
)

// BackoffKind selects the delay computation between recovery attempts.
type BackoffKind int

const (
	// BackoffNone applies no delay (in-process repairs and nudges).
	BackoffNone BackoffKind = iota
	// BackoffRetryAfter honors a provider-supplied Retry-After hint verbatim,
	// falling back to exponential backoff when absent.
	BackoffRetryAfter
	// BackoffExponential uses exponential backoff with ±10% jitter
	// (mirrors providers.RetryDo computeDelay).
	BackoffExponential
)

// String renders the backoff kind for logs and tests.
func (b BackoffKind) String() string {
	switch b {
	case BackoffRetryAfter:
		return "retry_after"
	case BackoffExponential:
		return "exponential"
	default:
		return "none"
	}
}

// RecoveryPolicy is the per-class recovery contract from roadmap §6: each
// error class defines whether it retries, how often, with which backoff, and
// which escalation levers (model fallback, strategy switch) are allowed.
type RecoveryPolicy struct {
	// Retryable mirrors the taxonomy's canonical retryability. The engine
	// never retries a non-retryable class regardless of remaining budget.
	Retryable bool
	// MaxAttempts bounds consecutive recoveries of THIS class within one run
	// (the per-class counter; distinct from the global run budget).
	MaxAttempts int
	// BackoffKind selects the inter-attempt delay computation.
	BackoffKind BackoffKind
	// BaseBackoff seeds exponential backoff; ignored for BackoffNone.
	BaseBackoff time.Duration
	// FallbackAllowed marks classes where escalating to a fallback model is a
	// legitimate recovery lever (consumed by ModelFallbackProvider paths).
	FallbackAllowed bool
	// StrategySwitch marks classes where switching conversation strategy
	// (compaction, corrective prompt, tool-choice) is preferred over a plain
	// provider retry.
	StrategySwitch bool
}

// DefaultRecoveryBaseBackoff seeds exponential delays for transport-level
// classes. Small enough not to stall interactive runs, large enough to let a
// recovering upstream breathe between attempts.
const DefaultRecoveryBaseBackoff = 500 * time.Millisecond

// recoveryPolicies maps every reliability taxonomy class relevant to the
// think-stage failure paths onto its recovery policy. Classes reached through
// the tool loop detector (looping, repeated calls) keep their existing
// observe-only wiring in loop_tools.go and are listed here for completeness so
// the table stays the single source of truth for roadmap §6.
var recoveryPolicies = map[reliability.ErrorCode]RecoveryPolicy{
	// --- Provider transport classes ---------------------------------------
	reliability.ErrProviderRateLimited: {
		Retryable: true, MaxAttempts: 8,
		BackoffKind: BackoffRetryAfter, BaseBackoff: DefaultRecoveryBaseBackoff,
		FallbackAllowed: true,
	},
	reliability.ErrProviderOverloaded: {
		Retryable: true, MaxAttempts: 8,
		BackoffKind: BackoffExponential, BaseBackoff: DefaultRecoveryBaseBackoff,
		FallbackAllowed: true,
	},
	reliability.ErrProviderServerError: {
		Retryable: true, MaxAttempts: 8,
		BackoffKind: BackoffExponential, BaseBackoff: DefaultRecoveryBaseBackoff,
		FallbackAllowed: true,
	},
	reliability.ErrProviderTimeout: {
		Retryable: true, MaxAttempts: 5,
		BackoffKind:     BackoffExponential,
		BaseBackoff:     DefaultRecoveryBaseBackoff,
		FallbackAllowed: true,
	},
	reliability.ErrProviderConnection: {
		Retryable: true, MaxAttempts: 5,
		BackoffKind:     BackoffExponential,
		BaseBackoff:     DefaultRecoveryBaseBackoff,
		FallbackAllowed: true,
	},
	reliability.ErrProviderContextOverflow: {
		// Context overflow recovers by SHRINKING the request (compaction),
		// never by hammering the same oversized request.
		Retryable:      true,
		MaxAttempts:    3,
		StrategySwitch: true,
	},
	reliability.ErrProviderInvalidResponse: {
		Retryable: true, MaxAttempts: 3,
		BackoffKind: BackoffExponential, BaseBackoff: DefaultRecoveryBaseBackoff,
	},

	// Permanent provider conditions: fail safe, no retry, no fallback
	// (a billing/auth problem follows the run to any candidate).
	reliability.ErrProviderAuth:          {Retryable: false},
	reliability.ErrProviderAuthPermanent: {Retryable: false},
	reliability.ErrProviderBadRequest:    {Retryable: false},
	reliability.ErrProviderBilling:       {Retryable: false},
	reliability.ErrProviderModelNotFound: {Retryable: false},
	reliability.ErrProviderContentPolicy: {Retryable: false},

	// --- Weak-model semantic classes --------------------------------------
	reliability.ErrModelEmptyOutput: {
		// Recovered by nudging the model for a real answer.
		Retryable:      true,
		MaxAttempts:    2, // = maxEmptyReplyRetries (documented default)
		StrategySwitch: true,
	},
	reliability.ErrModelMalformedToolCall: {
		// Recovered by the repair-then-corrective-hint path.
		Retryable:      true,
		MaxAttempts:    3, // = maxTruncRetries (documented default)
		StrategySwitch: true,
	},
	reliability.ErrModelInvalidJSON: {
		// Repair-first; falls under the malformed-call retry ladder when the
		// repair cannot restore a parseable payload.
		Retryable:      true,
		MaxAttempts:    3,
		StrategySwitch: true,
	},
	reliability.ErrModelPrematureCompletion: {
		// Handled by the ContinuationGate (one continuation per run).
		Retryable:      true,
		MaxAttempts:    1,
		StrategySwitch: true,
	},
	reliability.ErrModelRepeatedToolCall: {
		// Warning-level loop detection injects a corrective prompt.
		Retryable:      true,
		MaxAttempts:    2,
		StrategySwitch: true,
	},
	reliability.ErrModelLooping: {
		// Critical loop detection force-stops the run; never retried.
		Retryable: false,
	},

	// --- Runtime / verification classes -----------------------------------
	reliability.ErrRunCancelled:      {Retryable: false},
	reliability.ErrRunDeadline:       {Retryable: false},
	reliability.ErrRunStalled:        {Retryable: true, MaxAttempts: 2, StrategySwitch: true},
	reliability.ErrRunRecoveryFailed: {Retryable: false},
	// Verification failures recover by CONTINUING the conversation (asking the
	// model to finish), NOT by re-sending the request to the provider.
	reliability.ErrModelLowSignal: {Retryable: true, MaxAttempts: 1, StrategySwitch: true},
}

// PolicyFor returns the recovery policy for an error code. Unknown codes get
// the conservative default: retryable with a small bounded attempt count and
// generic exponential backoff (fail safe without amplifying the failure).
func PolicyFor(code reliability.ErrorCode) RecoveryPolicy {
	if p, ok := recoveryPolicies[code]; ok {
		return p
	}
	return RecoveryPolicy{
		Retryable:   true,
		MaxAttempts: 3,
		BackoffKind: BackoffExponential,
		BaseBackoff: DefaultRecoveryBaseBackoff,
	}
}

// ClassifyWeakResponse maps a structurally-broken but transport-successful
// response onto the weak-model classes of the taxonomy (roadmap Phase 7:
// empty / "..." / invalid JSON / wrong tool / missing arg / premature done).
// Detection order matters: finish_reason signals (truncation) beat content
// heuristics because a truncated call can look empty-ish too.
func ClassifyWeakResponse(resp *weakResponseView) reliability.ErrorCode {
	if resp == nil {
		return reliability.ErrModelEmptyOutput
	}
	// Truncated tool-call arguments: Gemini emits finish_reason="tool_calls"
	// even when max_tokens cut the args off (empty args on allowlisted tools).
	if resp.TruncatedToolCalls {
		return reliability.ErrModelMalformedToolCall
	}
	// Provider could not parse the arguments JSON at all.
	if resp.ParseErrorToolCalls {
		return reliability.ErrModelInvalidJSON
	}
	// No text, no tool calls, no images: the classic weak-model shrug
	// (includes the literal "..." placeholder reply).
	if resp.EmptyFinal {
		return reliability.ErrModelEmptyOutput
	}
	// Text present but it is a low-signal filler ("...", "done", whitespace).
	if resp.LowSignalText {
		return reliability.ErrModelLowSignal
	}
	return ""
}

// weakResponseView is the minimal projection of a ChatResponse the classifier
// needs. Kept separate from providers.ChatResponse so tests can exercise the
// classifier without building full responses.
type weakResponseView struct {
	TruncatedToolCalls  bool
	ParseErrorToolCalls bool
	EmptyFinal          bool
	LowSignalText       bool
}

// isLowSignalReply reports whether a final text answer carries no usable
// signal ("...", "…", "done", whitespace). Bounded list: expanding it requires
// telemetry justification (same bar as mutatingToolsRequireArgs).
func isLowSignalReply(content string) bool {
	switch normalizeReplySignal(content) {
	case "", ".", "..", "...", "…", "-", "done", "ok":
		return true
	default:
		return false
	}
}

// normalizeReplySignal lowercases and trims common decorations around a reply.
func normalizeReplySignal(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case ' ', '\t', '\n', '\r', '*', '_', '`', '"', '\'':
			i++
		default:
			out = append(out, c)
			i++
		}
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Global recovery budget
// ---------------------------------------------------------------------------

// DefaultRecoveryMaxRetryCount / DefaultRecoveryMaxRetryTime mirror the
// config-layer constants in internal/config (DefaultRecoveryMaxRetryCount=8,
// DefaultRecoveryMaxRetryTimeMs=300000). Duplicated as literals so the
// pipeline package does not import internal/config (it already does for
// CompactionConfig, but keeping the engine self-describing makes the budget
// contract greppable next to the policy table).
const (
	DefaultRecoveryMaxRetryCount = 8
	DefaultRecoveryMaxRetryTime  = 300 * time.Second
)

// RecoveryBudget is the GLOBAL per-run recovery spend tracker (roadmap §8:
// max_retry_time / max_retry_count across ALL recovery actions, not per
// class). One instance lives on each RunState via Think.Recovery.
type RecoveryBudget struct {
	// MaxAttempts caps total recovery actions per run (0 → default 8).
	MaxAttempts int
	// MaxTime caps cumulative wall-clock spent in recovery per run (0 →
	// default 5m).
	MaxTime time.Duration

	attempts int
	spent    time.Duration
}

// newRecoveryBudget resolves zero-valued fields to the production defaults.
func newRecoveryBudget(maxAttempts int, maxTime time.Duration) *RecoveryBudget {
	if maxAttempts <= 0 {
		maxAttempts = DefaultRecoveryMaxRetryCount
	}
	if maxTime <= 0 {
		maxTime = DefaultRecoveryMaxRetryTime
	}
	return &RecoveryBudget{MaxAttempts: maxAttempts, MaxTime: maxTime}
}

// Allow reports whether one more recovery action fits the budget. It does not
// consume anything — callers consume via Spend after acting.
func (b *RecoveryBudget) Allow() bool {
	if b == nil {
		return false // nil budget = engine disabled: never authorize recovery
	}
	return b.attempts < b.MaxAttempts && b.spent < b.MaxTime
}

// Spend records one consumed recovery action costing d wall-clock time.
func (b *RecoveryBudget) Spend(d time.Duration) {
	if b == nil {
		return
	}
	b.attempts++
	b.spent += d
	// Guard pathological clock jumps: spending must never free budget.
	if b.spent < 0 {
		b.spent = b.MaxTime
	}
}

// Attempts reports consumed recovery actions (test/observability hook).
func (b *RecoveryBudget) Attempts() int {
	if b == nil {
		return 0
	}
	return b.attempts
}

// Spent reports accumulated recovery wall-clock (test/observability hook).
func (b *RecoveryBudget) Spent() time.Duration {
	if b == nil {
		return 0
	}
	return b.spent
}

// Exhausted reports whether the budget can no longer authorize any action.
func (b *RecoveryBudget) Exhausted() bool {
	return !b.Allow()
}

// ---------------------------------------------------------------------------
// Decision engine
// ---------------------------------------------------------------------------

// RecoveryDecision is the outcome of evaluating one failure against the
// policy table + global budget.
type RecoveryDecision struct {
	Action RecoveryAction
	Code   reliability.ErrorCode
	// RetryIn is the delay to apply before the retry attempt. Zero for
	// BackoffNone and for GiveUp decisions.
	RetryIn time.Duration
	// Reason is a short machine-readable explanation for logs/tests.
	Reason string
}

// RecoveryEngine evaluates failures for ONE run. It pairs the static policy
// table with the run-scoped global budget and per-run class counters.
type RecoveryEngine struct {
	budget *RecoveryBudget
	// classAttempts counts consecutive recovery attempts per error class.
	classAttempts map[reliability.ErrorCode]int
	// rand sources jitter; swappable for deterministic tests.
	jitter func() float64
}

// NewRecoveryEngine builds an engine bound to a run's global budget resolved
// from reliability.recovery config values (0 → production defaults).
func NewRecoveryEngine(cfgMaxAttempts, cfgMaxTimeMs int) *RecoveryEngine {
	maxTime := time.Duration(cfgMaxTimeMs) * time.Millisecond
	return &RecoveryEngine{
		budget:        newRecoveryBudget(cfgMaxAttempts, maxTime),
		classAttempts: make(map[reliability.ErrorCode]int),
		jitter:        rand.Float64,
	}
}

// Budget exposes the underlying budget for gateway wiring and assertions.
func (e *RecoveryEngine) Budget() *RecoveryBudget { return e.budget }

// resetClasses clears per-class consecutive-attempt pressure for the given
// codes without touching the global budget. ThinkStage mirrors its legacy
// counters on a healthy response: truncation and overflow pressure reset,
// empty-reply pressure deliberately PERSISTS (maxEmptyReplyRetries bounds
// nudges per RUN, not consecutive ones).
func (e *RecoveryEngine) resetClasses(codes ...reliability.ErrorCode) {
	if e == nil {
		return
	}
	for _, c := range codes {
		delete(e.classAttempts, c)
	}
}

// Evaluate classifies a failure code, consults the policy + both budgets, and
// returns the action to take. attemptHint feeds Retry-After-aware backoff for
// rate-limit errors carrying a server hint.
func (e *RecoveryEngine) Evaluate(code reliability.ErrorCode, retryAfter time.Duration) RecoveryDecision {
	policy := PolicyFor(code)

	if !policy.Retryable {
		return RecoveryDecision{Action: RecoveryGiveUp, Code: code, Reason: "class_not_retryable"}
	}
	// Global budget gates everything, including retryable classes.
	if !e.budget.Allow() {
		return RecoveryDecision{Action: RecoveryGiveUp, Code: code, Reason: "global_recovery_budget_exhausted"}
	}
	if e.classAttempts[code] >= policy.MaxAttempts {
		return RecoveryDecision{Action: RecoveryGiveUp, Code: code, Reason: "class_attempts_exhausted"}
	}

	e.classAttempts[code]++
	delay := e.delayFor(policy, e.classAttempts[code], retryAfter)
	e.budget.Spend(delay)

	return RecoveryDecision{
		Action:  RecoveryRetry,
		Code:    code,
		RetryIn: delay,
		Reason:  "policy_retry",
	}
}

// Authorize checks whether one INLINE repair/nudge action (which also consumes
// a recovery slot) may proceed, consuming on success. Used by paths that fix
// rather than retry: JSON repair, corrective hints, emergency compaction.
func (e *RecoveryEngine) Authorize(code reliability.ErrorCode) bool {
	policy := PolicyFor(code)
	if !policy.Retryable || !e.budget.Allow() || e.classAttempts[code] >= policy.MaxAttempts {
		return false
	}
	e.classAttempts[code]++
	e.budget.Spend(0)
	return true
}

// delayFor computes the post-attempt delay. Called AFTER incrementing the
// counter, so attempt starts at 1.
func (e *RecoveryEngine) delayFor(policy RecoveryPolicy, attempt int, retryAfter time.Duration) time.Duration {
	switch policy.BackoffKind {
	case BackoffNone:
		return 0
	case BackoffRetryAfter:
		if retryAfter > 0 {
			// Honoured up to the 30s cap, mirroring providers.RetryDo
			// (computeDelay): a gateway advertising Retry-After: 3600 must not
			// park the run for an hour.
			if retryAfter > 30*time.Second {
				retryAfter = 30 * time.Second
			}
			return retryAfter
		}
		return e.exponential(policy, attempt)
	default:
		return e.exponential(policy, attempt)
	}
}

// exponential computes min(cap, base·2^(attempt-1)) with ±10% jitter, mirroring
// providers.computeDelay semantics.
func (e *RecoveryEngine) exponential(policy RecoveryPolicy, attempt int) time.Duration {
	base := policy.BaseBackoff
	if base <= 0 {
		base = DefaultRecoveryBaseBackoff
	}
	delay := float64(base) * math.Pow(2, float64(attempt-1))
	// Cap at 30s like the provider retry ladder.
	if delay > float64(30*time.Second) {
		delay = float64(30 * time.Second)
	}
	jittered := delay + (e.jitter()*2-1)*delay*0.10
	if jittered < 0 {
		jittered = float64(base)
	}
	return time.Duration(jittered)
}
