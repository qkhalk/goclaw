package tools

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseTimeoutArg(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want int
	}{
		{"missing", map[string]any{}, 0},
		{"not a number", map[string]any{"timeout_seconds": "30"}, 0},
		{"below minimum", map[string]any{"timeout_seconds": float64(0)}, 0},
		{"negative", map[string]any{"timeout_seconds": float64(-5)}, 0},
		{"valid", map[string]any{"timeout_seconds": float64(300)}, 300},
		{"clamped to max", map[string]any{"timeout_seconds": float64(99999)}, ExecMaxTimeoutSeconds},
	}
	for _, tc := range cases {
		if got := parseTimeoutArg(tc.args); got != tc.want {
			t.Errorf("%s: parseTimeoutArg = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestExecTimeoutOverridePreservesPartialOutput runs a command that prints
// progress and then sleeps far beyond the per-call override; the result must
// report the timeout AND carry the output produced before the kill.
func TestExecTimeoutOverridePreservesPartialOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh semantics")
	}
	tool := NewExecTool(t.TempDir(), false)
	tool.timeout = 60 * time.Second // tool default must be overridden by the arg

	res := tool.Execute(t.Context(), map[string]any{
		"command":         "echo before-death-line; sleep 60",
		"timeout_seconds": float64(2),
	})
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %+v", res)
	}
	if !strings.Contains(res.ForLLM, "timed out after 2s") {
		t.Fatalf("timeout notice missing: %q", res.ForLLM)
	}
	if !strings.Contains(res.ForLLM, "before-death-line") {
		t.Fatalf("partial output lost: %q", res.ForLLM)
	}
}

// TestExecTimeoutOverrideBeatsToolDefault verifies a command that outlives a
// short tool default completes when the per-call override extends the budget.
func TestExecTimeoutOverrideBeatsToolDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh semantics")
	}
	tool := NewExecTool(t.TempDir(), false)
	tool.timeout = 1 * time.Second

	res := tool.Execute(t.Context(), map[string]any{
		"command":         "sleep 2; echo slow-but-finished",
		"timeout_seconds": float64(10),
	})
	if res == nil || res.IsError {
		t.Fatalf("expected success with override, got %+v", res)
	}
	if !strings.Contains(res.ForLLM, "slow-but-finished") {
		t.Fatalf("output missing: %q", res.ForLLM)
	}
}
