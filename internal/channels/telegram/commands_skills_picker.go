package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// --- /skills interactive picker (paged buttons + detail + reply-to-run) ---

const (
	// skillPickerPageSize is how many skill buttons one page shows.
	skillPickerPageSize = 10
	// skillPickerTTL keeps a picker message actionable far longer than the
	// preference pickers: reply-to-run is the primary way users launch a
	// skill from the card, often minutes or hours later.
	skillPickerTTL = 24 * time.Hour
	// skillBtnLabelMax truncates button labels so two fit per row.
	skillBtnLabelMax = 20
)

// skillPickerCtx is the mutable state behind one /skills picker message. The
// message is edited in place between list and detail views; sel < 0 means the
// list view is showing.
type skillPickerCtx struct {
	infos     []skills.Info // sorted, whitelist-filtered snapshot
	page      int           // 0-based page index
	sel       int           // index into infos for the detail view; -1 = list
	chatIDStr string
	threadID  int // resolved thread for sends (forum topic / DM thread)
	loc       string
	expires   time.Time
}

// storeSkillPicker records picker state and lazily sweeps expired entries
// (shared sweep heuristic with storePicker).
func (c *Channel) storeSkillPicker(chatID int64, messageID int, spc skillPickerCtx) {
	if n := roughMapLen(&c.pendingSkills); n > 200 {
		now := time.Now()
		c.pendingSkills.Range(func(k, v any) bool {
			if p, ok := v.(skillPickerCtx); ok && now.After(p.expires) {
				c.pendingSkills.Delete(k)
			}
			return true
		})
	}
	c.pendingSkills.Store(chatMsgKey(chatID, messageID), spc)
}

// loadSkillPicker returns live picker state for a message (false when absent
// or expired).
func (c *Channel) loadSkillPicker(chatID int64, messageID int) (skillPickerCtx, bool) {
	raw, ok := c.pendingSkills.Load(chatMsgKey(chatID, messageID))
	if !ok {
		return skillPickerCtx{}, false
	}
	spc, ok := raw.(skillPickerCtx)
	if !ok || time.Now().After(spc.expires) {
		return skillPickerCtx{}, false
	}
	return spc, true
}

// handleSkillsPicker renders the /skills interactive list: 10 buttons per
// page with prev/next navigation, edited in place as the user navigates.
func (c *Channel) handleSkillsPicker(ctx context.Context, chatID int64, chatIDStr string, isGroup, isForum bool, messageThreadID, dmThreadID int, setThread func(*telego.SendMessageParams), tgLang string) {
	if c.skillsLister == nil {
		return // commands.go routes here only when sessionPrefs != nil; lister checked for safety
	}

	infos := c.skillsLister.ListSkills(ctx)
	loc := c.chatLocale(ctx, c.chatSessionKey(chatIDStr, isGroup, isForum, messageThreadID, dmThreadID), tgLang)
	if len(infos) > 0 {
		if whitelist := resolveTopicConfig(c.config, chatIDStr, messageThreadID).skills; whitelist != nil {
			infos = filterSkillsByWhitelist(infos, whitelist)
		}
		sort.Slice(infos, func(i, j int) bool { return infos[i].Slug < infos[j].Slug })
	}
	if len(infos) == 0 {
		msg := tu.Message(tu.ID(chatID), i18n.T(loc, i18n.MsgTGSkillsNone))
		setThread(msg)
		_, _ = c.bot.SendMessage(ctx, msg)
		return
	}

	spc := skillPickerCtx{
		infos:     infos,
		sel:       -1,
		chatIDStr: chatIDStr,
		threadID:  resolveThreadIDForSend(messageThreadID),
		loc:       loc,
		expires:   time.Now().Add(skillPickerTTL),
	}

	msg := tu.Message(tu.ID(chatID), skillPickerText(spc))
	msg.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: skillPickerKeyboard(spc)}
	setThread(msg)
	sent, err := c.bot.SendMessage(ctx, msg)
	if err != nil {
		slog.Warn("skills picker: failed to send", "chat_id", chatIDStr, "error", err)
		return
	}
	c.storeSkillPicker(chatID, sent.MessageID, spc)
}

