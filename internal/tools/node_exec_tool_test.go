package tools

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/nodes"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// fakeNodeSender delivers a canned result for every node.invoke it receives.
type fakeNodeSender struct {
	events []protocol.EventFrame
	result *nodes.InvokeResult
	reg    *nodes.Registry
}

func (f *fakeNodeSender) SendEvent(ev protocol.EventFrame) {
	f.events = append(f.events, ev)
	if f.result != nil && f.reg != nil {
		payload, _ := ev.Payload.(map[string]any)
		id, _ := payload["invokeId"].(string)
		delivered := *f.result
		delivered.InvokeID = id
		f.reg.DeliverResult(&delivered)
	}
}

// stubExecNodeStore implements the store.NodeStore surface the tool uses.
type stubExecNodeStore struct {
	node *store.Node
}

func (s *stubExecNodeStore) Create(_ context.Context, n *store.Node) error { return nil }

func (s *stubExecNodeStore) GetByID(_ context.Context, id string) (*store.Node, error) {
	if s.node != nil && s.node.ID == id {
		return s.node, nil
	}
	return nil, errLookupMiss{}
}

func (s *stubExecNodeStore) GetByKeyHash(_ context.Context, hash string) (*store.Node, error) {
	return nil, errLookupMiss{}
}

func (s *stubExecNodeStore) List(_ context.Context) ([]*store.Node, error) {
	if s.node == nil {
		return nil, nil
	}
	return []*store.Node{s.node}, nil
}

func (s *stubExecNodeStore) UpdateRegistration(_ context.Context, id, name, platform string, capabilities []string) error {
	return nil
}

func (s *stubExecNodeStore) TouchSeen(_ context.Context, id string) error { return nil }

func (s *stubExecNodeStore) SetTrust(_ context.Context, id, trust string) error { return nil }

func (s *stubExecNodeStore) Revoke(_ context.Context, id string) error { return nil }

type errLookupMiss struct{}

func (errLookupMiss) Error() string { return "no rows" }

// captureBus records broadcast events for audit assertions.
type captureBus struct {
	events []bus.Event
}

func (c *captureBus) Subscribe(string, bus.EventHandler) {}
func (c *captureBus) Unsubscribe(string)                 {}
func (c *captureBus) Broadcast(e bus.Event)              { c.events = append(c.events, e) }

func newTrustedExecNode() *store.Node {
	tenant := store.MasterTenantID.String()
	return &store.Node{
		ID:           uuid.NewString(),
		TenantID:     &tenant,
		Name:         "build-1",
		Trust:        store.NodeTrustTrusted,
		Capabilities: []string{store.NodeCapabilityExec},
	}
}

func TestNodeExecToolRequiresCommand(t *testing.T) {
	tool := NewNodeExecTool(&stubExecNodeStore{}, nodes.NewRegistry(), nil)
	res := tool.Execute(context.Background(), map[string]any{"node": "build-1"})
	if !res.IsError || res.ForLLM == "" {
		t.Fatalf("res = %+v, want error for missing command", res)
	}
}

func TestNodeExecToolRequiresNode(t *testing.T) {
	tool := NewNodeExecTool(&stubExecNodeStore{}, nodes.NewRegistry(), nil)
	res := tool.Execute(context.Background(), map[string]any{"command": "ls"})
	if !res.IsError {
		t.Fatalf("res = %+v, want error for missing node", res)
	}
}

func TestNodeExecToolUnknownNode(t *testing.T) {
	tool := NewNodeExecTool(&stubExecNodeStore{}, nodes.NewRegistry(), nil)
	res := tool.Execute(context.Background(), map[string]any{"node": "nope", "command": "ls"})
	if !res.IsError || res.ForLLM == "" {
		t.Fatalf("res = %+v, want error for unknown node", res)
	}
}

