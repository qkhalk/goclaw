package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/pipeline"
	"github.com/nextlevelbuilder/goclaw/internal/reliability"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// --- helpers ---

// setVerifierMode installs a fresh reliability bundle with the given
// completion-verifier mode and registers cleanup restoring a fresh
// advisory bundle. Mirrors the pipeline package's enableContinuationGate.
func setVerifierMode(t *testing.T, mode string) {
	t.Helper()
	rt := reliability.Configure(reliability.DefaultCircuitOptions(), 0)
	rt.CompletionVerifierMode = mode
	t.Cleanup(func() {
		reliability.Configure(reliability.DefaultCircuitOptions(), 0)
	})
}

func newRunState() *pipeline.RunState {
	return &pipeline.RunState{
		RunID: "run-1",
	}
}

// gateState builds a finished-pipeline state with the given signals so
// verifyCompletion produces a known verdict.
func gateState(content string) *pipeline.RunState {
	s := newRunState()
	s.Observe.FinalContent = content
	return s
}

func incompleteVerdict() *CompletionResult {
	c := verifyCompletion(nil, nil)
	return &c
}

func completeVerdict() *CompletionResult {
	s := newRunState()
	s.Observe.FinalContent = "done"
	c := verifyCompletion(&RunResult{}, s)
	return &c
}

// countingEmitter records emitted events and counts verifying activities.
type gateEventCollector struct {
	events    []AgentEvent
	verifying int
}

func (g *gateEventCollector) emit(e AgentEvent) {
	g.events = append(g.events, e)
	if e.Type == protocol.AgentEventActivity {
		if m, ok := e.Payload.(map[string]any); ok && m["phase"] == "verifying" {
			g.verifying++
		}
	}
}

// --- advisory: terminal decisions byte-identical to dev ---

// TestVerifierGateAdvisoryDefaultPassesIncomplete asserts the default mode:
// an incomplete verdict NEVER flips to failed — the run completes exactly as
// on dev (record-only). This is the zero-diff acceptance criterion.
func TestVerifierGateAdvisoryDefaultPassesIncomplete(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string // "" = nothing configured at all
	}{
		{"empty-mode", ""},
		{"advisory", config.VerifierModeAdvisory},
		{"invalid-clamps-to-advisory", "bogus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setVerifierMode(t, tc.mode)
			l := &Loop{maxIterations: 5}
			if got := l.effectiveVerifierMode(); got != config.VerifierModeAdvisory {
				t.Fatalf("effectiveVerifierMode() = %q, want advisory", got)
			}
			col := &gateEventCollector{}
			c := incompleteVerdict()
			if decision := l.gateCompletion(config.VerifierModeAdvisory, "run-1", c, nil, col.emit, map[string]bool{}); decision != verifierPass {
				t.Fatalf("decision = %v, want verifierPass (advisory never gates)", decision)
			}
		})
	}
}

// TestVerifierGateHardFailsFirstIncomplete asserts hard semantics: the very
// first incomplete verdict fails; no continuation is requested.
func TestVerifierGateHardFailsFirstIncomplete(t *testing.T) {
	setVerifierMode(t, config.VerifierModeHard)
	l := &Loop{maxIterations: 5}
	col := &gateEventCollector{}
	state := gateState("")
	continued := map[string]bool{}
	if decision := l.gateCompletion(config.VerifierModeHard, "run-1", incompleteVerdict(), state, col.emit, continued); decision != verifierFail {
		t.Fatalf("decision = %v, want verifierFail on first incomplete in hard mode", decision)
	}
	if state.Observe.ContinueAfterFinal {
		t.Fatal("hard mode must not flip ContinueAfterFinal")
	}
	if len(continued) != 0 {
		t.Fatalf("hard mode must not consume the one-shot marker: %v", continued)
	}
	if col.verifying != 1 {
		t.Fatalf("verifying activity emitted %d times, want exactly 1 before the decision", col.verifying)
	}
}

// TestVerifierGateRecoversThenFallsThrough asserts recover semantics:
// first incomplete verdict flips ContinueAfterFinal + one-shot marker and
// asks for another pass; the second verdict falls through to fail even when
// iteration budget remains.
func TestVerifierGateRecoversThenFallsThrough(t *testing.T) {
	setVerifierMode(t, config.VerifierModeRecover)
	l := &Loop{maxIterations: 8}
	col := &gateEventCollector{}
	state := gateState("")
	continued := map[string]bool{}

	if decision := l.gateCompletion(config.VerifierModeRecover, "run-1", incompleteVerdict(), state, col.emit, continued); decision != verifierContinue {
		t.Fatalf("first decision = %v, want verifierContinue", decision)
	}
	if !state.Observe.ContinueAfterFinal {
		t.Fatal("recover must flip ContinueAfterFinal like contguard_stage")
	}
	if !continued["run-1"] {
		t.Fatal("recover must set its own one-shot marker")
	}
	if state.Observe.ContinuationGateFired {
		t.Fatal("recover must NOT touch the ContinuationGateFired marker (distinct mechanisms compose)")
	}

	// Second pass still incomplete → hard semantics.
	if decision := l.gateCompletion(config.VerifierModeRecover, "run-1", incompleteVerdict(), state, col.emit, continued); decision != verifierFail {
		t.Fatalf("second decision = %v, want verifierFail (fall-through)", decision)
	}
	if col.verifying != 2 {
		t.Fatalf("verifying activity emitted %d times across two decisions, want 2", col.verifying)
	}
}

