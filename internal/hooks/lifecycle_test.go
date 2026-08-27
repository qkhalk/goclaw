package hooks

import (
	"testing"
)

func TestExtendedEventsExist(t *testing.T) {
	events := []HookEvent{
		EventBeforeRun, EventAfterRun,
		EventBeforeLLM, EventAfterLLM,
		EventBeforeCheckpoint, EventAfterCheckpoint,
		EventBeforeComplete, EventAfterComplete,
		EventOnError, EventOnRateLimit,
	}
	for _, e := range events {
		if e == "" {
			t.Error("empty event constant")
		}
	}
}

func TestIsBlockingExtended(t *testing.T) {
	tests := []struct {
		event HookEvent
		want  bool
	}{
		{EventBeforeRun, true},
		{EventAfterRun, false},
		{EventBeforeLLM, true},
		{EventAfterLLM, false},
		{EventBeforeCheckpoint, true},
		{EventAfterCheckpoint, false},
		{EventBeforeComplete, true},
		{EventAfterComplete, false},
		{EventOnError, false},
		{EventOnRateLimit, false},
	}
	for _, tt := range tests {
		if got := isBlockingExtended(tt.event); got != tt.want {
			t.Errorf("isBlockingExtended(%q) = %v, want %v", tt.event, got, tt.want)
		}
	}
}

func TestIsBlocking_Integrated(t *testing.T) {
	// Extended blocking events should return true from the main IsBlocking
	if !EventBeforeRun.IsBlocking() {
		t.Error("EventBeforeRun should be blocking")
	}
	if !EventBeforeLLM.IsBlocking() {
		t.Error("EventBeforeLLM should be blocking")
	}
	if EventAfterRun.IsBlocking() {
		t.Error("EventAfterRun should not be blocking")
	}
	if EventOnError.IsBlocking() {
		t.Error("EventOnError should not be blocking")
	}
	// Original events still work
	if !EventPreToolUse.IsBlocking() {
		t.Error("EventPreToolUse should still be blocking")
	}
	if EventStop.IsBlocking() {
		t.Error("EventStop should not be blocking")
	}
}

func TestFailurePolicy(t *testing.T) {
	tests := []struct {
		policy FailurePolicy
		valid  bool
	}{
		{FailurePolicyContinue, true},
		{FailurePolicyBlock, true},
		{FailurePolicyAbort, true},
		{FailurePolicy("invalid"), false},
	}
	for _, tt := range tests {
		switch tt.policy {
		case FailurePolicyContinue, FailurePolicyBlock, FailurePolicyAbort:
			if !tt.valid {
				t.Errorf("policy %q should be valid", tt.policy)
			}
		default:
			if tt.valid {
				t.Errorf("policy %q should be invalid", tt.policy)
			}
		}
	}
}

func TestGetFailurePolicy_Default(t *testing.T) {
	cfg := HookConfig{Metadata: map[string]any{}}
	if got := GetFailurePolicy(cfg); got != FailurePolicyContinue {
		t.Errorf("default = %q, want %q", got, FailurePolicyContinue)
	}
}

func TestGetFailurePolicy_FromMetadata(t *testing.T) {
	cfg := HookConfig{Metadata: map[string]any{"failure_policy": "abort"}}
	if got := GetFailurePolicy(cfg); got != FailurePolicyAbort {
		t.Errorf("from metadata = %q, want %q", got, FailurePolicyAbort)
	}
}

func TestIsToolAllowedByHook_Nil(t *testing.T) {
	if !IsToolAllowedByHook(nil, "shell") {
		t.Error("nil permissions should allow all tools")
	}
}

func TestIsToolAllowedByHook_Denylist(t *testing.T) {
	p := &HookPermissions{DeniedTools: []string{"shell"}}
	if IsToolAllowedByHook(p, "shell") {
		t.Error("shell should be denied")
	}
	if !IsToolAllowedByHook(p, "write_file") {
		t.Error("write_file should be allowed")
	}
}

func TestIsToolAllowedByHook_Allowlist(t *testing.T) {
	p := &HookPermissions{AllowedTools: []string{"write_file", "read_file"}}
	if !IsToolAllowedByHook(p, "write_file") {
		t.Error("write_file should be allowed")
	}
	if IsToolAllowedByHook(p, "shell") {
		t.Error("shell should be denied (not in allowlist)")
	}
}
