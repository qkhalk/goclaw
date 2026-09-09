package hooks_test

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/hooks"
)

// fakeGate scripts the approval seam's answers and records the queries it
// received, letting tests assert what the dispatcher routed through it.
type fakeGate struct {
	gateAllow   bool
	gateReason  string
	askAllow    bool
	askReason   string
	gateQueries []hooks.ApprovalQuery
	askQueries  []hooks.ApprovalQuery
}

func (g *fakeGate) GateToolCall(_ context.Context, q hooks.ApprovalQuery) (bool, string) {
	g.gateQueries = append(g.gateQueries, q)
	return g.gateAllow, g.gateReason
}

func (g *fakeGate) ResolveAsk(_ context.Context, q hooks.ApprovalQuery) (bool, string) {
	g.askQueries = append(g.askQueries, q)
	return g.askAllow, g.askReason
}

func preToolUseEvent() hooks.Event {
	return hooks.Event{
		EventID:   "evt-1",
		SessionID: "sess-1",
		ToolName:  "browser",
		ToolInput: map[string]any{"action": "navigate", "url": "https://example.com"},
		HookEvent: hooks.EventPreToolUse,
	}
}

func withTestGate(t *testing.T, g hooks.ApprovalGate) {
	t.Helper()
	hooks.SetApprovalGate(g)
	t.Cleanup(func() { hooks.SetApprovalGate(nil) })
}

func TestApprovalGate_PolicyDeny_BlocksAfterHookAllow(t *testing.T) {
	gate := &fakeGate{gateAllow: false, gateReason: "denied by policy"}
	withTestGate(t, gate)

	allowHook := newBaseHook(hooks.HandlerHTTP, hooks.EventPreToolUse)
	fs := &fakeStore{hooks: []hooks.HookConfig{allowHook}}
	d := hooks.NewStdDispatcher(hooks.StdDispatcherOpts{
		Store: fs,
		Audit: hooks.NewAuditWriter(fs, ""),
		Handlers: map[hooks.HandlerType]hooks.Handler{
			hooks.HandlerHTTP: &fakeHandler{decision: hooks.DecisionAllow},
		},
	})

	r, err := d.Fire(context.Background(), preToolUseEvent())
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if r.Decision != hooks.DecisionBlock {
		t.Fatalf("decision=%q, want block", r.Decision)
	}
	if r.DecisionReason != "denied by policy" {
		t.Errorf("reason=%q, want hook policy reason", r.DecisionReason)
	}
	if len(gate.gateQueries) != 1 {
		t.Fatalf("gate consulted %d times, want 1", len(gate.gateQueries))
	}
	q := gate.gateQueries[0]
	if q.ToolName != "browser" || q.SessionKey != "sess-1" {
		t.Errorf("query = %+v, want tool browser / session sess-1", q)
	}
	if q.ArgsDigest == "" {
		t.Error("ArgsDigest empty, want canonical hash")
	}
}

func TestApprovalGate_PolicyAllow_Proceeds(t *testing.T) {
	gate := &fakeGate{gateAllow: true}
	withTestGate(t, gate)

	fs := &fakeStore{}
	d := hooks.NewStdDispatcher(hooks.StdDispatcherOpts{Store: fs})

	r, err := d.Fire(context.Background(), preToolUseEvent())
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if r.Decision != hooks.DecisionAllow {
		t.Fatalf("decision=%q, want allow", r.Decision)
	}
	if len(gate.gateQueries) != 1 {
		t.Fatalf("gate consulted %d times, want 1", len(gate.gateQueries))
	}
}

func TestApprovalGate_NotConsultedForNonToolEvents(t *testing.T) {
	gate := &fakeGate{gateAllow: false, gateReason: "must not be asked"}
	withTestGate(t, gate)

	fs := &fakeStore{}
	d := hooks.NewStdDispatcher(hooks.StdDispatcherOpts{Store: fs})

	r, err := d.Fire(context.Background(), hooks.Event{
		EventID:   "evt-2",
		HookEvent: hooks.EventUserPromptSubmit,
		RawInput:  "hello",
	})
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if r.Decision != hooks.DecisionAllow {
		t.Fatalf("decision=%q, want allow", r.Decision)
	}
	if len(gate.gateQueries) != 0 {
		t.Fatalf("gate consulted %d times for non-tool event, want 0", len(gate.gateQueries))
	}
}

