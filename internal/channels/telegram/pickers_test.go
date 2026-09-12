package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// --- /skills picker pagination math ---

func TestSkillPickerPages(t *testing.T) {
	for _, tc := range []struct {
		n, want int
	}{{0, 1}, {1, 1}, {10, 1}, {11, 2}, {25, 3}} {
		if got := skillPickerPages(tc.n); got != tc.want {
			t.Errorf("skillPickerPages(%d) = %d, want %d", tc.n, got, tc.want)
		}
	}
}

func TestSkillButton_LabelShowsNameAndDescription(t *testing.T) {
	info := skills.Info{Slug: "security-audit", Name: "Security Audit", Description: "Pre-production security assessment with severity-ranked findings.\nSecond line."}
	btn := skillButton(info, 3)
	if !strings.HasPrefix(btn.Text, "Security Audit — ") {
		t.Errorf("label = %q, want name + description prefix", btn.Text)
	}
	if !strings.Contains(btn.Text, "severity-ranked") {
		t.Errorf("label = %q, want first description line", btn.Text)
	}
	if strings.Contains(btn.Text, "Second line") {
		t.Errorf("label = %q, want only the first description line", btn.Text)
	}
	if btn.CallbackData != "sk:s:3" {
		t.Errorf("callback = %q", btn.CallbackData)
	}

	noDesc := skillButton(skills.Info{Slug: "x", Name: "X"}, 0)
	if noDesc.Text != "X" {
		t.Errorf("no-desc label = %q, want bare name", noDesc.Text)
	}
}

func makeSkills(n int) []skills.Info {
	infos := make([]skills.Info, n)
	for i := range infos {
		infos[i] = skills.Info{Slug: strings.Repeat("s", 1) + string(rune('a'+i%26)) + string(rune('0'+i/26)), Name: "Skill"}
	}
	return infos
}

func callbackButtons(rows [][]telego.InlineKeyboardButton) []telego.InlineKeyboardButton {
	var out []telego.InlineKeyboardButton
	for _, row := range rows {
		out = append(out, row...)
	}
	return out
}

func TestSkillPickerKeyboard_Pagination(t *testing.T) {
	spc := skillPickerCtx{infos: makeSkills(25), page: 0, sel: -1, expires: time.Now().Add(time.Minute)}

	first := skillPickerKeyboard(spc)
	// Page 0: 10 full-width skill rows + nav (◀ hidden, ▶ visible).
	firstBtns := callbackButtons(first)
	if len(firstBtns) != 12 {
		t.Fatalf("page0 buttons = %d, want 12 (10 skills + indicator + next)", len(firstBtns))
	}
	if first[10][0].Text != "1/3" {
		t.Errorf("page indicator = %q, want 1/3", first[10][0].Text)
	}
	for _, b := range firstBtns {
		if b.Text == "◀" {
			t.Errorf("page 0 must hide prev button")
		}
	}

	spc.page = 2
	last := skillPickerKeyboard(spc)
	lastBtns := callbackButtons(last)
	if len(lastBtns) != 5+2 { // 5 skills on last page + ◀ + indicator
		t.Fatalf("page2 buttons = %d, want 7", len(lastBtns))
	}
	for _, b := range lastBtns {
		if b.Text == "▶" {
			t.Errorf("last page must hide next button")
		}
	}

	// Callback data must stay within Telegram's 64-byte budget.
	for _, b := range append(firstBtns, lastBtns...) {
		if len(b.CallbackData) > 64 {
			t.Errorf("callback data too long: %q", b.CallbackData)
		}
	}
}

func TestSkillPicker_DetailView(t *testing.T) {
	spc := skillPickerCtx{infos: []skills.Info{
		{Slug: "security-audit", Name: "Security Audit", Description: "Full audit."},
	}, sel: 0, loc: "en", expires: time.Now().Add(time.Minute)}

	text := skillPickerText(spc)
	if !strings.Contains(text, "Security Audit") || !strings.Contains(text, "/security-audit") {
		t.Errorf("detail text missing name/run hint:\n%s", text)
	}
	rows := [][]telego.InlineKeyboardButton{{
		{Text: "◀ Back to list", CallbackData: "sk:p:0"},
	}}
	if rows[0][0].CallbackData != "sk:p:0" {
		t.Fatalf("back callback = %q", rows[0][0].CallbackData)
	}
}

