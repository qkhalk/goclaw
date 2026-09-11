package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/sessions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- Per-chat session preferences (session metadata) ---

// SessionPrefsStore is the narrow slice of the session store the channel needs
// to persist per-chat preferences (thinking level, chat mode, status
// verbosity) in the session's metadata JSONB. Implemented by
// *store.PGSessionStore and *store.SQLiteSessionStore; tests supply a fake.
//
// SetSessionMetadata only mutates the in-memory session cache — callers MUST
// call Save afterwards to persist (same pattern as gateway/methods/sessions.go
// handlePatch). Get is read-through, so values survive gateway restarts.
type SessionPrefsStore interface {
	Get(ctx context.Context, key string) *store.SessionData
	SetSessionMetadata(ctx context.Context, key string, metadata map[string]string)
	Save(ctx context.Context, key string) error
}

// WithSessionPrefs sets the session store backing per-chat preference commands
// (/thinking, /dev, /status full|short). Nil = those commands reply that
// preferences are unavailable.
func WithSessionPrefs(s SessionPrefsStore) Option { return func(c *Channel) { c.sessionPrefs = s } }

// Session metadata keys authored by the channel preference commands. The
// gateway consumer reads ThinkingLevel and ChatMode at RunRequest build time
// (cmd/gateway_consumer_normal.go) — keep both sides in sync through these
// exported constants.
const (
	MetaKeyThinkingLevel   = "thinking_level"      // /thinking <level>
	MetaKeyChatMode        = "chat_mode"           // /dev on|off ("dev" or "")
	MetaKeyStatusVerbosity = "tg_status_verbosity" // /status full|short
)

// chatSessionKey reproduces the gateway consumer's session-key construction
// for the current chat (cmd/gateway_consumer_normal.go:83-112: scoped base key
// → forum-topic override → DM-thread override) so preferences written here are
// read back by the next inbound message of the same conversation.
func (c *Channel) chatSessionKey(chatIDStr string, isGroup, isForum bool, messageThreadID, dmThreadID int) string {
	agentKey := c.AgentID()
	peerKind := sessions.PeerDirect
	if isGroup {
		peerKind = sessions.PeerGroup
	}
	key := sessions.BuildScopedSessionKey(agentKey, c.Name(), peerKind, chatIDStr)
	if isForum && peerKind == sessions.PeerGroup && messageThreadID > 0 {
		key = sessions.BuildGroupTopicSessionKey(agentKey, c.Name(), chatIDStr, messageThreadID)
	}
	if dmThreadID > 0 && peerKind == sessions.PeerDirect {
		key = sessions.BuildDMThreadSessionKey(agentKey, c.Name(), chatIDStr, dmThreadID)
	}
	return key
}

// requireChatWriter mirrors the /reset group gate: in group chats only file
// writers may change chat-wide preferences. DB check failures fail open like
// /reset does (commands.go). Returns true when the caller may proceed.
func (c *Channel) requireChatWriter(ctx context.Context, chatID int64, isGroup bool, chatIDStr, senderID string, setThread func(*telego.SendMessageParams)) bool {
	if !isGroup || c.configPermStore == nil {
		return true
	}
	agentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		return true
	}
	groupID := fmt.Sprintf("group:%s:%s", c.Name(), chatIDStr)
	senderNumericID := strings.SplitN(senderID, "|", 2)[0]
	isWriter, err := c.configPermStore.CheckPermission(ctx, agentID, groupID, store.ConfigTypeFileWriter, senderNumericID)
	if err != nil {
		slog.Warn("security.chat_pref_writer_check_failed", "error", err, "sender", senderNumericID)
		return true // fail-open, matching /reset
	}
	if !isWriter {
		chatIDObj := tu.ID(chatID)
		msg := tu.Message(chatIDObj, "Only file writers can change chat settings in this group.")
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return false
	}
	return true
}

// setChatPrefs merges metadata into the chat session and persists it.
// Returns false (after logging) when the store is missing or Save fails —
// the in-memory cache still carries the value for this process either way.
func (c *Channel) setChatPrefs(ctx context.Context, sessionKey string, metadata map[string]string) bool {
	if c.sessionPrefs == nil {
		return false
	}
	c.sessionPrefs.SetSessionMetadata(ctx, sessionKey, metadata)
	if err := c.sessionPrefs.Save(ctx, sessionKey); err != nil {
		slog.Warn("chat prefs: failed to persist session metadata", "session", sessionKey, "error", err)
	}
	return true
}

// chatPrefsValue returns one metadata value for the chat session (read-through
// to DB, so values set before a gateway restart are still visible).
func (c *Channel) chatPrefsValue(ctx context.Context, sessionKey, metaKey string) string {
	if c.sessionPrefs == nil {
		return ""
	}
	if data := c.sessionPrefs.Get(ctx, sessionKey); data != nil {
		return data.Metadata[metaKey]
	}
	return ""
}

