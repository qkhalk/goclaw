package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- Shared inline-picker machinery (thinking / reasoning / dev) ---

// pickerTTL is how long a picker message stays actionable after it is sent.
const pickerTTL = 10 * time.Minute

// chatMsgKey keys pending-* maps: Telegram message IDs are per-chat counters,
// so chatID must disambiguate (chat 100 and chat 200 both have message #42).
func chatMsgKey(chatID int64, messageID int) string {
	return fmt.Sprintf("%d|%d", chatID, messageID)
}

// pickerCtx carries what a callback cannot recover on its own: the session
// key the pick applies to (forum/DM-thread shape is not derivable from the
// callback) and expiry.
type pickerCtx struct {
	kind       string // "thinking" | "reasoning" | "dev"
	sessionKey string
	loc        string // locale resolved at send time (callback cannot recover it)
	expires    time.Time
}

// storePicker records a picker message and lazily sweeps expired entries.
func (c *Channel) storePicker(chatID int64, messageID int, pc pickerCtx) {
	if n := roughMapLen(&c.pendingPickers); n > 200 {
		now := time.Now()
		c.pendingPickers.Range(func(k, v any) bool {
			if p, ok := v.(pickerCtx); ok && now.After(p.expires) {
				c.pendingPickers.Delete(k)
			}
			return true
		})
	}
	c.pendingPickers.Store(chatMsgKey(chatID, messageID), pc)
}

// roughMapLen is a cheap size estimate for the sweep heuristic.
func roughMapLen(m *sync.Map) int {
	n := 0
	m.Range(func(_, _ any) bool { n++; return true })
	return n
}

// editPickerMessage replaces a picker message's text and drops its keyboard.
func (c *Channel) editPickerMessage(ctx context.Context, chatID int64, messageID int, text string) {
	_, err := c.bot.EditMessageText(ctx, &telego.EditMessageTextParams{
		ChatID:    tu.ID(chatID),
		MessageID: messageID,
		Text:      text,
	})
	if err != nil {
		// Deleted message / "not modified" — best-effort UI, never fatal.
		slog.Debug("picker: edit message failed", "message_id", messageID, "error", err)
	}
}

// handlePickerCallback routes inline-keyboard callbacks. The shared dispatcher
// already answered the callback (commands_tasks.go AnswerCallbackQuery).
func (c *Channel) handlePickerCallback(ctx context.Context, query *telego.CallbackQuery, payload string) {
	switch {
	case strings.HasPrefix(payload, "sk:"):
		c.handleSkillsCallback(ctx, query, payload)
		return
	case strings.HasPrefix(payload, "ak:"):
		c.handleAskCallback(ctx, query, payload)
		return
	case strings.HasPrefix(payload, "lg:"):
		c.handleLanguageCallback(ctx, query, payload)
		return
	}

	msgID := query.Message.GetMessageID()
	chatID := query.Message.GetChat().ID

	raw, ok := c.pendingPickers.LoadAndDelete(chatMsgKey(chatID, msgID))
	if !ok {
		loc := normalizeTGLocale(query.From.LanguageCode)
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGPickerExpired))
		return
	}
	pc, ok := raw.(pickerCtx)
	if !ok || time.Now().After(pc.expires) {
		loc := normalizeTGLocale(query.From.LanguageCode)
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGPickerExpired))
		return
	}
	loc := pc.loc

	// The callback data is "th:<level>" / "dv:on|off" / "lg:<locale>" — the
	// appliers expect the bare value ("high", "on", "vi"), not the prefixed
	// form.
	_, value, _ := strings.Cut(payload, ":")

	switch pc.kind {
	case "dev":
		c.applyDevPick(ctx, chatID, msgID, pc.sessionKey, value, loc)
	case "language":
		c.applyLanguagePick(ctx, chatID, msgID, pc.sessionKey, value, loc)
	default: // thinking | reasoning share the value space
		c.applyThinkingPick(ctx, chatID, msgID, pc.sessionKey, value, loc)
	}
}

