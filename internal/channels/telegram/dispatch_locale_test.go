package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// dispatchLocaleHarness wires a minimal Channel whose bus captures the
// PublishedInbound produced by dispatchResolvedMessage.
func dispatchLocaleHarness(t *testing.T) (*Channel, *bus.MessageBus) {
	t.Helper()
	caller := &recordingTelegramCaller{}
	bot, err := telego.NewBot(
		"123456:abcdefghijklmnopqrstuvwxyzABCDE1234",
		telego.WithAPICaller(caller),
		telego.WithDiscardLogger(),
	)
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	msgBus := bus.New()
	ch := &Channel{
		BaseChannel: channels.NewBaseChannel("telegram", msgBus, nil),
		bot:         bot,
	}
	ch.SetRunning(true)
	return ch, msgBus
}

func consumeInbound(t *testing.T, msgBus *bus.MessageBus) bus.InboundMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg, ok := msgBus.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("no inbound message published")
	}
	return msg
}

func TestDispatchResolvedMessagePropagatesSenderLocale(t *testing.T) {
	ch, msgBus := dispatchLocaleHarness(t)

	msg := &telego.Message{
		MessageID: 7,
		Chat:      telego.Chat{ID: 123, Type: telego.ChatTypePrivate},
		From:      &telego.User{ID: 42, FirstName: "Khanh", LanguageCode: "vi"},
		Text:      "chào em",
	}
	rctx := resolvedMessageContext{
		content:     "chào em",
		senderID:    "42",
		senderLabel: "Khanh",
		chatID:      123,
		chatIDStr:   "123",
		localKey:    "telegram:123:42",
	}

	ch.dispatchResolvedMessage(context.Background(), rctx, []*telego.Message{msg})

	got := consumeInbound(t, msgBus)
	if loc := got.Metadata[tools.MetaUserLocale]; loc != "vi" {
		t.Errorf("user_locale metadata: got %q, want vi", loc)
	}
}

func TestDispatchResolvedMessageChannelPostHasNoLocale(t *testing.T) {
	ch, msgBus := dispatchLocaleHarness(t)

	// Channel posts carry no From — resolveMessageSender synthesizes a sender
	// without LanguageCode, so no locale may be pinned.
	msg := &telego.Message{
		MessageID: 8,
		Chat:      telego.Chat{ID: 500, Type: telego.ChatTypeChannel, Title: "News"},
		Text:      "broadcast",
	}
	rctx := resolvedMessageContext{
		content:     "broadcast",
		senderID:    "500",
		senderLabel: "News",
		chatID:      500,
		chatIDStr:   "500",
		localKey:    "telegram:500:500",
	}

	ch.dispatchResolvedMessage(context.Background(), rctx, []*telego.Message{msg})

	got := consumeInbound(t, msgBus)
	if loc, ok := got.Metadata[tools.MetaUserLocale]; ok {
		t.Errorf("user_locale metadata: got %q, want absent for channel post", loc)
	}
}
