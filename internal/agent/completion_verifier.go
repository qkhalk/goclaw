package agent

import (
	"log/slog"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/pipeline"
	"github.com/nextlevelbuilder/goclaw/internal/reliability"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// CompletionResult is the output of a run's completion verification (plan §11.2).
// It records whether the run produced a complete answer and which signals are
// missing. In advisory mode (default) the result is attached to run events and
// trace output only; recover/hard modes consume it in the terminal gate
// (gateCompletion).
type CompletionResult struct {
	Complete   bool
	Confidence float64
	Missing    []string
	Reason     string
}

// verifyCompletion inspects a finished pipeline run for completion signals:
//
//   - L0 (basic): the run produced final content.
//   - L1 (tool consistency): when the run used tools, at least one deliverable
//     was produced and every executed tool call has a recorded result (the
//     pipeline increments TotalToolCalls only after a result is processed, so
//     a nonzero count implies no pending calls).
//
// It is a deterministic, LLM-free check — no model judge, no DB writes. It
// exists to surface weak-model failure modes (empty output, premature
// completion with no deliverable) and — in the non-advisory terminal-gate
// modes — to gate the run.
func verifyCompletion(r *RunResult, s *pipeline.RunState) CompletionResult {
	if s == nil {
		return CompletionResult{
			Complete:   false,
			Confidence: 0,
			Missing:    []string{"content", "tool_state"},
			Reason:     "no run state available",
		}
	}

	content := strings.TrimSpace(s.Observe.FinalContent)
	toolCalls := s.Tool.TotalToolCalls
	deliverables := len(s.Tool.Deliverables)

	var missing []string
	// L0: final content must exist. A loop-killed run has a canned apology
	// string in FinalContent, so the loop kill is reported as missing content
	// only when nothing was actually produced.
	if content == "" {
		missing = append(missing, "content")
	}
	// L1: a run that used tools should also have produced a deliverable.
	// Chat-only runs (no tools) complete with content alone.
	if toolCalls > 0 && deliverables == 0 {
		missing = append(missing, "deliverable")
	}
	// L1: a run with no tools and no content has no completion signal at all.
	if toolCalls == 0 && content == "" && !s.Tool.LoopKilled {
		missing = append(missing, "tool_calls")
	}
	// A run force-stopped by the loop detector is never complete.
	if s.Tool.LoopKilled {
		missing = append(missing, "loop_killed")
	}

	complete := len(missing) == 0

	// Confidence blends the two levels: full when both pass; 0.5 when only the
	// L1 deliverable signal is missing (content present); 0.25 when L0 content
	// itself is missing; 0.1 when the run was force-stopped by the detector.
	confidence := 1.0
	if !complete {
		switch {
		case s.Tool.LoopKilled:
			confidence = 0.1
		case content == "":
			confidence = 0.25
		default:
			confidence = 0.5
		}
	}

	reason := "complete"
	if !complete {
		reason = "incomplete: missing " + strings.Join(missing, ", ")
	}

	return CompletionResult{
		Complete:   complete,
		Confidence: confidence,
		Missing:    missing,
		Reason:     reason,
	}
}

// effectiveVerifierMode resolves the completion-verifier mode for this loop:
// the per-Loop override wins (tests, embedders), otherwise the process-wide
// reliability bundle carries the mode plumbed from config at gateway startup
// (SetCompletionVerifier). Zero values resolve to advisory via
// config.EffectiveVerifierModeOf's clamping, keeping out-of-the-box behavior
// unchanged.
func (l *Loop) effectiveVerifierMode() string {
	if l.verifierMode != "" {
		return config.EffectiveVerifierModeOf(l.verifierMode)
	}
	if r := reliability.Default(); r != nil {
		return config.EffectiveVerifierModeOf(r.CompletionVerifierMode)
	}
	return config.VerifierModeAdvisory
}

// verifierGateDecision is what the terminal gate tells the run loop after the
// pipeline finished with an incomplete verdict.
type verifierGateDecision int

const (
	// verifierPass lets the run complete (verdict accepted, or advisory mode).
	verifierPass verifierGateDecision = iota

	// verifierContinue asks for exactly one more pipeline pass: recover mode
	// flips Observe.ContinueAfterFinal on the state and re-runs the pipeline;
	// the caller-side one-shot marker keeps the retry bounded per Run call.
	verifierContinue

	// verifierFail terminates the run as failed (hard semantics): incomplete
	// verdict on the first pass in hard mode, or on the second pass in
	// recover mode.
	verifierFail
)

// gateCompletion applies the configured verifier mode to an incomplete
// verdict AFTER the pipeline produced its final answer.
//
//   - hard: immediate fail — the run terminates AgentRunStatusFailed.
//   - recover: first incomplete verdict flips ContinueAfterFinal on the state
//     and returns continue; the caller re-runs the pipeline once more. A
//     second incomplete verdict falls through to hard semantics. No iteration
//     budget or when the in-pipeline ContinuationGate already fired ⇒ hard too.
//   - advisory: record-only pass (callers may skip the gate entirely).
//
// The verifying activity event (phase "verifying") is emitted before the
// decision so the timeline shows the gate ran regardless of outcome.
// continued is the caller's one-shot marker set (keyed by runID), distinct
// from Observe.ContinuationGateFired so the two gates compose.
func (l *Loop) gateCompletion(mode, runID string, completion *CompletionResult, state *pipeline.RunState, emitRun func(AgentEvent), continued map[string]bool) verifierGateDecision {
	emitVerifyingActivity(runID, emitRun)

	switch mode {
	case config.VerifierModeHard:
		slog.Warn("completion_verifier.hard_fail", "run_id", runID, "missing", completion.Missing)
		return verifierFail

	case config.VerifierModeRecover:
		if !continued[runID] &&
			state != nil &&
			l.maxIterations > 0 &&
			state.Iteration+1 < l.maxIterations &&
			!state.Observe.ContinuationGateFired {
			// Mirror contguard_stage.go: flip ContinueAfterFinal exactly like
			// the continuation gate does, then let the loop run one more pass.
			state.Observe.ContinueAfterFinal = true
			continued[runID] = true
			slog.Info("completion_verifier.recover_continue",
				"run_id", runID, "iteration", state.Iteration, "missing", completion.Missing)
			return verifierContinue
		}
		if continued[runID] {
			slog.Warn("completion_verifier.recover_exhausted", "run_id", runID, "missing", completion.Missing)
		} else {
			slog.Warn("completion_verifier.recover_unavailable", "run_id", runID, "missing", completion.Missing)
		}
		return verifierFail

	default:
		// Advisory (unknown values clamp to advisory upstream): record-only.
		return verifierPass
	}
}

// emitVerifyingActivity fires the phase:"verifying" activity event that makes
// RunTimelineStatusVerifying reachable (timelineKindForEvent maps activity
// events with phase verifying to store.RunTimelineStatusVerifying). Fired
// before the final gate decision in every non-advisory mode.
func emitVerifyingActivity(runID string, emitRun func(AgentEvent)) {
	if emitRun == nil || runID == "" {
		return
	}
	emitRun(AgentEvent{
		Type:    protocol.AgentEventActivity,
		RunID:   runID,
		Payload: map[string]any{"phase": "verifying"},
	})
}