// handleLanguageCallback applies lg:<locale> picks without a stored pickerCtx
// when possible — the locale value itself is self-describing, but the session
// key is not, so state is still required.
func (c *Channel) handleLanguageCallback(ctx context.Context, query *telego.CallbackQuery, payload string) {
	msgID := query.Message.GetMessageID()
	chatID := query.Message.GetChat().ID

	raw, ok := c.pendingPickers.LoadAndDelete(chatMsgKey(chatID, msgID))
	if !ok {
		loc := normalizeTGLocale(query.From.LanguageCode)
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGPickerExpired))
		return
	}
	pc, ok := raw.(pickerCtx)
	if !ok || time.Now().After(pc.expires) {
		loc := normalizeTGLocale(query.From.LanguageCode)
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGPickerExpired))
		return
	}
	_, locale, _ := strings.Cut(payload, ":")
	c.applyLanguagePick(ctx, chatID, msgID, pc.sessionKey, locale, pc.loc)
}

// applyThinkingPick validates the payload, persists it, edits confirmation.
func (c *Channel) applyThinkingPick(ctx context.Context, chatID int64, msgID int, sessionKey, payload, loc string) {
	level, err := normalizeThinkingLevel(payload)
	if err != nil {
		// Payload came from our own keyboard; anything else is stale data.
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGPickerExpired))
		return
	}
	if level == "" {
		c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyThinkingLevel: ""})
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGThinkingCleared))
		return
	}
	c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyThinkingLevel: level})
	slog.Info("chat prefs: thinking level set via picker", "session", sessionKey, "level", level)
	c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGThinkingSet, "level", level))
}

// applyDevPick handles dv:on / dv:off.
func (c *Channel) applyDevPick(ctx context.Context, chatID int64, msgID int, sessionKey, payload, loc string) {
	switch payload {
	case "on":
		c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyChatMode: "dev"})
		slog.Info("chat prefs: dev mode enabled via picker", "session", sessionKey)
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGDevEnabled))
	case "off":
		c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyChatMode: ""})
		slog.Info("chat prefs: dev mode disabled via picker", "session", sessionKey)
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGDevDisabled))
	default:
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGPickerExpired))
	}
}

// --- Model-aware thinking levels ---

// resolveAgentData fetches the full AgentData for the channel's agent key
// (nil when the store or agent is unavailable).
func (c *Channel) resolveAgentData(ctx context.Context) *store.AgentData {
	key := c.AgentID()
	if key == "" || c.agentStore == nil {
		return nil
	}
	if id, err := uuid.Parse(key); err == nil {
		if agent, gerr := c.agentStore.GetByID(ctx, id); gerr == nil {
			return agent
		}
		return nil
	}
	ctx = store.WithTenantID(ctx, c.TenantID())
	agent, err := c.agentStore.GetByKey(ctx, key)
	if err != nil {
		return nil
	}
	return agent
}

// thinkingLevelsForModel returns the picker levels for the agent's model:
// capability-filtered when the model is in the registry (GPT-5/Codex family),
// the full standard list otherwise. "none" never appears — it enables Claude
// thinking instead of disabling (see session_prefs.go).
func thinkingLevelsForModel(agent *store.AgentData) []string {
	if agent != nil && agent.Model != "" {
		if capInfo := providers.LookupReasoningCapability(agent.Model); capInfo != nil && len(capInfo.Levels) > 0 {
			levels := make([]string, 0, len(capInfo.Levels))
			for _, l := range capInfo.Levels {
				if l == "none" {
					continue
				}
				levels = append(levels, l)
			}
			if len(levels) > 0 {
				return levels
			}
		}
	}
	return thinkingLevelList
}

// --- /thinking + /reasoning + /dev pickers ---