// skillPickerPages returns the page count for a skill list.
func skillPickerPages(n int) int {
	if n == 0 {
		return 1
	}
	return (n + skillPickerPageSize - 1) / skillPickerPageSize
}

// skillPickerText renders the picker message text for the ctx's current view.
func skillPickerText(spc skillPickerCtx) string {
	if spc.sel >= 0 && spc.sel < len(spc.infos) {
		return skillDetailText(spc.infos[spc.sel], spc.loc)
	}
	return i18n.T(spc.loc, i18n.MsgTGSkillsTitle, len(spc.infos), spc.page+1, skillPickerPages(len(spc.infos))) +
		"\n\n" + i18n.T(spc.loc, i18n.MsgTGSkillsHint)
}

// skillDetailText renders the detail card shown when a skill button is taped:
// full description (no truncation), availability note, run instructions.
func skillDetailText(info skills.Info, loc string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "📦 %s\n\n", info.Name)
	desc := strings.TrimSpace(info.Description)
	if desc == "" {
		desc = i18n.T(loc, i18n.MsgTGSkillsNoDesc)
	}
	sb.WriteString(desc)
	if info.UnavailableReason != "" {
		sb.WriteString("\n\n⚠️ " + i18n.T(loc, i18n.MsgTGSkillsUnavailable, info.UnavailableReason))
	}
	sb.WriteString("\n\n↩️ " + i18n.T(loc, i18n.MsgTGSkillsRunHint, info.Slug))
	return sb.String()
}

// skillPickerKeyboard builds the list-view keyboard: 2 skill buttons per row
// plus one navigation row (prev/page/next; edge buttons hidden).
func skillPickerKeyboard(spc skillPickerCtx) [][]telego.InlineKeyboardButton {
	pages := skillPickerPages(len(spc.infos))
	start := spc.page * skillPickerPageSize
	end := min(start+skillPickerPageSize, len(spc.infos))

	var rows [][]telego.InlineKeyboardButton
	for i := start; i < end; i += 2 {
		row := []telego.InlineKeyboardButton{skillButton(spc.infos[i], i)}
		if i+1 < end {
			row = append(row, skillButton(spc.infos[i+1], i+1))
		}
		rows = append(rows, row)
	}

	nav := make([]telego.InlineKeyboardButton, 0, 3)
	if spc.page > 0 {
		nav = append(nav, telego.InlineKeyboardButton{Text: "◀", CallbackData: fmt.Sprintf("sk:p:%d", spc.page-1)})
	}
	nav = append(nav, telego.InlineKeyboardButton{
		Text:         fmt.Sprintf("%d/%d", spc.page+1, pages),
		CallbackData: fmt.Sprintf("sk:p:%d", spc.page), // self no-op keeps the shape uniform
	})
	if spc.page < pages-1 {
		nav = append(nav, telego.InlineKeyboardButton{Text: "▶", CallbackData: fmt.Sprintf("sk:p:%d", spc.page+1)})
	}
	return append(rows, nav)
}

// skillButton builds one skill entry button. Callback carries the global
// index (slug itself may not fit the 64-byte callback budget).
func skillButton(info skills.Info, idx int) telego.InlineKeyboardButton {
	label := info.Name
	if label == "" {
		label = info.Slug
	}
	return telego.InlineKeyboardButton{
		Text:         truncateStr(label, skillBtnLabelMax),
		CallbackData: fmt.Sprintf("sk:s:%d", idx),
	}
}