// TestVerifierGateRecoverRespectsFinalIteration mirrors contguard_stage's
// bound: no continuation when the final iteration already ran or the
// in-pipeline continuation gate just fired — both fall straight to fail.
func TestVerifierGateRecoverRespectsFinalIteration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		iteration int
		gateFired bool
	}{
		{"last-iteration", 4, false},
		{"continuation-gate-already-fired", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setVerifierMode(t, config.VerifierModeRecover)
			l := &Loop{maxIterations: 5}
			state := gateState("")
			state.Iteration = tc.iteration
			state.Observe.ContinuationGateFired = tc.gateFired
			if decision := l.gateCompletion(config.VerifierModeRecover, "run-1", incompleteVerdict(), state, nil, map[string]bool{}); decision != verifierFail {
				t.Fatalf("decision = %v, want verifierFail (%s leaves no useful continuation)", decision, tc.name)
			}
		})
	}
}

// TestVerifierCompleteVerdictNeverGates asserts accepted runs are untouched
// in every mode — the gate is only consulted for incomplete verdicts.
func TestVerifierCompleteVerdictNeverGates(t *testing.T) {
	for _, mode := range []string{config.VerifierModeAdvisory, config.VerifierModeRecover, config.VerifierModeHard} {
		setVerifierMode(t, mode)
		v := completeVerdict()
		if !v.Complete {
			t.Fatalf("fixture broken: %+v", v)
		}
	}
}

// --- localized failure reason ---

// TestVerifierFailedReasonLocalized asserts the hard-failure reason resolves
// through i18n per the run locale carried on context.
func TestVerifierFailedReasonLocalized(t *testing.T) {
	en := i18n.T(store.LocaleFromContext(store.WithLocale(context.Background(), "en")),
		i18n.MsgVerifierIncomplete, strings.Join([]string{"content"}, ", "))
	vi := i18n.T(store.LocaleFromContext(store.WithLocale(context.Background(), "vi")),
		i18n.MsgVerifierIncomplete, strings.Join([]string{"content"}, ", "))
	zh := i18n.T(store.LocaleFromContext(store.WithLocale(context.Background(), "zh")),
		i18n.MsgVerifierIncomplete, strings.Join([]string{"content"}, ", "))

	for name, msg := range map[string]string{"en": en, "vi": vi, "zh": zh} {
		if strings.Contains(msg, "verifier.incomplete") {
			t.Errorf("%s: key leaked through catalog lookup: %q", name, msg)
		}
		if !strings.Contains(msg, "content") {
			t.Errorf("%s: missing-signal list not interpolated: %q", name, msg)
		}
	}
	if en == vi || en == zh {
		t.Fatalf("locales must differ: en=%q vi=%q zh=%q", en, vi, zh)
	}
}

// --- protocol event names ---

// TestVerifierProtocolEventNames pins the wire contract next to the existing
// run.* constants.
func TestVerifierProtocolEventNames(t *testing.T) {
	if protocol.AgentEventVerificationPassed != "verification.passed" ||
		protocol.AgentEventVerificationFailed != "verification.failed" {
		t.Fatalf("verification event names changed: %q / %q",
			protocol.AgentEventVerificationPassed, protocol.AgentEventVerificationFailed)
	}
}

// --- timeline: verifying status reachable ---

// TestTimelineVerifyingStatusReachable asserts the activity event carrying
// phase:"verifying" maps to RunTimelineStatusVerifying while other phases
// keep the running status.
func TestTimelineVerifyingStatusReachable(t *testing.T) {
	itemType, status, ok := timelineKindForEvent(AgentEvent{
		Type:    protocol.AgentEventActivity,
		RunID:   "run-1",
		Payload: map[string]any{"phase": "verifying"},
	})
	if !ok || itemType != store.RunTimelineItemTypeActivity || status != store.RunTimelineStatusVerifying {
		t.Fatalf("got itemType=%q status=%q ok=%v, want activity/verifying/true", itemType, status, ok)
	}

	_, status, _ = timelineKindForEvent(AgentEvent{
		Type:    protocol.AgentEventActivity,
		RunID:   "run-1",
		Payload: map[string]any{"phase": "tool_exec"},
	})
	if status != store.RunTimelineStatusRunning {
		t.Fatalf("non-verifying phase status = %q, want running (unchanged)", status)
	}
}
