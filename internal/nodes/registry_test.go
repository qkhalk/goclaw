package nodes

import (
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// fakeSender records events; optionally delivers a correlated result.
type fakeSender struct {
	events   []protocol.EventFrame
	onEvent  func(f *fakeSender, ev protocol.EventFrame)
	closed   bool
	isCloser bool // when true, implements Close()
}

func (f *fakeSender) SendEvent(ev protocol.EventFrame) {
	f.events = append(f.events, ev)
	if f.onEvent != nil {
		f.onEvent(f, ev)
	}
}

// Close implements Closeable — the type assertion in Disconnect keys on the
// method set, so keep it on the concrete type.
func (f *fakeSender) Close() { f.closed = true }

func TestRegistrySetGetOnline(t *testing.T) {
	r := NewRegistry()
	if r.Online("n1") {
		t.Fatal("unknown node should be offline")
	}
	s := &fakeSender{}
	r.Set("n1", s, "tenant-1")
	if !r.Online("n1") {
		t.Fatal("registered node should be online")
	}
	got, ok := r.Get("n1")
	if !ok || got != Sender(s) {
		t.Fatalf("Get returned (%T, %v)", got, ok)
	}
	if gotTenant := r.TenantID("n1"); gotTenant != "tenant-1" {
		t.Fatalf("TenantID = %q, want tenant-1", gotTenant)
	}
}

func TestRegistryTTLLazyExpiry(t *testing.T) {
	r := NewRegistry()
	r.onlineTTL = 20 * time.Millisecond
	s := &fakeSender{}
	r.Set("n1", s, "")
	if !r.Online("n1") {
		t.Fatal("fresh entry should be online")
	}
	time.Sleep(40 * time.Millisecond)
	if r.Online("n1") {
		t.Fatal("stale entry should read offline")
	}
	if _, ok := r.Get("n1"); ok {
		t.Fatal("stale Get should miss")
	}
	// A refreshed heartbeat revives it.
	r.Set("n1", s, "")
	if !r.Online("n1") {
		t.Fatal("refreshed entry should be online")
	}
}

func TestRegistryRemoveStaleSenderProtection(t *testing.T) {
	r := NewRegistry()
	old := &fakeSender{}
	newer := &fakeSender{}
	r.Set("n1", old, "")
	r.Set("n1", newer, "") // reconnect replaces the entry

	// A late disconnect for the OLD sender must not evict the newer one.
	if _, removed := r.Remove("n1", old); removed {
		t.Fatal("Remove with stale sender should be a no-op")
	}
	if !r.Online("n1") {
		t.Fatal("newer connection should stay online")
	}
	if _, removed := r.Remove("n1", newer); !removed {
		t.Fatal("Remove with the current sender should evict")
	}
	if r.Online("n1") {
		t.Fatal("node should be offline after eviction")
	}
}

func TestRegistryDisconnectClosesSender(t *testing.T) {
	r := NewRegistry()
	s := &fakeSender{isCloser: true}
	r.Set("n1", s, "")
	r.Disconnect("n1")
	if !s.closed {
		t.Fatal("Disconnect should close a Closeable sender")
	}
	if r.Online("n1") {
		t.Fatal("node should be offline after Disconnect")
	}
	// Unknown / non-closeable senders must not panic.
	r.Disconnect("unknown")
	r.Set("n2", &fakeSender{}, "")
	r.Disconnect("n2")
}

func TestRegistryDeliverResult(t *testing.T) {
	r := NewRegistry()
	if r.DeliverResult(&InvokeResult{InvokeID: "nope"}) {
		t.Fatal("unmatched delivery should return false")
	}
	ch := r.registerWaiter("inv-1")
	if !r.DeliverResult(&InvokeResult{InvokeID: "inv-1", ExitCode: 0}) {
		t.Fatal("matched delivery should return true")
	}
	select {
	case res := <-ch:
		if res.InvokeID != "inv-1" {
			t.Fatalf("got invoke id %q", res.InvokeID)
		}
	default:
		t.Fatal("waiter never received the result")
	}
	// Waiter is consumed after first delivery (registration is one-shot).
	if _, ok := r.resolveWaiter("inv-1"); ok {
		t.Fatal("waiter should be removed after delivery")
	}
	if r.DeliverResult(nil) {
		t.Fatal("nil result should not be delivered")
	}
}