// --- callbacks edit the picker message ---

func newPickerTestChannel(t *testing.T, lister SkillsLister) (*Channel, *recordingTelegramCaller) {
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
	ch := &Channel{
		BaseChannel:  channels.NewBaseChannel("telegram", bus.New(), nil),
		bot:          bot,
		skillsLister: lister,
	}
	ch.SetName("telegram")
	ch.SetAgentID("fox")
	ch.SetRunning(true)
	return ch, caller
}

func testCallbackQuery(data string, chatID int64, msgID int, lang string) *telego.CallbackQuery {
	return &telego.CallbackQuery{
		ID: "cbq1",
		From: telego.User{
			ID:           42,
			FirstName:    "Tester",
			LanguageCode: lang,
		},
		Message: &telego.Message{
			MessageID: msgID,
			Chat:      telego.Chat{ID: chatID, Type: "private"},
			Date:      time.Now().Unix(),
		},
		Data: data,
	}
}

func TestHandleSkillsCallback_NavigatesPages(t *testing.T) {
	ch, caller := newPickerTestChannel(t, &fakeSkillsLister{infos: makeSkills(25)})
	ch.storeSkillPicker(-100, 101, skillPickerCtx{
		infos: makeSkills(25), page: 0, sel: -1, chatIDStr: "-100",
		threadID: 0, loc: "en", expires: time.Now().Add(time.Minute),
	})

	ch.handleSkillsCallback(context.Background(), testCallbackQuery("sk:p:1", -100, 101, "vi"), "sk:p:1")

	var edit *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "editMessageText" {
			edit = &caller.calls[i]
			break
		}
	}
	if edit == nil {
		t.Fatalf("editMessageText not called, methods: %v", caller.methodNames())
	}
	if text, _ := edit.body["text"].(string); !strings.Contains(text, "2/3") {
		t.Errorf("edited text = %q, want page 2/3 marker", text)
	}
	// State must be persisted with the new page.
	raw, ok := ch.pendingSkills.Load(chatMsgKey(-100, 101))
	if !ok {
		t.Fatalf("picker state missing after navigation")
	}
	spc := raw.(skillPickerCtx)
	if spc.page != 1 || spc.sel != -1 {
		t.Errorf("stored page=%d sel=%d, want page=1 sel=-1", spc.page, spc.sel)
	}
}

func TestHandleSkillsCallback_ExpiredEditsNotice(t *testing.T) {
	ch, caller := newPickerTestChannel(t, nil)

	ch.handleSkillsCallback(context.Background(), testCallbackQuery("sk:p:0", -100, 101, "en"), "sk:p:0")

	var edit *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "editMessageText" {
			edit = &caller.calls[i]
			break
		}
	}
	if edit == nil {
		t.Fatalf("editMessageText not called, methods: %v", caller.methodNames())
	}
	if text, _ := edit.body["text"].(string); !strings.Contains(text, "expired") {
		t.Errorf("edited text = %q, want expired notice", text)
	}
}

// --- reply-to-run / answer transforms ---

func TestTransformInteractiveReply_SkillRun(t *testing.T) {
	ch, _ := newPickerTestChannel(t, nil)
	ch.pendingSkills.Store(chatMsgKey(-100, 101), skillPickerCtx{
		infos:   []skills.Info{{Slug: "security-audit"}},
		sel:     0,
		expires: time.Now().Add(time.Minute),
	})

	reply := &telego.Message{MessageID: 101, Chat: telego.Chat{ID: -100}}
	got := ch.transformInteractiveReply(reply, "check my server")
	if got != "/security-audit check my server" {
		t.Errorf("transform = %q, want skill-run rewrite", got)
	}
}

func TestTransformInteractiveReply_AskQuestion(t *testing.T) {
	ch, _ := newPickerTestChannel(t, nil)
	ch.pendingAsks.Store(chatMsgKey(-100, 101), askCtx{
		question: "Which DB?",
		options:  []string{"Postgres", "MySQL"},
		expires:  time.Now().Add(time.Minute),
	})

	reply := &telego.Message{MessageID: 101, Chat: telego.Chat{ID: -100}}
	got := ch.transformInteractiveReply(reply, "we use postgres 16")
	if got != "[Answering your question] we use postgres 16" {
		t.Errorf("transform = %q, want answer prefix", got)
	}
}

