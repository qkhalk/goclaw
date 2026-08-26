package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
)

// TestReapIdleSessions_VerifiesJanitorSweep tests that ReapIdleSessions reaps
// a session queue when all of: queue empty, no active runs, and idle time
// exceeds the configured threshold.
func TestReapIdleSessions_VerifiesJanitorSweep(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.SessionIdleEvictMs = 1 // 1 ms threshold

	noopFn := func(ctx context.Context, req agent.RunRequest) (*agent.RunResult, error) {
		return &agent.RunResult{Content: "ok"}, nil
	}
	s := NewScheduler(nil, cfg, noopFn)
	defer s.Stop()

	// Enqueue and immediately complete so the queue is idle+empty.
	ctx := context.Background()
	out := s.Schedule(ctx, "agent:a:web:dm:u1", "test", agent.RunRequest{Input: "hi"})
	<-out

	// Wait past the threshold.
	time.Sleep(10 * time.Millisecond)

	reaped := s.ReapIdleSessions(1 * time.Millisecond)
	if reaped == 0 {
		t.Fatal("expected ReapIdleSessions to reap at least 1 session")
	}
}

// TestReapIdleSessions_SkipsActiveSession verifies that a session with an
// active run is not reaped.
func TestReapIdleSessions_SkipsActiveSession(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.SessionIdleEvictMs = 1

	block := make(chan struct{})
	slowFn := func(ctx context.Context, req agent.RunRequest) <-chan agent.RunOutcome {
		ch := make(chan agent.RunOutcome, 1)
		go func() {
			<-block
			ch <- agent.RunOutcome{Result: &agent.RunResult{Response: "ok"}}
		}()
		return ch
	}
	s := NewScheduler(nil, cfg, slowFn)
	defer s.Stop()

	ctx := context.Background()
	_ = s.Schedule(ctx, "agent:b:web:dm:u1", "slow", agent.RunRequest{Input: "go"})

	// Give the run time to start, then reap — active run must survive.
	time.Sleep(10 * time.Millisecond)
	reaped := s.ReapIdleSessions(1 * time.Millisecond)
	if reaped != 0 {
		t.Fatal("expected ReapIdleSessions to reap 0 (active run must survive)")
	}
	close(block) // unblock
}

// TestReapIdleSessions_SkipsNonIdle verifies that a session with a recent
// activity timestamp is not reaped.
func TestReapIdleSessions_SkipsNonIdle(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.SessionIdleEvictMs = 60_000 // 60 s threshold — no way to be idle in a test

	noopFn := func(ctx context.Context, req agent.RunRequest) (*agent.RunResult, error) {
		return &agent.RunResult{Content: "ok"}, nil
	}
	s := NewScheduler(nil, cfg, noopFn)
	defer s.Stop()

	ctx := context.Background()
	out := s.Schedule(ctx, "agent:c:web:dm:u1", "test", agent.RunRequest{Input: "hi"})
	<-out

	reaped := s.ReapIdleSessions(60_000 * time.Millisecond)
	if reaped != 0 {
		t.Fatal("expected ReapIdleSessions to reap 0 (idle threshold not exceeded)")
	}
}
