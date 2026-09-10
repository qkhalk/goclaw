package pipeline

import (
	"testing"
	"time"
)

func TestRecoveryDelayForRetryAfterCapped(t *testing.T) {
	e := NewRecoveryEngine(3, 60000)
	policy := RecoveryPolicy{BackoffKind: BackoffRetryAfter}

	// A gateway advertising a huge Retry-After must not park the run for an
	// hour — same 30s cap as providers.computeDelay.
	if d := e.delayFor(policy, 1, 1*time.Hour); d != 30*time.Second {
		t.Fatalf("Retry-After 1h: got %v, want 30s (capped)", d)
	}
	// Hints within the cap pass through verbatim.
	if d := e.delayFor(policy, 1, 7*time.Second); d != 7*time.Second {
		t.Fatalf("Retry-After 7s: got %v, want 7s (verbatim)", d)
	}
	// No hint → exponential backoff.
	if d := e.delayFor(policy, 1, 0); d <= 0 {
		t.Fatalf("no hint: got %v, want exponential backoff > 0", d)
	}
}