func TestTransformInteractiveReply_Passthrough(t *testing.T) {
	ch, _ := newPickerTestChannel(t, nil)
	ch.pendingSkills.Store(chatMsgKey(-100, 101), skillPickerCtx{
		infos:   []skills.Info{{Slug: "cook"}},
		sel:     0,
		expires: time.Now().Add(time.Minute),
	})

	reply := &telego.Message{MessageID: 101, Chat: telego.Chat{ID: -100}}
	other := &telego.Message{MessageID: 999, Chat: telego.Chat{ID: -100}}
	cases := []struct {
		name, in, want string
	}{
		{"slash commands untouched", "/cook plan it", "/cook plan it"},
		{"empty content", "", ""},
		{"unknown reply target", "hello", "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := reply
			if tc.name == "unknown reply target" {
				target = other
			}
			if got := ch.transformInteractiveReply(target, tc.in); got != tc.want {
				t.Errorf("transform(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// Expired skill ctx must not transform.
	ch.pendingSkills.Store(chatMsgKey(-100, 101), skillPickerCtx{
		infos: []skills.Info{{Slug: "cook"}}, sel: 0, expires: time.Now().Add(-time.Minute),
	})
	if got := ch.transformInteractiveReply(reply, "run it"); got != "run it" {
		t.Errorf("expired transform = %q, want passthrough", got)
	}

	// List view (sel=-1) must not transform.
	ch.pendingSkills.Store(chatMsgKey(-100, 101), skillPickerCtx{
		infos: []skills.Info{{Slug: "cook"}}, sel: -1, expires: time.Now().Add(time.Minute),
	})
	if got := ch.transformInteractiveReply(reply, "run it"); got != "run it" {
		t.Errorf("list-view transform = %q, want passthrough", got)
	}
}

// --- ask_options keyboard + callbacks ---

func TestAskKeyboard(t *testing.T) {
	rows := askKeyboard([]string{"Postgres", "MySQL", "SQLite"}, "vi")
	if len(rows) != 2+1 { // 2 option rows + Other
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	other := rows[len(rows)-1][0]
	if other.CallbackData != "ak:o" || !strings.HasPrefix(other.Text, "✏️") {
		t.Errorf("other button = %+v", other)
	}
	for _, b := range callbackButtons(rows) {
		if len(b.CallbackData) > 64 {
			t.Errorf("callback data too long: %q", b.CallbackData)
		}
	}
}

func TestHandleAskCallback_OptionPublishesInbound(t *testing.T) {
	ch, caller := newPickerTestChannel(t, nil)
	tenant := uuid.New()
	ch.SetTenantID(tenant)
	ch.pendingAsks.Store(chatMsgKey(-100, 101), askCtx{
		question:  "Which DB?",
		options:   []string{"Postgres", "MySQL"},
		chatIDStr: "-100",
		localKey:  "-100",
		expires:   time.Now().Add(time.Minute),
	})

	ch.handleAskCallback(context.Background(), testCallbackQuery("ak:1", -100, 101, "vi"), "ak:1")

	inCh := make(chan bus.InboundMessage, 1)
	go func() {
		if in, ok := ch.Bus().ConsumeInbound(context.Background()); ok {
			inCh <- in
		}
	}()
	select {
	case in := <-inCh:
		if !strings.HasPrefix(in.Content, "[Answering your question] Which DB?") ||
			!strings.Contains(in.Content, "MySQL") {
			t.Errorf("inbound content = %q", in.Content)
		}
		if in.Channel != "telegram" || in.ChatID != "-100" || in.SenderID != "42" {
			t.Errorf("inbound routing = %+v", in)
		}
		if in.TenantID != tenant {
			t.Errorf("tenant = %v, want %v", in.TenantID, tenant)
		}
	case <-time.After(time.Second):
		t.Fatalf("no inbound message published")
	}

	// Question message must be edited with the answered card.
	var edit *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "editMessageText" {
			edit = &caller.calls[i]
			break
		}
	}
	if edit == nil {
		t.Fatalf("editMessageText not called, methods: %v", caller.methodNames())
	}
	// State must be consumed.
	if _, ok := ch.pendingAsks.Load(chatMsgKey(-100, 101)); ok {
		t.Errorf("pendingAsk still present after answer")
	}
}

func TestHandleAskCallback_OtherKeepsWaiting(t *testing.T) {
	ch, caller := newPickerTestChannel(t, nil)
	ch.pendingAsks.Store(chatMsgKey(-100, 101), askCtx{
		question:  "Which DB?",
		options:   []string{"Postgres"},
		chatIDStr: "-100",
		localKey:  "-100",
		expires:   time.Now().Add(time.Minute),
	})

	ch.handleAskCallback(context.Background(), testCallbackQuery("ak:o", -100, 101, "vi"), "ak:o")

	otherCh := make(chan bus.InboundMessage, 1)
	go func() {
		if in, ok := ch.Bus().ConsumeInbound(context.Background()); ok {
			otherCh <- in
		}
	}()
	select {
	case in := <-otherCh:
		t.Fatalf("Other must not publish inbound, got %q", in.Content)
	case <-time.After(50 * time.Millisecond):
	}

	var edit *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "editMessageText" {
			edit = &caller.calls[i]
			break
		}
	}
	if edit == nil {
		t.Fatalf("editMessageText not called")
	}
	if text, _ := edit.body["text"].(string); !strings.Contains(text, "Which DB?") {
		t.Errorf("edited text = %q, want question + hint", text)
	}
	if _, ok := ch.pendingAsks.Load(chatMsgKey(-100, 101)); !ok {
		t.Errorf("pendingAsk must stay for the reply path")
	}
}

func TestSendAskQuestion_SendsKeyboard(t *testing.T) {
	ch, caller := newPickerTestChannel(t, nil)
	encoded, _ := json.Marshal([]string{"Postgres", "MySQL"})

	if err := ch.sendAskQuestion(context.Background(), -100, "-100", "Which DB?", string(encoded), 0, 0); err != nil {
		t.Fatalf("sendAskQuestion: %v", err)
	}

	var send *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "sendMessage" {
			send = &caller.calls[i]
			break
		}
	}
	if send == nil {
		t.Fatalf("sendMessage not called, methods: %v", caller.methodNames())
	}
	if text, _ := send.body["text"].(string); !strings.Contains(text, "Which DB?") {
		t.Errorf("sent text = %q", text)
	}
	// The recording caller always returns message_id 101.
	if _, ok := ch.pendingAsks.Load(chatMsgKey(-100, 101)); !ok {
		t.Errorf("pendingAsk not stored after send")
	}
}

// --- preference pickers (thinking / reasoning / dev) ---

func TestStorePickerChatKeyed(t *testing.T) {
	ch, _ := newPickerTestChannel(t, nil)
	// Same message ID in two chats must not collide.
	ch.storePicker(-100, 42, pickerCtx{kind: "dev", expires: time.Now().Add(time.Minute)})
	ch.storePicker(200, 42, pickerCtx{kind: "thinking", expires: time.Now().Add(time.Minute)})

	if _, ok := ch.pendingPickers.Load(chatMsgKey(-100, 42)); !ok {
		t.Errorf("chat -100 entry missing")
	}
	if _, ok := ch.pendingPickers.Load(chatMsgKey(200, 42)); !ok {
		t.Errorf("chat 200 entry missing")
	}
}

func TestHandlePickerCallback_Expired(t *testing.T) {
	ch, caller := newPickerTestChannel(t, nil)

	ch.handlePickerCallback(context.Background(), testCallbackQuery("th:high", -100, 101, "en"), "th:high")

	var edit *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "editMessageText" {
			edit = &caller.calls[i]
			break
		}
	}
	if edit == nil {
		t.Fatalf("editMessageText not called, methods: %v", caller.methodNames())
	}
	if text, _ := edit.body["text"].(string); !strings.Contains(text, "expired") {
		t.Errorf("edited text = %q, want expired notice", text)
	}
}

// --- review-driven regressions ---

func TestTransformInteractiveReply_BuiltinSlugCollision(t *testing.T) {
	ch, _ := newPickerTestChannel(t, nil)
	ch.pendingSkills.Store(chatMsgKey(-100, 101), skillPickerCtx{
		infos:   []skills.Info{{Slug: "status"}}, // collides with /status
		sel:     0,
		expires: time.Now().Add(time.Minute),
	})
	reply := &telego.Message{MessageID: 101, Chat: telego.Chat{ID: -100}}
	if got := ch.transformInteractiveReply(reply, "show me"); got != "show me" {
		t.Errorf("builtin-slug transform = %q, want untouched content", got)
	}
}

func TestSendAskQuestion_PlaceholderSentinelSendsFresh(t *testing.T) {
	ch, caller := newPickerTestChannel(t, nil)
	// FinalizeStream stores -1 when a stream message landed with unknown ID.
	ch.placeholders.Store("-100", -1)
	encoded, _ := json.Marshal([]string{"Postgres"})

	if err := ch.sendAskQuestion(context.Background(), -100, "-100", "Which DB?", string(encoded), 0, 0); err != nil {
		t.Fatalf("sendAskQuestion: %v", err)
	}
	var send *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "sendMessage" {
			send = &caller.calls[i]
			break
		}
	}
	if send == nil {
		t.Fatalf("sendMessage not called, methods: %v", caller.methodNames())
	}
	if text, _ := send.body["text"].(string); !strings.Contains(text, "Which DB?") {
		t.Errorf("sent text = %q", text)
	}
}