func TestNodeExecToolUntrustedDenied(t *testing.T) {
	node := newTrustedExecNode()
	node.Trust = store.NodeTrustPending
	reg := nodes.NewRegistry()
	reg.Set(node.ID, &fakeNodeSender{}, "")
	tool := NewNodeExecTool(&stubExecNodeStore{node: node}, reg, nil)
	res := tool.Execute(context.Background(), map[string]any{"node": "build-1", "command": "ls"})
	if !res.IsError {
		t.Fatalf("res = %+v, want error for untrusted node", res)
	}
}

func TestNodeExecToolOfflineImmediate(t *testing.T) {
	node := newTrustedExecNode()
	tool := NewNodeExecTool(&stubExecNodeStore{node: node}, nodes.NewRegistry(), nil)
	start := time.Now()
	res := tool.Execute(context.Background(), map[string]any{"node": "build-1", "command": "ls"})
	if !res.IsError {
		t.Fatalf("res = %+v, want error for offline node", res)
	}
	if time.Since(start) > time.Second {
		t.Fatal("offline path must be immediate")
	}
}

func TestNodeExecToolHappyPathAudits(t *testing.T) {
	node := newTrustedExecNode()
	reg := nodes.NewRegistry()
	sender := &fakeNodeSender{
		reg:    reg,
		result: &nodes.InvokeResult{ExitCode: 0, Stdout: "file.txt"},
	}
	reg.Set(node.ID, sender, "")
	audit := &captureBus{}
	tool := NewNodeExecTool(&stubExecNodeStore{node: node}, reg, audit)

	res := tool.Execute(context.Background(), map[string]any{
		"node":    "build-1",
		"command": "ls",
		"args":    []any{"-1"},
	})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.ForLLM)
	}
	if len(sender.events) != 1 || sender.events[0].Event != protocol.EventNodeInvoke {
		t.Fatalf("events = %+v, want one node.invoke", sender.events)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(audit.events))
	}
	if audit.events[0].Name != protocol.EventAuditLog {
		t.Fatalf("audit event = %s", audit.events[0].Name)
	}
}

func TestNodeExecToolNonZeroExitIsError(t *testing.T) {
	node := newTrustedExecNode()
	reg := nodes.NewRegistry()
	sender := &fakeNodeSender{
		reg:    reg,
		result: &nodes.InvokeResult{ExitCode: 2, Stderr: "boom"},
	}
	reg.Set(node.ID, sender, "")
	tool := NewNodeExecTool(&stubExecNodeStore{node: node}, reg, nil)

	res := tool.Execute(context.Background(), map[string]any{"node": "build-1", "command": "ls"})
	if !res.IsError {
		t.Fatalf("res = %+v, want error for non-zero exit", res)
	}
}

func TestNodeExecToolTimeoutBounded(t *testing.T) {
	node := newTrustedExecNode()
	reg := nodes.NewRegistry()
	reg.Set(node.ID, &fakeNodeSender{}, "") // silent
	tool := NewNodeExecTool(&stubExecNodeStore{node: node}, reg, nil)

	start := time.Now()
	res := tool.Execute(context.Background(), map[string]any{
		"node": "build-1", "command": "sleep", "timeout_sec": float64(0.05),
	})
	if !res.IsError {
		t.Fatalf("res = %+v, want timeout error", res)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("tool timeout must be bounded (timeout_sec coercion)")
	}
}

func TestNodeExecToolCrossTenantHidden(t *testing.T) {
	node := newTrustedExecNode()
	other := uuid.Must(uuid.NewV7()).String()
	node.TenantID = &other
	reg := nodes.NewRegistry()
	reg.Set(node.ID, &fakeNodeSender{}, "")
	tool := NewNodeExecTool(&stubExecNodeStore{node: node}, reg, nil)

	ctx := store.WithTenantID(context.Background(), store.MasterTenantID)
	res := tool.Execute(ctx, map[string]any{"node": node.ID, "command": "ls"})
	if !res.IsError {
		t.Fatalf("res = %+v, want not-found style error across tenants", res)
	}
}
