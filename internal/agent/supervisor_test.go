package agent

import (
	"strings"
	"testing"
	"time"
)

func TestSupervisorNilSafe(t *testing.T) {
	var sup *RunSupervisor
	if v := sup.RecordLLMCall(); !v.Allowed || v.StopReason != "" || v.Warning != "" {
		t.Fatalf("nil RecordLLMCall = %+v, want allowed", v)
	}
	if v := sup.RecordToolResult(true); !v.Allowed {
		t.Fatalf("nil RecordToolResult = %+v, want allowed", v)
	}
	if v := sup.CheckDeadline(); !v.Allowed {
		t.Fatalf("nil CheckDeadline = %+v, want allowed", v)
	}
}

func TestSupervisorDefaultsResolve(t *testing.T) {
	d := SupervisorLimits{}.withDefaults()
	want := DefaultSupervisorLimits()
	if d != want {
		t.Fatalf("zero limits resolved to %+v, want %+v", d, want)
	}
	// Explicit values win over defaults.
	custom := SupervisorLimits{MaxLLMCalls: 5}.withDefaults()
	if custom.MaxLLMCalls != 5 {
		t.Fatalf("explicit MaxLLMCalls = %d, want 5", custom.MaxLLMCalls)
	}
	// WarnAtPercent > 100 falls back to default.
	bad := SupervisorLimits{WarnAtPercent: 150}.withDefaults()
	if bad.WarnAtPercent != want.WarnAtPercent {
		t.Fatalf("WarnAtPercent = %d, want default %d", bad.WarnAtPercent, want.WarnAtPercent)
	}
}

func TestSupervisorLLMCallCap(t *testing.T) {
	sup := NewRunSupervisor(SupervisorLimits{MaxLLMCalls: 3, WarnAtPercent: 60}, time.Now())

	// Calls 1-2 allowed without warning; call 2 fires the 60% warning once.
	v1 := sup.RecordLLMCall()
	if !v1.Allowed || v1.Warning != "" {
		t.Fatalf("call 1 = %+v, want allowed no warning", v1)
	}
	v2 := sup.RecordLLMCall()
	if !v2.Allowed || v2.Warning == "" {
		t.Fatalf("call 2 = %+v, want allowed with warn-threshold warning", v2)
	}
	v3 := sup.RecordLLMCall()
	if !v3.Allowed || v3.Warning != "" {
		t.Fatalf("call 3 = %+v, want allowed, warning already fired", v3)
	}
	v4 := sup.RecordLLMCall()
	if v4.Allowed {
		t.Fatal("call 4 must be blocked at the hard cap")
	}
	if !strings.Contains(v4.StopReason, "LLM call") {
		t.Fatalf("stop reason %q should mention the budget", v4.StopReason)
	}
}

func TestSupervisorConsecutiveToolFailures(t *testing.T) {
	sup := NewRunSupervisor(SupervisorLimits{MaxConsecutiveToolFailures: 3, WarnAtPercent: 60}, time.Now())

	// Two failures then a success resets the streak.
	for i := 0; i < 2; i++ {
		if v := sup.RecordToolResult(true); !v.Allowed {
			t.Fatalf("failure %d = %+v, want allowed", i+1, v)
		}
	}
	if v := sup.RecordToolResult(false); !v.Allowed {
		t.Fatalf("success reset = %+v, want allowed", v)
	}
	if v := sup.RecordToolResult(true); !v.Allowed {
		t.Fatalf("failure after reset = %+v, want allowed", v)
	}
	// Three consecutive failures trip the cap.
	for i := 0; i < 2; i++ {
		if v := sup.RecordToolResult(true); !v.Allowed {
			t.Fatalf("streak failure %d = %+v, want allowed", i+1, v)
		}
	}
	v := sup.RecordToolResult(true)
	if v.Allowed {
		t.Fatal("third consecutive failure must be blocked")
	}
	if !strings.Contains(v.StopReason, "consecutive failed tool call") {
		t.Fatalf("stop reason %q should mention failures", v.StopReason)
	}
}

func TestSupervisorDeadline(t *testing.T) {
	// Started 31 minutes ago against a 30m deadline: immediate stop.
	sup := NewRunSupervisor(SupervisorLimits{MaxRunTime: 30 * time.Minute}, time.Now().Add(-31*time.Minute))
	if v := sup.CheckDeadline(); v.Allowed {
		t.Fatal("deadline must be exceeded")
	}
	if v := sup.RecordLLMCall(); v.Allowed {
		t.Fatal("LLM call must be blocked past the deadline")
	}
	if v := sup.RecordToolResult(false); v.Allowed {
		t.Fatal("tool result must be blocked past the deadline")
	}

	// Started 29 minutes ago: allowed, but the wrap-up warning fires.
	warned := NewRunSupervisor(SupervisorLimits{MaxRunTime: 30 * time.Minute, WarnAtPercent: 90}, time.Now().Add(-29*time.Minute))
	v := warned.CheckDeadline()
	if !v.Allowed || v.Warning == "" {
		t.Fatalf("near-deadline = %+v, want allowed with warning", v)
	}
	// The warning fires once.
	if v := warned.CheckDeadline(); v.Warning != "" {
		t.Fatalf("second check = %+v, warning must fire once", v)
	}

	// Negative MaxRunTime disables the deadline entirely.
	disabled := NewRunSupervisor(SupervisorLimits{MaxRunTime: -1}, time.Now().Add(-100*time.Hour))
	if v := disabled.CheckDeadline(); !v.Allowed {
		t.Fatalf("disabled deadline = %+v, want allowed", v)
	}
}

func TestSupervisorNegativeCapsDisable(t *testing.T) {
	sup := NewRunSupervisor(SupervisorLimits{
		MaxLLMCalls:                -1,
		MaxRunTime:                 -1,
		MaxConsecutiveToolFailures: -1,
	}, time.Now())
	for i := 0; i < 500; i++ {
		if v := sup.RecordLLMCall(); !v.Allowed {
			t.Fatalf("call %d blocked despite disabled cap: %+v", i+1, v)
		}
		if v := sup.RecordToolResult(true); !v.Allowed {
			t.Fatalf("failure %d blocked despite disabled cap: %+v", i+1, v)
		}
	}
}