func TestApplyDevPick_PersistsMode(t *testing.T) {
	prefs := newFakePrefsStore()
	ch, _ := newPrefsTestChannel(t, prefs, nil)
	ch.SetAgentID("fox")

	ch.applyDevPick(context.Background(), -100, 101, "agent:fox:telegram:direct:1", "on", "en")
	if got := prefs.data["agent:fox:telegram:direct:1"]["chat_mode"]; got != "dev" {
		t.Errorf("chat_mode = %q, want dev", got)
	}

	ch.applyDevPick(context.Background(), -100, 101, "agent:fox:telegram:direct:1", "off", "en")
	if got := prefs.data["agent:fox:telegram:direct:1"]["chat_mode"]; got != "" {
		t.Errorf("chat_mode = %q, want empty", got)
	}
}

// Full production loop: /thinking sends the picker (recording caller returns
// message_id 101) and the tap on THAT message must apply the level — this is
// the regression test for the "picker has expired" report.
func TestSendThinkingPicker_HappyPath(t *testing.T) {
	prefs := newFakePrefsStore()
	ch, caller := newPrefsTestChannel(t, prefs, nil)
	ch.SetAgentID("fox")

	ch.sendThinkingPicker(context.Background(), 7148278449, "7148278449", false, false, 0, 0,
		func(*telego.SendMessageParams) {}, "thinking", "en")

	var send *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "sendMessage" {
			send = &caller.calls[i]
			break
		}
	}
	if send == nil {
		t.Fatalf("sendMessage not called")
	}
	if _, ok := ch.pendingPickers.Load(chatMsgKey(7148278449, 101)); !ok {
		t.Fatalf("picker entry not stored under the sent message id")
	}
	ch.handlePickerCallback(context.Background(), testCallbackQuery("th:high", 7148278449, 101, "en"), "th:high")

	if got := prefs.data["agent:fox:telegram:direct:7148278449"]["thinking_level"]; got != "high" {
		t.Errorf("thinking_level = %q, want high", got)
	}
	var edit *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "editMessageText" {
			edit = &caller.calls[i]
			break
		}
	}
	if edit == nil {
		t.Fatalf("editMessageText not called")
	}
	if text, _ := edit.body["text"].(string); !strings.Contains(text, "high") {
		t.Errorf("confirm text = %q, want thinking level", text)
	}
}

