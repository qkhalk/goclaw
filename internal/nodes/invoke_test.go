package nodes

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// fakeLookup serves nodes from a map.
type fakeLookup struct {
	nodes map[string]*store.Node
}

func (f *fakeLookup) GetByID(_ context.Context, id string) (*store.Node, error) {
	if n, ok := f.nodes[id]; ok {
		return n, nil
	}
	return nil, sql.ErrNoRows
}

func trustedNode(id string) *store.Node {
	tenant := store.MasterTenantID.String()
	return &store.Node{
		ID:           id,
		TenantID:     &tenant,
		Name:         "node-" + id[:8],
		Trust:        store.NodeTrustTrusted,
		Capabilities: []string{store.NodeCapabilityExec},
	}
}

func TestInvokeOffline(t *testing.T) {
	reg := NewRegistry()
	id := uuid.NewString()
	lookup := &fakeLookup{nodes: map[string]*store.Node{id: trustedNode(id)}}
	// Trusted + capable, but no live connection → immediate typed error.
	_, err := Invoke(context.Background(), reg, lookup, id, InvokeRequest{Command: "ls", Timeout: 50 * time.Millisecond})
	if !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("err = %v, want ErrNodeOffline", err)
	}
}

func TestInvokeUnknownNode(t *testing.T) {
	reg := NewRegistry()
	lookup := &fakeLookup{nodes: map[string]*store.Node{}}
	_, err := Invoke(context.Background(), reg, lookup, uuid.NewString(), InvokeRequest{Command: "ls"})
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("err = %v, want ErrNodeNotFound", err)
	}
	_, err = Invoke(context.Background(), nil, lookup, uuid.NewString(), InvokeRequest{Command: "ls"})
	if !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("nil registry err = %v, want ErrNodeOffline", err)
	}
	_, err = Invoke(context.Background(), reg, nil, uuid.NewString(), InvokeRequest{Command: "ls"})
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("nil lookup err = %v, want ErrNodeNotFound", err)
	}
}

func TestInvokeUntrusted(t *testing.T) {
	reg := NewRegistry()
	id := uuid.NewString()
	node := trustedNode(id)
	node.Trust = store.NodeTrustPending
	reg.Set(id, &fakeSender{}, "")
	lookup := &fakeLookup{nodes: map[string]*store.Node{id: node}}
	_, err := Invoke(context.Background(), reg, lookup, id, InvokeRequest{Command: "ls"})
	if !errors.Is(err, ErrNodeUntrusted) {
		t.Fatalf("err = %v, want ErrNodeUntrusted", err)
	}
}

func TestInvokeMissingExecCapability(t *testing.T) {
	reg := NewRegistry()
	id := uuid.NewString()
	node := trustedNode(id)
	node.Capabilities = []string{store.NodeCapabilityFS}
	reg.Set(id, &fakeSender{}, "")
	lookup := &fakeLookup{nodes: map[string]*store.Node{id: node}}
	_, err := Invoke(context.Background(), reg, lookup, id, InvokeRequest{Command: "ls"})
	if !errors.Is(err, ErrNodeNoExecCap) {
		t.Fatalf("err = %v, want ErrNodeNoExecCap", err)
	}
}

func TestInvokeCrossTenantReadsAsNotFound(t *testing.T) {
	reg := NewRegistry()
	id := uuid.NewString()
	node := trustedNode(id)
	otherTenant := uuid.Must(uuid.NewV7())
	reg.Set(id, &fakeSender{}, "")
	lookup := &fakeLookup{nodes: map[string]*store.Node{id: node}}
	ctx := store.WithTenantID(context.Background(), otherTenant)
	_, err := Invoke(ctx, reg, lookup, id, InvokeRequest{Command: "ls"})
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("err = %v, want ErrNodeNotFound (cross-tenant must not enumerate)", err)
	}
}

func TestInvokeHappyPath(t *testing.T) {
	reg := NewRegistry()
	id := uuid.NewString()
	sender := &fakeSender{
		onEvent: func(f *fakeSender, ev protocol.EventFrame) {
			payload, ok := ev.Payload.(map[string]any)
			if !ok {
				t.Errorf("payload type = %T", ev.Payload)
				return
			}
			invokeID, _ := payload["invokeId"].(string)
			reg.DeliverResult(&InvokeResult{InvokeID: invokeID, ExitCode: 0, Stdout: "out"})
		},
	}
	reg.Set(id, sender, "")
	lookup := &fakeLookup{nodes: map[string]*store.Node{id: trustedNode(id)}}

	res, err := Invoke(context.Background(), reg, lookup, id, InvokeRequest{Command: "ls", Args: []string{"-la"}, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res == nil || res.Stdout != "out" {
		t.Fatalf("res = %+v", res)
	}
	if len(sender.events) != 1 || sender.events[0].Event != protocol.EventNodeInvoke {
		t.Fatalf("events = %+v, want one node.invoke", sender.events)
	}
}

func TestInvokeTimeout(t *testing.T) {
	reg := NewRegistry()
	id := uuid.NewString()
	reg.Set(id, &fakeSender{}, "") // silent daemon
	lookup := &fakeLookup{nodes: map[string]*store.Node{id: trustedNode(id)}}

	start := time.Now()
	_, err := Invoke(context.Background(), reg, lookup, id, InvokeRequest{Command: "sleep", Timeout: 40 * time.Millisecond})
	if !errors.Is(err, ErrInvokeTimeout) {
		t.Fatalf("err = %v, want ErrInvokeTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout path took %s — must be bounded", elapsed)
	}
}

func TestInvokeContextCancelled(t *testing.T) {
	reg := NewRegistry()
	id := uuid.NewString()
	reg.Set(id, &fakeSender{}, "")
	lookup := &fakeLookup{nodes: map[string]*store.Node{id: trustedNode(id)}}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := Invoke(ctx, reg, lookup, id, InvokeRequest{Command: "sleep", Timeout: 10 * time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestInvokeWaiterCleanedUpAfterTimeout(t *testing.T) {
	reg := NewRegistry()
	id := uuid.NewString()
	reg.Set(id, &fakeSender{}, "")
	lookup := &fakeLookup{nodes: map[string]*store.Node{id: trustedNode(id)}}

	if _, err := Invoke(context.Background(), reg, lookup, id, InvokeRequest{Command: "x", Timeout: 30 * time.Millisecond}); !errors.Is(err, ErrInvokeTimeout) {
		t.Fatalf("err = %v", err)
	}
	// After the timeout the waiter must be gone: a late daemon result for the
	// dead correlation id must not be buffered for anyone.
	reg.pendingMu.Lock()
	remaining := len(reg.pending)
	reg.pendingMu.Unlock()
	if remaining != 0 {
		t.Fatalf("%d waiters leaked after timeout", remaining)
	}
}

func TestTruncateResult(t *testing.T) {
	big := make([]byte, MaxResultBytes+100)
	for i := range big {
		big[i] = 'x'
	}
	res := TruncateResult(&InvokeResult{Stdout: string(big), Stderr: "small"})
	if len(res.Stdout) != MaxResultBytes {
		t.Fatalf("stdout len = %d, want %d", len(res.Stdout), MaxResultBytes)
	}
	if res.Stderr != "small" {
		t.Fatalf("stderr = %q", res.Stderr)
	}
	if TruncateResult(nil) != nil {
		t.Fatal("TruncateResult(nil) should stay nil")
	}
}
