package telegram

import (
	"context"
	"log/slog"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// --- /dev — per-chat dev mode toggle ---

// handleDevCommand implements /dev [on|off]. Dev mode injects a
// plan-first/ask-first behavior section into the system prompt of every run in
// this chat (consumer prepends agent.DevModePromptSection to
// ExtraSystemPrompt). With no argument it shows the current mode.
func (c *Channel) handleDevCommand(ctx context.Context, chatID int64, chatIDStr, senderID string, isGroup, isForum bool, messageThreadID, dmThreadID int, setThread func(*telego.SendMessageParams), arg string) {
	chatIDObj := tu.ID(chatID)
	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		msg.ParseMode = telego.ModeHTML
		setThread(msg)
		if _, err := c.bot.SendMessage(ctx, msg); err != nil {
			slog.Warn("dev command: failed to send reply", "chat_id", chatIDStr, "error", err)
		}
	}

	if c.sessionPrefs == nil {
		send("Chat preferences are not available (no session store configured).")
		return
	}
	if !c.requireChatWriter(ctx, chatID, isGroup, chatIDStr, senderID, setThread) {
		return
	}

	sessionKey := c.chatSessionKey(chatIDStr, isGroup, isForum, messageThreadID, dmThreadID)
	current := c.chatPrefsValue(ctx, sessionKey, MetaKeyChatMode) == "dev"

	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "":
		if current {
			send("<b>Dev mode</b>: ON — plan-first, asks clarifying questions, confirms destructive operations.\nTurn off: <code>/dev off</code>")
		} else {
			send("<b>Dev mode</b>: OFF.\nTurn on: <code>/dev on</code> — the agent plans before acting, asks when a request is ambiguous, and confirms before destructive operations.")
		}
		return
	case "on", "enable":
		c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyChatMode: "dev"})
		slog.Info("chat prefs: dev mode enabled", "session", sessionKey, "chat_id", chatIDStr)
		send("👨‍💻 Dev mode ON. The agent now plans before acting, asks when your request is ambiguous, and confirms before destructive operations. Takes effect from your next message.")
	case "off", "disable":
		c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyChatMode: ""})
		slog.Info("chat prefs: dev mode disabled", "session", sessionKey, "chat_id", chatIDStr)
		send("Dev mode OFF — normal behavior from your next message.")
	default:
		send("⚠️ Usage: <code>/dev on</code> or <code>/dev off</code>")
	}
}

// devModeStatusLine renders the Mode: field for /status ("dev" or "normal").
func devModeStatusLine(chatMode string) string {
	if chatMode == "dev" {
		return "dev"
	}
	return "normal"
}
