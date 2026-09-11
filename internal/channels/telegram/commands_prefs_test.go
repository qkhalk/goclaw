package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- fakes ---

type fakePrefsStore struct {
	data      map[string]map[string]string
	saveCalls int
}

func newFakePrefsStore() *fakePrefsStore {
	return &fakePrefsStore{data: map[string]map[string]string{}}
}

func (f *fakePrefsStore) Get(_ context.Context, key string) *store.SessionData {
	if meta, ok := f.data[key]; ok {
		return &store.SessionData{Key: key, Metadata: meta}
	}
	return nil
}

func (f *fakePrefsStore) SetSessionMetadata(_ context.Context, key string, metadata map[string]string) {
	if f.data[key] == nil {
		f.data[key] = map[string]string{}
	}
	for k, v := range metadata {
		f.data[key][k] = v
	}
}

func (f *fakePrefsStore) Save(_ context.Context, _ string) error {
	f.saveCalls++
	return nil
}

// newPrefsTestChannel wires a Channel with a fake bot caller and stores.
func newPrefsTestChannel(t *testing.T, prefs *fakePrefsStore, provider StatusProvider) (*Channel, *recordingTelegramCaller) {
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
	// Avoid a typed-nil interface: a nil *fakePrefsStore must become a nil
	// interface so the handler's "not available" guard fires.
	var prefsStore SessionPrefsStore
	if prefs != nil {
		prefsStore = prefs
	}
	ch := &Channel{
		BaseChannel:    channels.NewBaseChannel("telegram", nil, nil),
		bot:            bot,
		sessionPrefs:   prefsStore,
		statusProvider: provider,
	}
	ch.SetName("telegram")
	ch.SetRunning(true)
	return ch, caller
}

func lastSentText(t *testing.T, c *recordingTelegramCaller) string {
	t.Helper()
	if len(c.calls) == 0 {
		t.Fatalf("no message sent")
	}
	text, _ := c.calls[len(c.calls)-1].body["text"].(string)
	return text
}

// --- chatSessionKey must mirror the gateway consumer construction ---

