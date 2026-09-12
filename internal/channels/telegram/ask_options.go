package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
)

// --- ask_options channel side: question keyboard + answer routing ---

// askPickerTTL keeps a question's buttons/reply-hook actionable. The reply
// path itself is stateless (transformInteractiveReply prefixes the reply, the
// question text stays in the replied-to message), so expiry only degrades
// button presses to an "expired" edit.
const askPickerTTL = 24 * time.Hour

// askOtherPayload is the callback data of the free-text "Other" button.
const askOtherPayload = "ak:o"

// askCtx is the state behind one ask_options question message.
type askCtx struct {
	question  string
	options   []string
	chatIDStr string // raw ChatID (may carry :topic:/:thread: suffix)
	localKey  string // composite key the consumer expects in metadata
	isForum   bool
	threadID  int
	expires   time.Time
}

// askKeyboard builds the option keyboard: two options per row plus a
// dedicated Other row. Callback data carries the option index; labels are
// recovered from askCtx (64-byte callback budget).
func askKeyboard(options []string, loc string) [][]telego.InlineKeyboardButton {
	var rows [][]telego.InlineKeyboardButton
	for i := 0; i < len(options); i += 2 {
		row := []telego.InlineKeyboardButton{{Text: options[i], CallbackData: fmt.Sprintf("ak:%d", i)}}
		if i+1 < len(options) {
			row = append(row, telego.InlineKeyboardButton{Text: options[i+1], CallbackData: fmt.Sprintf("ak:%d", i+1)})
		}
		rows = append(rows, row)
	}
	return append(rows, []telego.InlineKeyboardButton{
		{Text: "✏️ " + i18n.T(loc, "telegram.ask.other"), CallbackData: askOtherPayload},
	})
}

// sendAskQuestion renders an ask_options question: plain text (no HTML so a
// markdown-ish question never breaks delivery) with the option keyboard. The
// placeholder for this chat, if any, is edited into the question so the turn
// visibly ends here.
func (c *Channel) sendAskQuestion(ctx context.Context, chatID int64, localKey, question, encodedOptions string, replyTo, threadID int) error {
	var options []string
	if err := json.Unmarshal([]byte(encodedOptions), &options); err != nil {
		return fmt.Errorf("ask_options: invalid options payload: %w", err)
	}
	if len(options) == 0 {
		return fmt.Errorf("ask_options: empty options payload")
	}
	loc := c.chatLocale(ctx, c.sessionKeyFromLocalKey(localKey, threadID), "")
	keyboard := telego.InlineKeyboardMarkup{InlineKeyboard: askKeyboard(options, loc)}

	msgID := 0
	if pID, ok := c.placeholders.LoadAndDelete(localKey); ok {
		msgID = pID.(int)
		if msgID < 0 {
			// Stream sentinel: a stream message landed but its ID is unknown —
			// send the question fresh instead of editing a ghost.
			msgID = 0
		}
		if msgID > 0 {
			if _, err := c.bot.EditMessageText(ctx, &telego.EditMessageTextParams{
				ChatID:      tu.ID(chatID),
				MessageID:   msgID,
				Text:        question,
				ReplyMarkup: &keyboard,
			}); err != nil {
				slog.Warn("ask_options: placeholder edit failed, sending fresh", "chat_id", chatID, "error", err)
				msgID = 0
			}
		}
	}
	if msgID == 0 {
		tgMsg := tu.Message(tu.ID(chatID), question)
		if sendThreadID := resolveThreadIDForSend(threadID); sendThreadID > 0 {
			tgMsg.MessageThreadID = sendThreadID
		}
		if replyTo > 0 {
			tgMsg.ReplyParameters = &telego.ReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true}
		}
		tgMsg.ReplyMarkup = &keyboard
		sent, err := c.bot.SendMessage(ctx, tgMsg)
		if err != nil {
			return err
		}
		msgID = sent.MessageID
	}

	c.storeAsk(chatID, msgID, askCtx{
		question:  question,
		options:   options,
		chatIDStr: c.rawChatIDFromLocalKey(localKey),
		localKey:  localKey,
		isForum:   strings.Contains(localKey, ":topic:"),
		threadID:  threadID,
		expires:   time.Now().Add(askPickerTTL),
	})
	return nil
}

// storeAsk records a question message and lazily sweeps expired entries.
func (c *Channel) storeAsk(chatID int64, messageID int, ac askCtx) {
	if n := roughMapLen(&c.pendingAsks); n > 200 {
		now := time.Now()
		c.pendingAsks.Range(func(k, v any) bool {
			if p, ok := v.(askCtx); ok && now.After(p.expires) {
				c.pendingAsks.Delete(k)
			}
			return true
		})
	}
	c.pendingAsks.Store(chatMsgKey(chatID, messageID), ac)
}

