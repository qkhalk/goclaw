package tools

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/hooks"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestPermissionOverrideTable proves the per-run permission mode maps each
// gated tool class to the right action, and that unknown/exec classes stay
// with the existing gating paths (mode does not apply).
func TestPermissionOverrideTable(t *testing.T) {
	cases := []struct {
		mode    string
		class   ToolClass
		action  classAction
		applies bool
	}{
		// No/unknown mode: never applies.
		{"", ToolClassWriteFile, actionAllow, false},
		{"bogus", ToolClassExec, actionAllow, false},
		// plan denies every class it applies to; exec self-gates.
		{PermModePlan, ToolClassExec, actionAllow, false},
		{PermModePlan, ToolClassWriteFile, actionDeny, true},
		{PermModePlan, ToolClassWorkstationExec, actionDeny, true},
		{PermModePlan, ToolClassBrowser, actionDeny, true},
		// full access allows everything gated.
		{PermModeFullAccess, ToolClassWriteFile, actionAllow, true},
		{PermModeFullAccess, ToolClassBrowser, actionAllow, true},
		// write approval: mutating classes ask, browser reads pass.
		{PermModeWriteApproval, ToolClassWriteFile, actionAsk, true},
		{PermModeWriteApproval, ToolClassWorkstationExec, actionAsk, true},
		{PermModeWriteApproval, ToolClassBrowser, actionAllow, true},
		// always ask hits every non-exec class.
		{PermModeAlwaysAsk, ToolClassWriteFile, actionAsk, true},
		{PermModeAlwaysAsk, ToolClassBrowser, actionAsk, true},
		{PermModeAlwaysAsk, ToolClassExec, actionAllow, false},
		// Unclassified tools are never mode-gated.
		{PermModePlan, "", actionAllow, false},
	}
	for _, tc := range cases {
		got, ok := permissionOverride(tc.mode, tc.class)
		if got != tc.action || ok != tc.applies {
			t.Fatalf("permissionOverride(%q, %q) = (%v, %v), want (%v, %v)",
				tc.mode, tc.class, got, ok, tc.action, tc.applies)
		}
	}
}

// TestGateToolCallPlanModeDenies proves the gate consults the per-run mode
// from ctx: in plan mode a write_file call is denied even when the configured
// policy is off. Config on the manager is empty, so the deny can only come
// from the override.
func TestGateToolCallPlanModeDenies(t *testing.T) {
	m := NewExecApprovalManager(ExecApprovalConfig{})
	ctx := store.WithRunContext(context.Background(), &store.RunContext{PermissionMode: PermModePlan})
	allow, reason := m.GateToolCall(ctx, hooks.ApprovalQuery{ToolName: "write_file", SessionKey: "s1"})
	if allow {
		t.Fatalf("plan mode must deny write_file, got allow")
	}
	if reason == "" {
		t.Fatalf("deny reason must be set")
	}
}

// TestGateToolCallFullAccessAllows proves full_access bypasses a configured
// ask policy for the same call.
func TestGateToolCallFullAccessAllows(t *testing.T) {
	m := NewExecApprovalManager(ExecApprovalConfig{
		ToolPolicies: map[ToolClass]ToolApprovalMode{ToolClassWriteFile: ToolModeAsk},
	})
	ctx := store.WithRunContext(context.Background(), &store.RunContext{PermissionMode: PermModeFullAccess})
	allow, _ := m.GateToolCall(ctx, hooks.ApprovalQuery{ToolName: "write_file", SessionKey: "s1"})
	if !allow {
		t.Fatalf("full_access must bypass ask policy, got deny")
	}
}

// TestGateToolCallDefaultUsesConfig proves the no-mode path still enforces
// the configured policy (regression guard for the override insertion).
func TestGateToolCallDefaultUsesConfig(t *testing.T) {
	m := NewExecApprovalManager(ExecApprovalConfig{
		ToolPolicies: map[ToolClass]ToolApprovalMode{ToolClassWriteFile: ToolModeDeny},
	})
	ctx := context.Background() // no RunContext → no mode
	allow, _ := m.GateToolCall(ctx, hooks.ApprovalQuery{ToolName: "write_file", SessionKey: "s1"})
	if allow {
		t.Fatalf("configured deny must still apply without a permission mode")
	}
}