func TestChatSessionKey_MatchesConsumerConstruction(t *testing.T) {
	ch, _ := newPrefsTestChannel(t, nil, nil)
	ch.SetAgentID("fox")

	cases := []struct {
		name       string
		chatID     string
		isGroup    bool
		isForum    bool
		threadID   int
		dmThreadID int
		want       string
	}{
		{"dm", "386246614", false, false, 0, 0, "agent:fox:telegram:direct:386246614"},
		{"group", "-100123", true, false, 0, 0, "agent:fox:telegram:group:-100123"},
		{"forum topic", "-100123", true, true, 5, 0, "agent:fox:telegram:group:-100123:topic:5"},
		{"dm thread", "386246614", false, false, 0, 7, "agent:fox:telegram:direct:386246614:thread:7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ch.chatSessionKey(tc.chatID, tc.isGroup, tc.isForum, tc.threadID, tc.dmThreadID)
			if got != tc.want {
				t.Errorf("chatSessionKey = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- normalizeThinkingLevel ---

func TestNormalizeThinkingLevel(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"high", "high", false},
		{"OFF", "off", false},
		{"  low ", "low", false},
		{"adaptive", "adaptive", false},
		{"default", "", false},
		{"reset", "", false},
		{"none", "", true},
		{"banana", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeThinkingLevel(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeThinkingLevel(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeThinkingLevel(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeThinkingLevel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- /thinking handler persistence semantics ---

func TestThinkingCommand_SetPersistsThroughSave(t *testing.T) {
	prefs := newFakePrefsStore()
	ch, caller := newPrefsTestChannel(t, prefs, nil)

	ch.handleThinkingCommand(context.Background(), 1, "386246614", "42", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "high")

	if prefs.saveCalls == 0 {
		t.Fatalf("Save was not called after SetSessionMetadata")
	}
	got := prefs.data["agent::telegram:direct:386246614"]["thinking_level"]
	if got != "high" {
		t.Errorf("persisted thinking_level = %q, want high", got)
	}
	if text := lastSentText(t, caller); !strings.Contains(text, "high") {
		t.Errorf("reply missing level: %q", text)
	}
}

func TestThinkingCommand_NoneRejected(t *testing.T) {
	prefs := newFakePrefsStore()
	ch, _ := newPrefsTestChannel(t, prefs, nil)

	ch.handleThinkingCommand(context.Background(), 1, "386246614", "42", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "none")

	if _, exists := prefs.data["agent::telegram:direct:386246614"]; exists {
		t.Errorf("none must not persist anything")
	}
}

func TestThinkingCommand_DefaultClears(t *testing.T) {
	prefs := newFakePrefsStore()
	prefs.SetSessionMetadata(context.Background(), "agent::telegram:direct:1", map[string]string{"thinking_level": "high"})
	ch, _ := newPrefsTestChannel(t, prefs, nil)

	ch.handleThinkingCommand(context.Background(), 1, "1", "42", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "default")

	if got := prefs.data["agent::telegram:direct:1"]["thinking_level"]; got != "" {
		t.Errorf("thinking_level = %q, want cleared", got)
	}
}

func TestThinkingCommand_NoStore_RepliesUnavailable(t *testing.T) {
	ch, caller := newPrefsTestChannel(t, nil, nil)

	ch.handleThinkingCommand(context.Background(), 1, "1", "42", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "")

	if text := lastSentText(t, caller); !strings.Contains(text, "not available") {
		t.Errorf("expected availability notice, got %q", text)
	}
}

// --- /dev handler ---

func TestDevCommand_OnOffShow(t *testing.T) {
	prefs := newFakePrefsStore()
	ch, caller := newPrefsTestChannel(t, prefs, nil)
	key := "agent::telegram:direct:1"

	ch.handleDevCommand(context.Background(), 1, "1", "42", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "on")
	if got := prefs.data[key]["chat_mode"]; got != "dev" {
		t.Errorf("chat_mode = %q, want dev", got)
	}
	if prefs.saveCalls == 0 {
		t.Errorf("Save not called after /dev on")
	}

	ch.handleDevCommand(context.Background(), 1, "1", "42", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "")
	if text := lastSentText(t, caller); !strings.Contains(text, "ON") {
		t.Errorf("status reply should show ON, got %q", text)
	}

	ch.handleDevCommand(context.Background(), 1, "1", "42", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "off")
	if got := prefs.data[key]["chat_mode"]; got != "" {
		t.Errorf("chat_mode = %q, want cleared", got)
	}
}

// --- /status rendering ---

func TestStatusCommand_FullCardHasAllSections(t *testing.T) {
	prefs := newFakePrefsStore()
	provider := newFakeStatusProvider()
	ch, caller := newPrefsTestChannel(t, prefs, provider)

	ch.handleStatusCommand(context.Background(), 1, "1", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "")

	text := lastSentText(t, caller)
	for _, want := range []string{"GoClaw", "Uptime", "Agent:", "Model:", "Session:", "Cost", "Context", "Compactions", "Think:", "Docs:"} {
		if !strings.Contains(text, want) {
			t.Errorf("full status missing %q in:\n%s", want, text)
		}
	}
}

func TestStatusCommand_ShortCardTruncated(t *testing.T) {
	prefs := newFakePrefsStore()
	provider := newFakeStatusProvider()
	ch, caller := newPrefsTestChannel(t, prefs, provider)

	ch.handleStatusCommand(context.Background(), 1, "1", true, false, 0, 0,
		func(*telego.SendMessageParams) {}, "")

	text := lastSentText(t, caller)
	if strings.Contains(text, "Compactions") {
		t.Errorf("short status must not contain context/compaction lines:\n%s", text)
	}
	if !strings.Contains(text, "status full") {
		t.Errorf("short status should hint /status full:\n%s", text)
	}
}

func TestStatusCommand_VerbosityPersisted(t *testing.T) {
	prefs := newFakePrefsStore()
	provider := newFakeStatusProvider()
	ch, _ := newPrefsTestChannel(t, prefs, provider)

	ch.handleStatusCommand(context.Background(), 1, "1", true, false, 0, 0,
		func(*telego.SendMessageParams) {}, "full")

	if got := prefs.data["agent::telegram:group:1"]["tg_status_verbosity"]; got != "full" {
		t.Errorf("tg_status_verbosity = %q, want full", got)
	}
}

func newFakeStatusProvider() StatusProvider {
	return &fakeStatusProvider{}
}

type fakeStatusProvider struct{}

func (f *fakeStatusProvider) Gateway(context.Context) StatusGatewayInfo {
	return StatusGatewayInfo{
		Version:         "3.18.0",
		StartedAt:       time.Now().Add(-2 * time.Hour),
		SystemUptime:    5 * 24 * time.Hour,
		LaneName:        "main",
		LaneActive:      1,
		LaneConcurrency: 4,
		LanePending:     2,
	}
}

func (f *fakeStatusProvider) Session(context.Context, string) (StatusSessionInfo, bool) {
	return StatusSessionInfo{
		Model: "mimo-v2.5-free", Provider: "oc",
		UpdatedAt: time.Now(), InputTokens: 12300, OutputTokens: 4500,
		LastPromptTokens: 37000, ContextWindow: 200000, CompactionCount: 0,
	}, true
}

func (f *fakeStatusProvider) SessionCost(context.Context, string) (float64, bool) {
	return 0.0074, true
}

// --- formatting helpers ---

func TestHumanizeTokens(t *testing.T) {
	cases := map[int64]string{
		0: "0", 999: "999", 1234: "1.2k", 37000: "37k", 1234567: "1.2M",
	}
	for in, want := range cases {
		if got := humanizeTokens(in); got != want {
			t.Errorf("humanizeTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestShortenSessionKey(t *testing.T) {
	if got := shortenSessionKey("agent:fox:telegram:direct:123"); got != "telegram:direct:123" {
		t.Errorf("shortenSessionKey = %q", got)
	}
	if got := shortenSessionKey("weird"); got != "weird" {
		t.Errorf("shortenSessionKey passthrough = %q", got)
	}
}