func TestHookAsk_WithGate_CreatesApprovalAndAllows(t *testing.T) {
	// gateAllow=true: after the hook's ask is approved, the end-of-chain
	// policy gate must also allow (it sees the same fake gate).
	gate := &fakeGate{gateAllow: true, askAllow: true}
	withTestGate(t, gate)

	askHook := newBaseHook(hooks.HandlerHTTP, hooks.EventPreToolUse)
	fs := &fakeStore{hooks: []hooks.HookConfig{askHook}}
	d := hooks.NewStdDispatcher(hooks.StdDispatcherOpts{
		Store: fs,
		Audit: hooks.NewAuditWriter(fs, ""),
		Handlers: map[hooks.HandlerType]hooks.Handler{
			hooks.HandlerHTTP: &fakeHandler{decision: hooks.DecisionAsk},
		},
	})

	r, err := d.Fire(context.Background(), preToolUseEvent())
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if r.Decision != hooks.DecisionAllow {
		t.Fatalf("decision=%q, want allow after human approval", r.Decision)
	}
	if len(gate.askQueries) != 1 {
		t.Fatalf("ResolveAsk called %d times, want 1", len(gate.askQueries))
	}
	q := gate.askQueries[0]
	if q.ToolName != "browser" || q.ArgsDigest == "" || q.SessionKey != "sess-1" {
		t.Errorf("ask query = %+v, want browser/digest/sess-1", q)
	}
}

func TestHookAsk_WithGate_DenyBlocks(t *testing.T) {
	gate := &fakeGate{askAllow: false, askReason: "denied by user"}
	withTestGate(t, gate)

	askHook := newBaseHook(hooks.HandlerHTTP, hooks.EventPreToolUse)
	fs := &fakeStore{hooks: []hooks.HookConfig{askHook}}
	d := hooks.NewStdDispatcher(hooks.StdDispatcherOpts{
		Store: fs,
		Audit: hooks.NewAuditWriter(fs, ""),
		Handlers: map[hooks.HandlerType]hooks.Handler{
			hooks.HandlerHTTP: &fakeHandler{decision: hooks.DecisionAsk},
		},
	})

	r, err := d.Fire(context.Background(), preToolUseEvent())
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if r.Decision != hooks.DecisionBlock {
		t.Fatalf("decision=%q, want block", r.Decision)
	}
	if r.DecisionReason != "denied by user" {
		t.Errorf("reason=%q, want deny reason", r.DecisionReason)
	}
}

func TestHookAsk_WithoutGate_DegradesToBlock(t *testing.T) {
	// No gate wired: ask must degrade to block (Wave 1 behavior).
	askHook := newBaseHook(hooks.HandlerHTTP, hooks.EventPreToolUse)
	fs := &fakeStore{hooks: []hooks.HookConfig{askHook}}
	d := hooks.NewStdDispatcher(hooks.StdDispatcherOpts{
		Store: fs,
		Audit: hooks.NewAuditWriter(fs, ""),
		Handlers: map[hooks.HandlerType]hooks.Handler{
			hooks.HandlerHTTP: &fakeHandler{decision: hooks.DecisionAsk},
		},
	})

	r, err := d.Fire(context.Background(), preToolUseEvent())
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if r.Decision != hooks.DecisionBlock {
		t.Fatalf("decision=%q, want block (legacy degrade)", r.Decision)
	}
}

func TestHookAsk_NonToolEvent_DegradesToBlockEvenWithGate(t *testing.T) {
	gate := &fakeGate{askAllow: true}
	withTestGate(t, gate)

	askHook := newBaseHook(hooks.HandlerHTTP, hooks.EventUserPromptSubmit)
	fs := &fakeStore{hooks: []hooks.HookConfig{askHook}}
	d := hooks.NewStdDispatcher(hooks.StdDispatcherOpts{
		Store: fs,
		Audit: hooks.NewAuditWriter(fs, ""),
		Handlers: map[hooks.HandlerType]hooks.Handler{
			hooks.HandlerHTTP: &fakeHandler{decision: hooks.DecisionAsk},
		},
	})

	r, err := d.Fire(context.Background(), hooks.Event{
		EventID:   "evt-3",
		HookEvent: hooks.EventUserPromptSubmit,
		RawInput:  "hello",
	})
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if r.Decision != hooks.DecisionBlock {
		t.Fatalf("decision=%q, want block for non-tool ask", r.Decision)
	}
	if len(gate.askQueries) != 0 {
		t.Fatalf("ResolveAsk called %d times for non-tool event, want 0", len(gate.askQueries))
	}
}

func TestApprovalGate_AskApprovalSkipsChainBudgetCheck(t *testing.T) {
	// A human approval wait legitimately outlives the chain budget; the
	// approved call must NOT be vetoed by the post-hook budget check.
	gate := &fakeGate{gateAllow: true, askAllow: true}
	withTestGate(t, gate)

	askHook := newBaseHook(hooks.HandlerHTTP, hooks.EventPreToolUse)
	fs := &fakeStore{hooks: []hooks.HookConfig{askHook}}
	d := hooks.NewStdDispatcher(hooks.StdDispatcherOpts{
		Store:       fs,
		Audit:       hooks.NewAuditWriter(fs, ""),
		ChainBudget: 1, // 1ns: expired by the time the ask resolves
		Handlers: map[hooks.HandlerType]hooks.Handler{
			hooks.HandlerHTTP: &fakeHandler{decision: hooks.DecisionAsk},
		},
	})

	r, err := d.Fire(context.Background(), preToolUseEvent())
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if r.Decision != hooks.DecisionAllow {
		t.Fatalf("decision=%q reason=%q, want allow after approval despite chain budget", r.Decision, r.DecisionReason)
	}
}
