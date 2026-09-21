package pipeline

import (
	"context"
	"testing"
)

func TestRunStateMarkContinuation(t *testing.T) {
	t.Parallel()

	var nilState *RunState
	nilState.MarkContinuation() // must not panic

	state := buildMinimalRunState()
	state.ExitCode = BreakLoop
	state.MarkContinuation()
	if !state.Resuming() {
		t.Fatal("MarkContinuation() did not set the resuming flag")
	}
	if state.ExitCode != Continue {
		t.Errorf("ExitCode = %v, want Continue (zero) like a restored checkpoint", state.ExitCode)
	}
}

// TestPipelineMarkContinuationResumesLikeCheckpoint pins the semantics the
// completion-verifier recover path relies on (agent/loop_run.go): after
// MarkContinuation, a second Pipeline.Run on the same live state skips setup,
// resumes the iteration counter, and consumes Observe.ContinueAfterFinal at
// the first break — one more iteration produces the kept final answer.
func TestPipelineMarkContinuationResumesLikeCheckpoint(t *testing.T) {
	t.Parallel()

	// iter breaks the loop on every call; the second pass's first call also
	// writes FinalContent to stand in for the model's recovered answer.
	var iter *mockStage
	iter = &mockStage{name: "iter", execFn: func(_ context.Context, state *RunState) error {
		if iter.execCnt == 2 {
			state.Observe.FinalContent = "recovered answer"
		}
		return nil
	}, result: BreakLoop}
	setup := newMockStageNoResult("setup")

	p := NewPipeline(
		[]Stage{setup},
		[]Stage{iter},
		nil,
		PipelineDeps{Config: PipelineConfig{MaxIterations: 3}},
	)

	state := buildMinimalRunState()
	if _, err := p.Run(context.Background(), state); err != nil {
		t.Fatalf("pass 1 Run() error: %v", err)
	}
	if setup.execCnt != 1 {
		t.Fatalf("pass 1 setup execCnt = %d, want 1", setup.execCnt)
	}
	if state.Iteration != 0 {
		t.Fatalf("pass 1 iteration = %d, want 0 (broke on first iteration)", state.Iteration)
	}

	// Simulate the verifier recover gate: flip ContinueAfterFinal, re-arm.
	state.Observe.ContinueAfterFinal = true
	state.MarkContinuation()

	result, err := p.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("pass 2 Run() error: %v", err)
	}
	if setup.execCnt != 1 {
		t.Errorf("pass 2 setup execCnt = %d, want 1 (setup must be skipped)", setup.execCnt)
	}
	if result.Content != "recovered answer" {
		t.Errorf("Content = %q, want %q", result.Content, "recovered answer")
	}
	// The flag was consumed at the pass-2 first break; one more iteration ran
	// to produce the kept final answer.
	if state.Iteration != 1 {
		t.Errorf("final iteration = %d, want 1", state.Iteration)
	}
}
