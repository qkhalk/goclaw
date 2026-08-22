package agent

import (
	"context"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// Tests for the run-level watchdog (Wave 1 WS-D): D1 classification
// boundaries and D2 ladder escalation order. The router is nil everywhere —
// nudges/aborts degrade to logged no-ops, which is exactly what the
// assertions rely on (actuation is observable through Sweep's return value).

func newTestWatchdog() *Watchdog {
	return NewWatchdog(nil, WatchdogConfig{})
}

// --- D1: classification boundaries -----------------------------------------

func TestWatchdogVerdictHealthy(t *testing.T) {
	w := newTestWatchdog()
	now := time.Now()
	w.ObserveToolCall("run-1", "sess-1", "read_file", "h1", now)

	if got := w.Classify("run-1", now.Add(5*time.Second)); got != VerdictHealthy {
		t.Errorf("Classify = %q, want healthy for a fresh active run", got)
	}
}

func TestWatchdogVerdictStalledAfterSilence(t *testing.T) {
	w := newTestWatchdog()
	now := time.Now()
	w.ObserveAssistantOutput("run-2", "sess-1", "working", now)

	// Just below the 90s stall window → still healthy.
	if got := w.Classify("run-2", now.Add(89*time.Second)); got == VerdictStalled {
		t.Error("Classify = stalled before the stall window elapsed")
	}
	// At/after the window → stalled.
	if got := w.Classify("run-2", now.Add(90*time.Second)); got != VerdictStalled {
		t.Errorf("Classify = %q, want stalled after 90s of silence", got)
	}
}

func TestWatchdogVerdictLoopingOnRepeatedTool(t *testing.T) {
	w := newTestWatchdog()
	now := time.Now()
	for i := 0; i < DefaultWatchdogSameToolRepeat; i++ {
		w.ObserveToolCall("run-3", "sess-1", "exec", "same-args-hash", now.Add(time.Duration(i)*time.Second))
	}
	if got := w.Classify("run-3", now); got != VerdictLooping {
		t.Errorf("Classify = %q, want looping after %d identical tool calls", got, DefaultWatchdogSameToolRepeat)
	}

	// A different call resets the streak — back under threshold.
	w.ObserveToolCall("run-3", "sess-1", "exec", "other-args-hash", now)
	if got := w.Classify("run-3", now); got != VerdictHealthy {
		t.Errorf("Classify = %q, want healthy after the streak broke", got)
	}
}

func TestWatchdogVerdictLoopingOnRepeatedOutput(t *testing.T) {
	w := newTestWatchdog()
	now := time.Now()
	for i := 0; i < DefaultWatchdogSameOutputRepeat; i++ {
		w.ObserveAssistantOutput("run-4", "sess-1", "I will help you with that.", now)
	}
	if got := w.Classify("run-4", now); got != VerdictLooping {
		t.Errorf("Classify = %q, want looping after %d byte-identical outputs", got, DefaultWatchdogSameOutputRepeat)
	}
}

func TestWatchdogVerdictRecoveringStuckNoArtifact(t *testing.T) {
	w := newTestWatchdog()
	now := time.Now()

	// Tokens grow, but nothing deliverable ever lands.
	for i := 0; i < 5; i++ {
		w.ObserveUsage("run-5", "sess-1", 1000*(i+1), now.Add(time.Duration(i)*time.Second))
		// Keep liveness alive so the stall detector does not win.
		w.Observe(AgentEvent{Type: protocol.AgentEventActivity, RunID: "run-5"}, now.Add(time.Duration(i)*time.Second))
	}
	// Past StallAfter since start, still no artifact → recovering-stuck beats slow.
	got := w.Classify("run-5", now.Add(DefaultWatchdogStallAfter))
	if got != VerdictRecoveringStuck {
		t.Errorf("Classify = %q, want recovering_stuck (budget growth, no artifact)", got)
	}
}

func TestWatchdogVerdictSlowLongRunning(t *testing.T) {
	w := newTestWatchdog()
	start := time.Now()

	// Active, producing distinct artifacts, but running long. The last
	// observation lands at the evaluation instant so the stall detector
	// (which wins on recency) stays quiet and the duration rule decides.
	last := start.Add(DefaultWatchdogSlowAfter)
	for i := 0; i < 10; i++ {
		at := start.Add(time.Duration(i) * (DefaultWatchdogSlowAfter / 10))
		if at.After(last) {
			at = last
		}
		w.ObserveAssistantOutput("run-6", "sess-1", "step result "+string(rune('a'+i)), at)
	}
	if got := w.Classify("run-6", last); got != VerdictSlow {
		t.Errorf("Classify = %q, want slow for a long-running but progressing run", got)
	}
}

func TestWatchdogForgetClearsState(t *testing.T) {
	w := newTestWatchdog()
	now := time.Now()
	w.ObserveUsage("run-7", "sess-1", 5000, now)

	// Simulate terminal event via Observe (run.completed).
	w.Observe(AgentEvent{Type: protocol.AgentEventRunCompleted, RunID: "run-7"}, now)

	if got := w.Classify("run-7", now.Add(time.Hour)); got != VerdictHealthy {
		t.Errorf("Classify after forget = %q, want healthy (unknown run)", got)
	}
}

// --- D2: ladder escalation order -------------------------------------------

func TestWatchdogLadderEscalatesBeforeAbort(t *testing.T) {
	w := newTestWatchdog()
	ctx := context.Background()
	start := time.Now()

	// A stalled run observed repeatedly. Rung schedule per applyLadder:
	// depth 0 = checkpoint (log), 1 = nudge, 2 = strategy-switch (log),
	// 3 = model-fallback (log), 4+ = safe-fail abort. With a nil router the
	// nudge is not delivered (Sweep counts only delivered nudges), but the
	// abort at depth>=4 must count as actuated.
	w.ObserveToolCall("run-l", "sess-1", "exec", "hash", start)

	// Silence past the stall window so verdict = stalled every sweep.
	later := start.Add(2 * DefaultWatchdogStallAfter)

	// Sweep 1 → depth 0 (checkpoint, log-only, not actuated).
	if n := w.Sweep(ctx, later); n != 0 {
		t.Errorf("Sweep#1 acted on %d runs, want 0 (checkpoint rung)", n)
	}
	// Sweep 2 → depth 1 (nudge attempt; nil router ⇒ not delivered ⇒ not counted).
	if n := w.Sweep(ctx, later); n != 0 {
		t.Errorf("Sweep#2 acted on %d runs, want 0 (nudge with nil router)", n)
	}
	// Sweep 3 → depth 2 (strategy-switch observation).
	if n := w.Sweep(ctx, later); n != 0 {
		t.Errorf("Sweep#3 acted on %d runs, want 0 (strategy-switch rung)", n)
	}
	// Sweep 4 → depth 3 (model-fallback observation).
	if n := w.Sweep(ctx, later); n != 0 {
		t.Errorf("Sweep#4 acted on %d runs, want 0 (model-fallback rung)", n)
	}
	// Sweep 5 → depth 4 (safe-fail abort — actuated even with nil router).
	if n := w.Sweep(ctx, later); n != 1 {
		t.Errorf("Sweep#5 acted on %d runs, want 1 (safe-fail)", n)
	}
}

func TestWatchdogNudgeCooldownSuppressesRepeat(t *testing.T) {
	w := newTestWatchdog()
	start := time.Now()
	w.ObserveToolCall("run-n", "sess-1", "exec", "hash", start)
	later := start.Add(2 * DefaultWatchdogStallAfter)

	_ = w.Sweep(context.Background(), later) // depth 0
	_ = w.Sweep(context.Background(), later) // depth 1: nudge recorded

	// Directly verify the cooldown bookkeeping the nudge rung relies on:
	// a second nudge inside defaultWatchdogNudgeCooldown must be suppressed.
	w.mu.Lock()
	last, tracked := w.nudged["run-n"]
	w.mu.Unlock()
	if !tracked {
		t.Fatal("nudge timestamp not recorded at depth 1")
	}
	if last.IsZero() || last.Before(later.Add(-defaultWatchdogNudgeCooldown)) {
		t.Errorf("nudge last = %v, want within cooldown window of %v", last, later)
	}
}

func TestWatchdogNilSafe(t *testing.T) {
	var w *Watchdog
	now := time.Now()

	// Every entry point tolerates a nil receiver.
	w.Observe(AgentEvent{RunID: "x"}, now)
	w.ObserveToolCall("x", "s", "t", "h", now)
	w.ObserveAssistantOutput("x", "s", "c", now)
	w.ObserveUsage("x", "s", 10, now)
	if got := w.Classify("x", now); got != VerdictHealthy {
		t.Errorf("nil Classify = %q, want healthy", got)
	}
	if n := w.Sweep(context.Background(), now); n != 0 {
		t.Errorf("nil Sweep acted on %d runs, want 0", n)
	}
}
