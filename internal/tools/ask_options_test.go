package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

func TestAskOptionsTool_Validation(t *testing.T) {
	tool := NewAskOptionsTool()

	cases := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{"missing question", map[string]any{"options": []any{"a"}}, "question is required"},
		{"empty options", map[string]any{"question": "Q?", "options": []any{}}, "1-4 choices"},
		{"too many options", map[string]any{"question": "Q?", "options": []any{"a", "b", "c", "d", "e"}}, "too many"},
		{"empty label", map[string]any{"question": "Q?", "options": []any{"  "}}, "empty labels"},
		{"label too long", map[string]any{"question": "Q?", "options": []any{strings.Repeat("x", 49)}}, "too long"},
		{"duplicate", map[string]any{"question": "Q?", "options": []any{"a", "a"}}, "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := tool.Execute(context.Background(), tc.args)
			if !res.IsError || !strings.Contains(res.ForLLM, tc.wantErr) {
				t.Errorf("result = %+v, want error containing %q", res, tc.wantErr)
			}
		})
	}
}

func TestAskOptionsTool_RejectsInternalContext(t *testing.T) {
	tool := NewAskOptionsTool()
	tool.SetMessageBus(bus.New())

	ctx := WithToolChannel(WithToolChatID(context.Background(), "42"), ChannelSystem)
	res := tool.Execute(ctx, map[string]any{"question": "Q?", "options": []any{"a"}})
	if !res.IsError || !strings.Contains(res.ForLLM, "no channel/chat") {
		t.Errorf("result = %+v, want internal-channel rejection", res)
	}
}

func TestAskOptionsTool_RejectsNonTelegramChannel(t *testing.T) {
	tool := NewAskOptionsTool()
	tool.SetMessageBus(bus.New())

	ctx := WithToolChannel(WithToolChatID(context.Background(), "42"), "zalo")
	res := tool.Execute(ctx, map[string]any{"question": "Q?", "options": []any{"a"}})
	if !res.IsError || !strings.Contains(res.ForLLM, "Telegram only") {
		t.Errorf("result = %+v, want non-telegram rejection", res)
	}
}

func TestAskOptionsTool_PublishesOutboundWithKeyboardMetadata(t *testing.T) {
	mb := bus.New()
	tool := NewAskOptionsTool()
	tool.SetMessageBus(mb)

	ctx := WithToolLocalKey(WithToolChannel(WithToolChatID(context.Background(), "-100"), "telegram"), "-100:topic:42")
	res := tool.Execute(ctx, map[string]any{
		"question": "Which DB?",
		"options":  []any{"Postgres", "MySQL"},
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if !strings.Contains(res.ForLLM, "End your turn") {
		t.Errorf("result = %q, want end-turn instruction", res.ForLLM)
	}

	outCh := make(chan bus.OutboundMessage, 1)
	go func() {
		if msg, ok := mb.SubscribeOutbound(context.Background()); ok {
			outCh <- msg
		}
	}()
	select {
	case msg := <-outCh:
		if msg.Channel != "telegram" {
			t.Errorf("channel = %q", msg.Channel)
		}
		// Composite local key wins over the raw chat ID so the channel Send
		// path resolves forum-topic routing + the placeholder key.
		if msg.ChatID != "-100:topic:42" {
			t.Errorf("chatID = %q, want local key", msg.ChatID)
		}
		if msg.Metadata["local_key"] != "-100:topic:42" {
			t.Errorf("local_key metadata = %q", msg.Metadata["local_key"])
		}
		if msg.Metadata["message_thread_id"] != "42" {
			t.Errorf("message_thread_id metadata = %q", msg.Metadata["message_thread_id"])
		}
		if !strings.Contains(msg.Content, "Which DB?") {
			t.Errorf("content = %q", msg.Content)
		}
		raw := msg.Metadata[MetaAskOptions]
		if !strings.Contains(raw, "Postgres") || !strings.Contains(raw, "MySQL") {
			t.Errorf("metadata[%s] = %q", MetaAskOptions, raw)
		}
	case <-time.After(time.Second):
		t.Fatalf("no outbound published")
	}
}

func TestAskOptionsTool_NoBus(t *testing.T) {
	tool := NewAskOptionsTool()
	ctx := WithToolChannel(WithToolChatID(context.Background(), "42"), "telegram")
	res := tool.Execute(ctx, map[string]any{"question": "Q?", "options": []any{"a"}})
	if !res.IsError || !strings.Contains(res.ForLLM, "bus unavailable") {
		t.Errorf("result = %+v, want bus-unavailable error", res)
	}
}