// --- /thinking ---

// thinkingLevelList is the user-facing menu of accepted levels (single source
// of truth for the accepted values is providers.NormalizeReasoningEffort plus
// the "adaptive" sentinel, mirroring gateway thinkingOverrideFor).
var thinkingLevelList = []string{"off", "minimal", "low", "medium", "high", "xhigh", "auto", "adaptive"}

// normalizeThinkingLevel maps a /thinking argument to a persisted override:
// a standard effort level or "adaptive" passes through; "none" is rejected
// (it is NOT a disable — on Anthropic it enables thinking with a default
// budget); "default" maps to "" (clear the override, agent config applies).
func normalizeThinkingLevel(arg string) (level string, err error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "default", "reset":
		return "", nil
	case "none":
		return "", fmt.Errorf(`"none" is not accepted — on Claude models it ENABLES thinking. Use "off" to disable or "default" to follow the agent config`)
	}
	if v := providers.NormalizeReasoningEffort(arg); v != "" {
		return v, nil
	}
	if strings.EqualFold(strings.TrimSpace(arg), providers.ReasoningEffortAdaptive) {
		return providers.ReasoningEffortAdaptive, nil
	}
	return "", fmt.Errorf("unknown level %q — valid: %s", arg, strings.Join(thinkingLevelList, ", "))
}

// agentDefaultThinkingLevel reads the agent's configured default level for
// display purposes (raw thinking_level column; "" = provider default).
func (c *Channel) agentDefaultThinkingLevel(ctx context.Context) string {
	if c.agentStore == nil {
		return ""
	}
	key := c.AgentID()
	if key == "" {
		return ""
	}
	agent, err := c.agentStore.GetByKey(ctx, key)
	if err != nil || agent == nil {
		return ""
	}
	return agent.ThinkingLevel
}

// handleThinkingCommand implements /thinking [level|default]. With no argument
// it shows the current override and the agent default; with a valid level it
// persists the per-chat override that the gateway consumer applies to
// RunRequest.ThinkingLevelOverride from the next message onwards.
func (c *Channel) handleThinkingCommand(ctx context.Context, chatID int64, chatIDStr, senderID string, isGroup, isForum bool, messageThreadID, dmThreadID int, setThread func(*telego.SendMessageParams), arg string) {
	chatIDObj := tu.ID(chatID)
	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		msg.ParseMode = telego.ModeHTML
		setThread(msg)
		if _, err := c.bot.SendMessage(ctx, msg); err != nil {
			slog.Warn("thinking command: failed to send reply", "chat_id", chatIDStr, "error", err)
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
	current := c.chatPrefsValue(ctx, sessionKey, MetaKeyThinkingLevel)

	arg = strings.TrimSpace(arg)
	if arg == "" {
		var sb strings.Builder
		sb.WriteString("<b>Thinking level</b>\n")
		if current != "" {
			fmt.Fprintf(&sb, "Current: <code>%s</code> (chat override)\n", escapeHTML(current))
		} else {
			sb.WriteString("Current: <i>agent default</i>\n")
		}
		if def := c.agentDefaultThinkingLevel(ctx); def != "" {
			fmt.Fprintf(&sb, "Agent default: <code>%s</code>\n", escapeHTML(def))
		}
		sb.WriteString("\nSet: <code>/thinking &lt;level&gt;</code> — ")
		sb.WriteString(strings.Join(thinkingLevelList, " · ") + "\n")
		sb.WriteString("Clear: <code>/thinking default</code> — follow agent config\n")
		sb.WriteString("<code>off</code> disables reasoning entirely; takes effect from your next message.")
		send(sb.String())
		return
	}

	level, err := normalizeThinkingLevel(arg)
	if err != nil {
		send("⚠️ " + escapeHTML(err.Error()))
		return
	}

	if level == "" {
		c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyThinkingLevel: ""})
		send("Thinking override cleared — the agent config applies from your next message.")
		return
	}

	c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyThinkingLevel: level})
	slog.Info("chat prefs: thinking level set", "session", sessionKey, "level", level, "chat_id", chatIDStr)
	switch level {
	case "off":
		send(fmt.Sprintf("🧠 Thinking disabled for this chat (was: %s). Takes effect from your next message.", displayLevel(current)))
	default:
		send(fmt.Sprintf("🧠 Thinking level set to <code>%s</code> (was: %s). Takes effect from your next message.", escapeHTML(level), displayLevel(current)))
	}
}

// displayLevel renders the previous level for the confirmation reply.
func displayLevel(level string) string {
	if level == "" {
		return "agent default"
	}
	return escapeHTML(level)
}
