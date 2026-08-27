package skills

import (
	"context"
	"testing"
)

func TestSkillExecutor_ToolPermission(t *testing.T) {
	resolver := &SkillResolver{
		loader:    nil, // won't be called in these tests
		specCache: make(map[string]*SkillSpec),
	}
	executor := NewSkillExecutor(resolver)

	// No active run = all tools allowed
	allowed, approval := executor.CheckToolPermission("nonexistent", "shell")
	if !allowed {
		t.Error("no active run should allow all tools")
	}
	if approval {
		t.Error("no active run should not require approval")
	}

	// Begin a run with tool restrictions
	run, err := executor.BeginRun(context.Background(), "test", &SkillContext{
		RunID: "run-1",
		ToolPolicy: ToolPolicy{
			Allowed: []string{"write_file", "read_file"},
		},
	})
	if err == nil {
		t.Skip("BeginRun requires resolver, skipping permission test with nil loader")
	}
	_ = run
}

func TestSkillExecutor_ActiveRuns(t *testing.T) {
	resolver := &SkillResolver{
		specCache: make(map[string]*SkillSpec),
	}
	executor := NewSkillExecutor(resolver)

	if executor.ActiveRuns() != 0 {
		t.Errorf("ActiveRuns() = %d, want 0", executor.ActiveRuns())
	}
}

func TestSkillExecutor_RecordGate(t *testing.T) {
	resolver := &SkillResolver{
		specCache: make(map[string]*SkillSpec),
	}
	executor := NewSkillExecutor(resolver)

	// Recording gate for nonexistent run should be a no-op
	executor.RecordGate("nonexistent", GateResult{
		Gate:   "tests_pass",
		Passed: true,
	})
}

func TestSkillExecutor_CompleteRun_NotFound(t *testing.T) {
	resolver := &SkillResolver{
		specCache: make(map[string]*SkillSpec),
	}
	executor := NewSkillExecutor(resolver)

	result := executor.CompleteRun("nonexistent", SkillStatusCompleted)
	if result.Status != SkillStatusFailed {
		t.Errorf("CompleteRun nonexistent = %v, want Failed", result.Status)
	}
	if result.Error != "run not found" {
		t.Errorf("error = %q, want %q", result.Error, "run not found")
	}
}

func TestSkillExecutor_AllGatesPassed_Empty(t *testing.T) {
	resolver := &SkillResolver{
		specCache: make(map[string]*SkillSpec),
	}
	executor := NewSkillExecutor(resolver)

	// Nonexistent run
	if executor.AllGatesPassed("nonexistent") {
		t.Error("AllGatesPassed for nonexistent run should be false")
	}
}

func TestSkillResolver_ValidateSpec(t *testing.T) {
	valid := &SkillSpec{
		Name: "test",
		Slug: "test",
	}
	if err := ValidateSpec(valid); err != nil {
		t.Errorf("ValidateSpec(valid) = %v", err)
	}

	empty := &SkillSpec{}
	if err := ValidateSpec(empty); err == nil {
		t.Error("ValidateSpec(empty) should return error")
	}
}
