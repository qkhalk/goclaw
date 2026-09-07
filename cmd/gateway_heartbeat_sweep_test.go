package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// fakeSweepRuns is a minimal RunsStore double for sweepStaleRunsOnce: it
// implements the legacy count path and the detail capability so both branches
// are exercisable. Only what the sweep touches is real; everything else panics
// so an unexpected call fails loudly.
type fakeSweepRuns struct {
	store.RunsStore // embed for the interface; every used method is overridden

	failed   []store.AgentRun
	detailed bool
}

func (f *fakeSweepRuns) RecoverStaleRuns(ctx context.Context, staleAfter time.Duration) (int64, error) {
	return int64(len(f.failed)), nil
}

func (f *fakeSweepRuns) RecoverStaleRunsWithDetail(ctx context.Context, staleAfter time.Duration) ([]store.AgentRun, error) {
	f.detailed = true
	return f.failed, nil
}

func TestSweepStaleRunsOnceNotifiesTerminalFailures(t *testing.T) {
	runID := "146b0889-7b59-483a-a7a0-694b0352b057"
	runs := &fakeSweepRuns{failed: []store.AgentRun{{
		RunID:      runID,
		SessionKey: "agent:fox-spirit:ws:direct:abc",
		UserID:     "user-1",
		Channel:    "ws",
		Status:     store.AgentRunStatusFailed,
		Error:      "run stalled: heartbeat expired",
	}}}

	var notified []store.AgentRun
	sweepStaleRunsOnce(context.Background(), runs, time.Minute, nil, func(r store.AgentRun) {
		notified = append(notified, r)
	})

	if !runs.detailed {
		t.Fatal("sweep did not use the detail-capable path")
	}
	if len(notified) != 1 {
		t.Fatalf("notify called %d times, want 1", len(notified))
	}
	if notified[0].RunID != runID {
		t.Errorf("notified run = %q, want %q", notified[0].RunID, runID)
	}
	if notified[0].SessionKey == "" {
		t.Error("notified run missing session key for event routing")
	}
}

func TestSweepStaleRunsOnceNoFailuresNoNotify(t *testing.T) {
	runs := &fakeSweepRuns{}
	called := false
	sweepStaleRunsOnce(context.Background(), runs, time.Minute, nil, func(store.AgentRun) {
		called = true
	})
	if called {
		t.Error("notify must not fire when the sweep fails no runs")
	}
}

// legacySweepRuns hides the detail capability so the count-only fallback runs.
type legacySweepRuns struct {
	store.RunsStore
	count int64
}

func (f *legacySweepRuns) RecoverStaleRuns(ctx context.Context, staleAfter time.Duration) (int64, error) {
	return f.count, nil
}

func TestSweepStaleRunsOnceLegacyFallback(t *testing.T) {
	runs := &legacySweepRuns{count: 3}
	called := false
	sweepStaleRunsOnce(context.Background(), runs, time.Minute, nil, func(store.AgentRun) {
		called = true
	})
	if called {
		t.Error("notify must not fire on the count-only legacy path")
	}
}

// fakeDetailTenantCheck ensures the notify path receives routing fields.
func TestSweepNotifyPayloadRoutingFields(t *testing.T) {
	tid := uuid.MustParse("0193a5b0-7000-7000-8000-000000000001")
	agentID := uuid.MustParse("0196f0a0-7000-7000-8000-000000000002")
	runs := &fakeSweepRuns{failed: []store.AgentRun{{
		RunID:      "run-1",
		SessionKey: "agent:a:ws:direct:k",
		AgentID:    &agentID,
		UserID:     "user-1",
		Channel:    "ws",
		ChatID:     "chat-9",
		TenantID:   tid,
		Status:     store.AgentRunStatusFailed,
		Error:      "run stalled: heartbeat expired",
	}}}

	sweepStaleRunsOnce(context.Background(), runs, time.Minute, nil, func(r store.AgentRun) {
		if r.TenantID != tid {
			t.Errorf("tenant = %v, want %v", r.TenantID, tid)
		}
		if r.Channel != "ws" {
			t.Errorf("channel = %q, want ws", r.Channel)
		}
		if r.Error == "" {
			t.Error("error text missing")
		}
	})
}