// handleSkillsCallback applies sk:p:<page> (navigate) and sk:s:<idx> (show
// detail) by editing the picker message in place. The shared dispatcher
// already answered the callback.
func (c *Channel) handleSkillsCallback(ctx context.Context, query *telego.CallbackQuery, payload string) {
	msgID := query.Message.GetMessageID()
	chatID := query.Message.GetChat().ID

	spc, ok := c.loadSkillPicker(chatID, msgID)
	if !ok {
		c.pendingSkills.Delete(chatMsgKey(chatID, msgID))
		loc := normalizeTGLocale(query.From.LanguageCode)
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGPickerExpired))
		return
	}

	var text string
	var rows [][]telego.InlineKeyboardButton
	switch {
	case strings.HasPrefix(payload, "sk:p:"):
		n, err := strconv.Atoi(strings.TrimPrefix(payload, "sk:p:"))
		if err != nil {
			return
		}
		pages := skillPickerPages(len(spc.infos))
		spc.page = n
		if spc.page < 0 {
			spc.page = 0
		}
		if spc.page > pages-1 {
			spc.page = pages - 1
		}
		spc.sel = -1
		text, rows = skillPickerText(spc), skillPickerKeyboard(spc)
	case strings.HasPrefix(payload, "sk:s:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(payload, "sk:s:"))
		if err != nil || idx < 0 || idx >= len(spc.infos) {
			return
		}
		spc.sel = idx
		text = skillPickerText(spc)
		rows = [][]telego.InlineKeyboardButton{{
			{Text: "◀ " + i18n.T(spc.loc, i18n.MsgTGSkillsBack), CallbackData: fmt.Sprintf("sk:p:%d", spc.page)},
		}}
	default:
		return
	}

	if _, err := c.bot.EditMessageText(ctx, &telego.EditMessageTextParams{
		ChatID:      tu.ID(chatID),
		MessageID:   msgID,
		Text:        text,
		ReplyMarkup: &telego.InlineKeyboardMarkup{InlineKeyboard: rows},
	}); err != nil {
		// Deleted message / "not modified" — best-effort UI, never fatal.
		slog.Debug("skills picker: edit failed", "message_id", msgID, "error", err)
		return
	}
	c.storeSkillPicker(chatID, msgID, spc)
}

// transformInteractiveReply rewrites a reply that targets one of the bot's
// interactive messages into the equivalent session input:
//   - ask_options question → "[Answering your question] <typed text>"
//   - skill detail card    → "/<slug> <typed text>" (runs the skill)
//
// Anything else passes through unchanged. Replies that are themselves slash
// commands are left alone so the command runs as typed.
func (c *Channel) transformInteractiveReply(reply *telego.Message, content string) string {
	trimmed := strings.TrimSpace(content)
	if reply == nil || trimmed == "" || strings.HasPrefix(trimmed, "/") {
		return content
	}
	key := chatMsgKey(reply.Chat.ID, reply.MessageID)

	if raw, ok := c.pendingAsks.Load(key); ok {
		if ac, ok := raw.(askCtx); ok && time.Now().Before(ac.expires) {
			return "[Answering your question] " + trimmed
		}
		return content
	}
	if raw, ok := c.pendingSkills.Load(key); ok {
		if spc, ok := raw.(skillPickerCtx); ok && time.Now().Before(spc.expires) &&
			spc.sel >= 0 && spc.sel < len(spc.infos) {
			slug := spc.infos[spc.sel].Slug
			// Never rewrite to a slug that collides with a built-in bot
			// command — handleBotCommand would dispatch that instead of
			// running the skill.
			if builtinBotCommands[strings.ToLower(slug)] {
				return content
			}
			return "/" + slug + " " + trimmed
		}
	}
	return content
}

// builtinBotCommands is the handleBotCommand switch set (commands.go). A
// skill sharing one of these names must not be reachable via reply-to-run.
var builtinBotCommands = map[string]bool{
	"start": true, "help": true, "reset": true, "stop": true, "stopall": true,
	"status": true, "thinking": true, "reasoning": true, "dev": true,
	"language": true, "tasks": true, "skills": true, "task_detail": true,
	"subagents": true, "subagent": true, "addwriter": true, "removewriter": true,
	"writers": true, "addcron": true, "removecron": true, "croners": true,
	"reactions": true,
}