// --- /language picker ---

func TestSendLanguagePicker_ShowsCurrentChoice(t *testing.T) {
	prefs := newFakePrefsStore()
	ch, caller := newPickerTestChannel(t, nil)
	ch.SetAgentID("fox")
	ch.sessionPrefs = prefs

	// loc "en" (client language fallback) → English marked current.
	ch.sendLanguagePicker(context.Background(), -100, "-100", "agent:fox:telegram:direct:1", func(*telego.SendMessageParams) {}, "en")

	var send *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "sendMessage" {
			send = &caller.calls[i]
			break
		}
	}
	if send == nil {
		t.Fatalf("sendMessage not called")
	}
	markup, _ := send.body["reply_markup"].(map[string]any)
	if markup == nil {
		t.Fatalf("reply_markup missing")
	}
	keyboard, _ := markup["inline_keyboard"].([]any)
	if len(keyboard) != 5 {
		t.Fatalf("keyboard rows = %d, want 5 locales", len(keyboard))
	}
	// No stored override, client lang "en" → English marked current.
	row0, _ := keyboard[0].([]any)
	btn0, _ := row0[0].(map[string]any)
	if !strings.HasPrefix(btn0["text"].(string), "✅") {
		t.Errorf("first button = %v, want ✅ English", btn0["text"])
	}
	if _, ok := ch.pendingPickers.Load(chatMsgKey(-100, 101)); !ok {
		t.Fatalf("language picker not stored")
	}

	// Tap Vietnamese → persisted + confirmation edit.
	ch.handlePickerCallback(context.Background(), testCallbackQuery("lg:vi", -100, 101, "en"), "lg:vi")
	if got := prefs.data["agent:fox:telegram:direct:1"]["locale"]; got != "vi" {
		t.Errorf("locale = %q, want vi", got)
	}
}