// sessionKeyFromLocalKey rebuilds the consumer's session key from a localKey
// (chat id plus :topic:/:thread: suffix). Group chat IDs are negative in
// Telegram, which distinguishes the peer kind on the send path.
func (c *Channel) sessionKeyFromLocalKey(localKey string, threadID int) string {
	if localKey == "" {
		return ""
	}
	chatIDStr := c.rawChatIDFromLocalKey(localKey)
	isGroup := strings.HasPrefix(chatIDStr, "-")
	isForum := strings.Contains(localKey, ":topic:")
	dmThreadID := 0
	if strings.Contains(localKey, ":thread:") {
		dmThreadID = threadID
	}
	return c.chatSessionKey(chatIDStr, isGroup, isForum, threadID, dmThreadID)
}

// rawChatIDFromLocalKey strips the :topic:/:thread: suffix from a localKey.
func (c *Channel) rawChatIDFromLocalKey(localKey string) string {
	if idx := strings.Index(localKey, ":topic:"); idx > 0 {
		return localKey[:idx]
	}
	if idx := strings.Index(localKey, ":thread:"); idx > 0 {
		return localKey[:idx]
	}
	return localKey
}

// handleAskCallback applies ak:<idx> (option picked) and ak:o (Other). The
// shared dispatcher already answered the callback.
func (c *Channel) handleAskCallback(ctx context.Context, query *telego.CallbackQuery, payload string) {
	msgID := query.Message.GetMessageID()
	chatID := query.Message.GetChat().ID
	chat := query.Message.GetChat()
	isGroup := chat.Type == "group" || chat.Type == "supergroup"
	loc := normalizeTGLocale(query.From.LanguageCode)

	raw, ok := c.pendingAsks.Load(chatMsgKey(chatID, msgID))
	if !ok {
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, "telegram.picker.expired"))
		return
	}
	ac, ok := raw.(askCtx)
	if !ok || time.Now().After(ac.expires) {
		c.pendingAsks.Delete(chatMsgKey(chatID, msgID))
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, "telegram.picker.expired"))
		return
	}

	if payload == askOtherPayload {
		// Keep the buttons (editMessageText drops the keyboard when
		// reply_markup is omitted) and append the free-text hint.
		if _, err := c.bot.EditMessageText(ctx, &telego.EditMessageTextParams{
			ChatID:      tu.ID(chatID),
			MessageID:   msgID,
			Text:        ac.question + "\n\n" + i18n.T(loc, i18n.MsgTGAskOtherHint),
			ReplyMarkup: &telego.InlineKeyboardMarkup{InlineKeyboard: askKeyboard(ac.options, loc)},
		}); err != nil {
			slog.Debug("ask_options: other-hint edit failed", "message_id", msgID, "error", err)
		}
		return
	}

	idx := 0
	if _, err := fmt.Sscanf(payload, "ak:%d", &idx); err != nil || idx < 0 || idx >= len(ac.options) {
		return
	}
	c.pendingAsks.Delete(chatMsgKey(chatID, msgID))
	c.publishAskAnswer(query, ac, ac.options[idx], isGroup)
	c.editPickerMessage(ctx, chatID, msgID,
		i18n.T(loc, i18n.MsgTGAskAnswered, strings.TrimSpace(ac.question), ac.options[idx]))
}

// publishAskAnswer injects the chosen option into the session as a normal
// user turn (pattern: /reset inbound publish, commands.go).
func (c *Channel) publishAskAnswer(query *telego.CallbackQuery, ac askCtx, answer string, isGroup bool) {
	senderID := query.From.ID
	peerKind := "direct"
	if isGroup {
		peerKind = "group"
	}
	metadata := map[string]string{
		"is_forum":          fmt.Sprintf("%t", ac.isForum),
		"message_thread_id": fmt.Sprintf("%d", ac.threadID),
	}
	if ac.localKey != "" {
		metadata["local_key"] = ac.localKey
	}
	c.Bus().PublishInbound(bus.InboundMessage{
		Channel:  c.Name(),
		SenderID: fmt.Sprintf("%d", senderID),
		ChatID:   ac.chatIDStr,
		Content:  "[Answering your question] " + strings.TrimSpace(ac.question) + " → " + answer,
		PeerKind: peerKind,
		AgentID:  c.AgentID(),
		UserID:   fmt.Sprintf("%d", senderID),
		TenantID: c.TenantID(),
		Metadata: metadata,
	})
}
