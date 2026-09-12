package telegram

import (
	"context"
	"log/slog"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
)

// --- Command locale resolution + /language ---

// MetaKeyLocale is the session-metadata key holding a per-chat locale
// override set by /language. Exported for the consumer side if it ever needs
// to read it.
const MetaKeyLocale = "locale"

// normalizeTGLocale maps a Telegram client language code to a supported
// catalog locale ("" when unsupported → i18n.T falls back to English).
func normalizeTGLocale(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(strings.SplitN(lang, "-", 2)[0]))
	if !i18n.IsSupported(lang) {
		return ""
	}
	return lang
}

// chatLocale resolves the locale for command replies in this chat:
// /language override > Telegram client language > "" (English).
func (c *Channel) chatLocale(ctx context.Context, sessionKey, tgLang string) string {
	if v := c.chatPrefsValue(ctx, sessionKey, MetaKeyLocale); v != "" && i18n.IsSupported(v) {
		return v
	}
	return normalizeTGLocale(tgLang)
}

// validLocales is the /language accepted set (matches the catalogs).
var validLocales = []string{"en", "vi", "zh", "ko", "ru"}

// handleLanguageCommand implements /language [locale]. With no argument it
// shows the current locale and the accepted list; with a valid locale it
// persists the per-chat override.
func (c *Channel) handleLanguageCommand(ctx context.Context, chatID int64, chatIDStr, senderID string, isGroup, isForum bool, messageThreadID, dmThreadID int, setThread func(*telego.SendMessageParams), tgLang, arg string) {
	chatIDObj := tu.ID(chatID)
	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		setThread(msg)
		if _, err := c.bot.SendMessage(ctx, msg); err != nil {
			slog.Warn("language command: failed to send reply", "chat_id", chatIDStr, "error", err)
		}
	}

	if c.sessionPrefs == nil {
		send(i18n.T("en", i18n.MsgTGLangUnavailable))
		return
	}
	if !c.requireChatWriter(ctx, chatID, isGroup, chatIDStr, senderID, setThread) {
		return
	}

	sessionKey := c.chatSessionKey(chatIDStr, isGroup, isForum, messageThreadID, dmThreadID)
	loc := c.chatLocale(ctx, sessionKey, tgLang)

	arg = strings.ToLower(strings.TrimSpace(arg))
	if arg == "" {
		current := loc
		if current == "" {
			current = "en"
		}
		send(i18n.T(loc, i18n.MsgTGLangCurrent, current) + "\n" +
			i18n.T(loc, i18n.MsgTGLangSetHint, strings.Join(validLocales, " · ")))
		return
	}
	if !i18n.IsSupported(arg) {
		send(i18n.T(loc, i18n.MsgTGLangUnsupported, arg, strings.Join(validLocales, " · ")))
		return
	}
	c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyLocale: arg})
	send(i18n.T(arg, i18n.MsgTGLangSet, arg))
}