// sendThinkingPicker renders the level keyboard with the current selection
// marked. Levels are laid out 3 per row; Default closes.
func (c *Channel) sendThinkingPicker(ctx context.Context, chatID int64, chatIDStr string, isGroup, isForum bool, messageThreadID, dmThreadID int, setThread func(*telego.SendMessageParams), kind, loc string) {
	sessionKey := c.chatSessionKey(chatIDStr, isGroup, isForum, messageThreadID, dmThreadID)

	agent := c.resolveAgentData(ctx)
	levels := thinkingLevelsForModel(agent)
	current := c.chatPrefsValue(ctx, sessionKey, MetaKeyThinkingLevel)

	title := i18n.T(loc, i18n.MsgTGThinkingTitle)
	if kind == "reasoning" {
		title = i18n.T(loc, i18n.MsgTGReasoningTitle)
	}
	if current != "" {
		title += "\n" + i18n.T(loc, i18n.MsgTGThinkingCurrent, "level", current)
	} else if agent != nil && agent.ThinkingLevel != "" {
		title += "\n" + i18n.T(loc, i18n.MsgTGThinkingAgentDef, "level", agent.ThinkingLevel)
	}

	var rows [][]telego.InlineKeyboardButton
	row := make([]telego.InlineKeyboardButton, 0, 3)
	for _, lvl := range levels {
		label := lvl
		if lvl == current {
			label = "✅ " + lvl
		}
		row = append(row, telego.InlineKeyboardButton{Text: label, CallbackData: "th:" + lvl})
		if len(row) == 3 {
			rows = append(rows, row)
			row = make([]telego.InlineKeyboardButton, 0, 3)
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	defaultLabel := i18n.T(loc, i18n.MsgTGThinkingDefault)
	if current == "" {
		defaultLabel = "✅ " + defaultLabel
	}
	rows = append(rows, []telego.InlineKeyboardButton{{Text: defaultLabel, CallbackData: "th:default"}})

	msg := tu.Message(tu.ID(chatID), title)
	msg.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	setThread(msg)
	sent, err := c.bot.SendMessage(ctx, msg)
	if err != nil {
		slog.Warn("picker: failed to send thinking picker", "chat_id", chatIDStr, "error", err)
		return
	}
	c.storePicker(chatID, sent.MessageID, pickerCtx{kind: kind, sessionKey: sessionKey, loc: loc, expires: time.Now().Add(pickerTTL)})
}

// sendReasoningPicker renders the quick ON/OFF keyboard (ON = follow agent
// config, OFF = disable reasoning entirely).
func (c *Channel) sendReasoningPicker(ctx context.Context, chatID int64, chatIDStr string, isGroup, isForum bool, messageThreadID, dmThreadID int, setThread func(*telego.SendMessageParams), loc string) {
	sessionKey := c.chatSessionKey(chatIDStr, isGroup, isForum, messageThreadID, dmThreadID)
	current := c.chatPrefsValue(ctx, sessionKey, MetaKeyThinkingLevel)

	title := i18n.T(loc, i18n.MsgTGReasoningTitle)
	if current != "" {
		title += "\n" + i18n.T(loc, i18n.MsgTGThinkingCurrent, "level", current)
	}

	onLabel := i18n.T(loc, i18n.MsgTGReasoningOn)
	offLabel := i18n.T(loc, i18n.MsgTGReasoningOff)
	if current == "off" {
		offLabel = "✅ " + offLabel
	} else if current == "" {
		onLabel = "✅ " + onLabel
	}
	rows := [][]telego.InlineKeyboardButton{{
		{Text: onLabel, CallbackData: "th:default"},
		{Text: offLabel, CallbackData: "th:off"},
	}}

	msg := tu.Message(tu.ID(chatID), title)
	msg.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	setThread(msg)
	sent, err := c.bot.SendMessage(ctx, msg)
	if err != nil {
		slog.Warn("picker: failed to send reasoning picker", "chat_id", chatIDStr, "error", err)
		return
	}
	c.storePicker(chatID, sent.MessageID, pickerCtx{kind: "reasoning", sessionKey: sessionKey, loc: loc, expires: time.Now().Add(pickerTTL)})
}

// sendDevPicker renders the dev-mode ON/OFF keyboard.
func (c *Channel) sendDevPicker(ctx context.Context, chatID int64, chatIDStr string, isGroup, isForum bool, messageThreadID, dmThreadID int, setThread func(*telego.SendMessageParams), loc string) {
	sessionKey := c.chatSessionKey(chatIDStr, isGroup, isForum, messageThreadID, dmThreadID)
	devOn := c.chatPrefsValue(ctx, sessionKey, MetaKeyChatMode) == "dev"

	title := i18n.T(loc, i18n.MsgTGDevTitle)

	onLabel, offLabel := i18n.T(loc, i18n.MsgTGDevOn), i18n.T(loc, i18n.MsgTGDevOff)
	if devOn {
		onLabel = "✅ " + onLabel
	} else {
		offLabel = "✅ " + offLabel
	}
	rows := [][]telego.InlineKeyboardButton{{
		{Text: onLabel, CallbackData: "dv:on"},
		{Text: offLabel, CallbackData: "dv:off"},
	}}

	msg := tu.Message(tu.ID(chatID), title)
	msg.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	setThread(msg)
	sent, err := c.bot.SendMessage(ctx, msg)
	if err != nil {
		slog.Warn("picker: failed to send dev picker", "chat_id", chatIDStr, "error", err)
		return
	}
	c.storePicker(chatID, sent.MessageID, pickerCtx{kind: "dev", sessionKey: sessionKey, loc: loc, expires: time.Now().Add(pickerTTL)})
}
