package channels

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func TestHandleAgentEvent_QuickAckNonStreamingOnly(t *testing.T) {
	behavior := ResolvedChatBehavior{
		Enabled: true,
		QuickAck: ResolvedQuickAckConfig{
			Enabled:    true,
			Mode:       QuickAckModeFixedTemplate,
			MinDelayMs: 0,
			Templates:  []string{"On it."},
		},
	}

	mb := bus.New()
	mgr := NewManager(mb)
	mgr.RegisterChannel("test", &chatBehaviorTestChannel{name: "test"})
	mgr.RegisterRunWithBehavior("run-1", "test", "chat-1", "msg-1", map[string]string{"local_key": "chat-1/topic"}, uuid.Nil, false, false, true, behavior)

	mgr.HandleAgentEvent(protocol.AgentEventRunStarted, "run-1", nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, ok := mb.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("expected quick acknowledgement outbound message")
	}
	if got.Content != "On it." || got.ChatID != "chat-1" || got.Metadata["local_key"] != "chat-1/topic" {
		t.Fatalf("quick ack outbound = %+v, want content and routing metadata", got)
	}

	mb = bus.New()
	mgr = NewManager(mb)
	mgr.RegisterChannel("test", &chatBehaviorTestChannel{name: "test"})
	mgr.RegisterRunWithBehavior("run-2", "test", "chat-1", "msg-1", nil, uuid.Nil, true, false, true, behavior)

	mgr.HandleAgentEvent(protocol.AgentEventRunStarted, "run-2", nil)

	ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if got, ok := mb.SubscribeOutbound(ctx); ok {
		t.Fatalf("streaming run emitted quick ack: %+v", got)
	}
}

func TestUnregisterRun_CancelsPendingQuickAck(t *testing.T) {
	mb := bus.New()
	mgr := NewManager(mb)
	mgr.RegisterChannel("test", &chatBehaviorTestChannel{name: "test"})
	mgr.RegisterRunWithBehavior("run-1", "test", "chat-1", "msg-1", nil, uuid.Nil, false, false, true, ResolvedChatBehavior{
		Enabled: true,
		QuickAck: ResolvedQuickAckConfig{
			Enabled:    true,
			Mode:       QuickAckModeFixedTemplate,
			MinDelayMs: 50,
			Templates:  []string{"On it."},
		},
	})

	mgr.HandleAgentEvent(protocol.AgentEventRunStarted, "run-1", nil)
	mgr.UnregisterRun("run-1")

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	if got, ok := mb.SubscribeOutbound(ctx); ok {
		t.Fatalf("unregistered run emitted quick ack: %+v", got)
	}
}

func TestCancelQuickAck_BlocksInFlightSend(t *testing.T) {
	mb := bus.New()
	mgr := NewManager(mb)
	rc := &RunContext{
		ChannelName: "test",
		ChatID:      "chat-1",
		ChatBehavior: ResolvedChatBehavior{
			Enabled: true,
			QuickAck: ResolvedQuickAckConfig{
				Enabled:   true,
				Mode:      QuickAckModeFixedTemplate,
				Templates: []string{"On it."},
			},
		},
	}

	mgr.cancelQuickAck(rc)
	mgr.sendQuickAck(rc)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if got, ok := mb.SubscribeOutbound(ctx); ok {
		t.Fatalf("cancelled quick ack emitted message: %+v", got)
	}
}

func TestHandleAgentEvent_GeneratedProgressCancelsFallback(t *testing.T) {
	behavior := ResolvedChatBehavior{
		Enabled: true,
		QuickAck: ResolvedQuickAckConfig{
			Enabled:    true,
			Mode:       QuickAckModeLLMGenerated,
			MinDelayMs: 50,
			Templates:  []string{"Fallback."},
		},
	}

	mb := bus.New()
	mgr := NewManager(mb)
	mgr.RegisterChannel("test", &chatBehaviorTestChannel{name: "test"})
	mgr.RegisterRunWithBehavior("run-1", "test", "chat-1", "msg-1", map[string]string{"local_key": "chat-1/topic"}, uuid.Nil, false, false, true, behavior)

	mgr.HandleAgentEvent(protocol.AgentEventRunStarted, "run-1", nil)
	mgr.HandleAgentEvent(protocol.AgentEventBlockReply, "run-1", map[string]string{"content": "I will check that now."})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, ok := mb.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("expected generated progress outbound message")
	}
	if got.Content != "I will check that now." || got.Metadata["local_key"] != "chat-1/topic" {
		t.Fatalf("generated progress outbound = %+v, want generated content and routing metadata", got)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	if got, ok := mb.SubscribeOutbound(ctx); ok {
		t.Fatalf("fallback emitted after generated progress: %+v", got)
	}
}

func TestHandleAgentEvent_GeneratedModeFallsBackWithoutBlockReply(t *testing.T) {
	behavior := ResolvedChatBehavior{
		Enabled: true,
		QuickAck: ResolvedQuickAckConfig{
			Enabled:    true,
			Mode:       QuickAckModeLLMGenerated,
			MinDelayMs: 0,
			Templates:  []string{"Fallback."},
		},
	}

	mb := bus.New()
	mgr := NewManager(mb)
	mgr.RegisterChannel("test", &chatBehaviorTestChannel{name: "test"})
	mgr.RegisterRunWithBehavior("run-1", "test", "chat-1", "msg-1", nil, uuid.Nil, false, false, true, behavior)

	mgr.HandleAgentEvent(protocol.AgentEventRunStarted, "run-1", nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, ok := mb.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("expected fallback quick acknowledgement")
	}
	if got.Content != "Fallback." {
		t.Fatalf("fallback content = %q, want Fallback.", got.Content)
	}
}

func TestHandleAgentEvent_QuickAckModeOffPreservesExplicitBlockReply(t *testing.T) {
	behavior := ResolvedChatBehavior{
		Enabled: true,
		QuickAck: ResolvedQuickAckConfig{
			Enabled:   true,
			Mode:      QuickAckModeOff,
			Templates: []string{"Fallback."},
		},
	}

	mb := bus.New()
	mgr := NewManager(mb)
	mgr.RegisterChannel("test", &chatBehaviorTestChannel{name: "test"})
	mgr.RegisterRunWithBehavior("run-1", "test", "chat-1", "msg-1", nil, uuid.Nil, false, true, true, behavior)

	mgr.HandleAgentEvent(protocol.AgentEventRunStarted, "run-1", nil)
	mgr.HandleAgentEvent(protocol.AgentEventBlockReply, "run-1", map[string]string{"content": "Explicit block reply."})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, ok := mb.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("expected explicit block reply outbound message")
	}
	if got.Content != "Explicit block reply." {
		t.Fatalf("explicit block reply content = %q", got.Content)
	}
}