func TestSendLanguagePicker_OverrideMarkedCurrent(t *testing.T) {
	prefs := newFakePrefsStore()
	prefs.SetSessionMetadata(context.Background(), "agent:fox:telegram:direct:1", map[string]string{"locale": "vi"})
	ch, caller := newPickerTestChannel(t, nil)
	ch.SetAgentID("fox")
	ch.sessionPrefs = prefs

	ch.sendLanguagePicker(context.Background(), -100, "-100", "agent:fox:telegram:direct:1", func(*telego.SendMessageParams) {}, "")

	var send *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "sendMessage" {
			send = &caller.calls[i]
			break
		}
	}
	markup, _ := send.body["reply_markup"].(map[string]any)
	keyboard, _ := markup["inline_keyboard"].([]any)
	viRow, _ := keyboard[1].([]any)
	viBtn, _ := viRow[0].(map[string]any)
	if !strings.HasPrefix(viBtn["text"].(string), "✅") {
		t.Errorf("vi button = %v, want ✅ marked", viBtn["text"])
	}
	enRow, _ := keyboard[0].([]any)
	enBtn, _ := enRow[0].(map[string]any)
	if strings.HasPrefix(enBtn["text"].(string), "✅") {
		t.Errorf("en button = %v, want unmarked when vi is current", enBtn["text"])
	}
}

// Regression for the field-reported "Mức thinking: level%!(EXTRA string=auto)"
// — the pick appliers must render the level with exactly one argument.
func TestApplyThinkingPick_ConfirmationTextHasNoFormatNoise(t *testing.T) {
	prefs := newFakePrefsStore()
	ch, caller := newPrefsTestChannel(t, prefs, nil)
	ch.SetAgentID("fox")

	ch.applyThinkingPick(context.Background(), -100, 101, "agent:fox:telegram:direct:1", "auto", "vi")

	if got := prefs.data["agent:fox:telegram:direct:1"]["thinking_level"]; got != "auto" {
		t.Fatalf("thinking_level = %q, want auto", got)
	}
	var edit *recordedTelegramCall
	for i := len(caller.calls) - 1; i >= 0; i-- {
		if caller.calls[i].method == "editMessageText" {
			edit = &caller.calls[i]
			break
		}
	}
	if edit == nil {
		t.Fatalf("editMessageText not called")
	}
	text, _ := edit.body["text"].(string)
	if strings.Contains(text, "%!") {
		t.Errorf("confirmation text has format noise: %q", text)
	}
	if !strings.Contains(text, "auto") {
		t.Errorf("confirmation text = %q, want the picked level", text)
	}
}
