package hooks

import (
	"context"
	"sync/atomic"
)

// ApprovalQuery describes a pre-tool-use call routed to the approval engine.
// ArgsDigest is the canonical sha256 of (tool name, canonical args JSON) as
// returned by CanonicalInputHash; it keys allow-once grants so a retried or
// resumed identical call passes without a second prompt.
type ApprovalQuery struct {
	ToolName   string
	ToolInput  map[string]any
	ArgsDigest string
	// SessionKey scopes allow-for-session grants to the requesting session.
	SessionKey string
	// Risk is a human-readable risk hint (e.g. the asking hook's reason).
	Risk string
}

// ApprovalGate is the seam between the hook pipeline and the tool approval
// engine (owned by internal/tools; cmd wiring installs the implementation via
// SetApprovalGate). Two flows route through it:
//
//   - GateToolCall: per-tool-class policy gating (config
//     tools.execApproval.tool_policies), evaluated once per pre_tool_use
//     event after the hook chain allowed the call.
//   - ResolveAsk: a pre_tool_use hook returned DecisionAsk — instead of
//     degrading to block, the engine creates an approval request and waits;
//     a human approval unblocks the call and records an allow-once grant for
//     (tool, argsDigest).
//
// denyReason is a human-readable explanation surfaced as the block reason.
type ApprovalGate interface {
	GateToolCall(ctx context.Context, q ApprovalQuery) (allow bool, denyReason string)
	ResolveAsk(ctx context.Context, q ApprovalQuery) (allow bool, denyReason string)
}

// approvalGateLookup mirrors the SetBuiltinAllowlistLookup pattern: a
// package-level atomic pointer installed once at startup by cmd wiring. The
// dispatcher reads it per Fire, so installation order relative to dispatcher
// construction does not matter and tests can swap/clear it safely.
var approvalGateLookup atomic.Pointer[ApprovalGate]

// SetApprovalGate installs the approval engine seam. Pass nil to clear (used
// by defer cleanup in tests). When unset, tool-class policies are not enforced
// and DecisionAsk degrades to block (Wave 1 behavior).
func SetApprovalGate(g ApprovalGate) {
	if g == nil {
		approvalGateLookup.Store(nil)
		return
	}
	approvalGateLookup.Store(&g)
}

// approvalGate returns the installed seam, or nil when unset.
func approvalGate() ApprovalGate {
	if fp := approvalGateLookup.Load(); fp != nil {
		return *fp
	}
	return nil
}

// approvalQueryFor builds an ApprovalQuery from a pre_tool_use event. The
// digest computation failure degrades to an empty digest: grants simply never
// match (fail closed for grants), while the approval flow itself is unaffected.
func approvalQueryFor(ev Event, risk string) ApprovalQuery {
	digest, _ := CanonicalInputHash(ev.ToolName, ev.ToolInput)
	return ApprovalQuery{
		ToolName:   ev.ToolName,
		ToolInput:  ev.ToolInput,
		ArgsDigest: digest,
		SessionKey: ev.SessionID,
		Risk:       risk,
	}
}
