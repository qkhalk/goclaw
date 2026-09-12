package telegram

import (
	"context"
	"log/slog"
	"strings"
	"time"

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

// localeButtonLabels are self-named button labels (native names + flags) —
// intentionally untranslated so each reader recognizes their own language.
var localeButtonLabels = map[string]string{
	"en": "🇬🇧 English",
	"vi": "🇻🇳 Tiếng Việt",
	"zh": "🇨🇳 中文",
	"ko": "🇰🇷 한국어",
	"ru": "🇷🇺 Русский",
}

// handleLanguageCommand implements /language [locale]. With no argument it
// renders the language picker keyboard; with a valid locale it persists the
// per-chat override directly.
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
		c.sendLanguagePicker(ctx, chatID, chatIDStr, sessionKey, setThread, loc)
		return
	}
	if !i18n.IsSupported(arg) {
		send(i18n.T(loc, i18n.MsgTGLangUnsupported, arg, strings.Join(validLocales, " · ")))
		return
	}
	c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyLocale: arg})
	send(i18n.T(arg, i18n.MsgTGLangSet, arg))
}

// sendLanguagePicker renders the language keyboard with the current choice
// marked. Taps come back as "lg:<locale>" callbacks.
func (c *Channel) sendLanguagePicker(ctx context.Context, chatID int64, chatIDStr, sessionKey string, setThread func(*telego.SendMessageParams), loc string) {
	current := c.chatPrefsValue(ctx, sessionKey, MetaKeyLocale)
	if current == "" {
		current = normalizeTGLocale(loc)
	}

	var rows [][]telego.InlineKeyboardButton
	for _, lc := range validLocales {
		label := localeButtonLabels[lc]
		if lc == current {
			label = "✅ " + label
		}
		rows = append(rows, []telego.InlineKeyboardButton{{Text: label, CallbackData: "lg:" + lc}})
	}

	msg := tu.Message(tu.ID(chatID), i18n.T(loc, i18n.MsgTGLangTitle))
	msg.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	setThread(msg)
	sent, err := c.bot.SendMessage(ctx, msg)
	if err != nil {
		slog.Warn("language picker: failed to send", "chat_id", chatIDStr, "error", err)
		return
	}
	c.storePicker(chatID, sent.MessageID, pickerCtx{kind: "language", sessionKey: sessionKey, loc: loc, expires: time.Now().Add(pickerTTL)})
}

// applyLanguagePick validates the picked locale, persists it, and edits the
// picker card into a confirmation.
func (c *Channel) applyLanguagePick(ctx context.Context, chatID int64, msgID int, sessionKey, locale, loc string) {
	if !i18n.IsSupported(locale) {
		c.editPickerMessage(ctx, chatID, msgID, i18n.T(loc, i18n.MsgTGPickerExpired))
		return
	}
	c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyLocale: locale})
	slog.Info("chat prefs: language set via picker", "session", sessionKey, "locale", locale)
	c.editPickerMessage(ctx, chatID, msgID, i18n.T(locale, i18n.MsgTGLangSet, locale))
}
