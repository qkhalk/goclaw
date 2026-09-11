package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// --- /status — rich runtime status ---

// StatusGatewayInfo carries process-level facts for the /status command.
type StatusGatewayInfo struct {
	Version         string        // cmd.Version stamp ("dev" when unset)
	StartedAt       time.Time     // gateway process start (zero = unknown)
	SystemUptime    time.Duration // host uptime from /proc/uptime (0 = unavailable)
	LaneName        string        // scheduler lane reported ("" = queue segment omitted)
	LaneActive      int
	LaneConcurrency int
	LanePending     int
}

// StatusSessionInfo carries the chat session's runtime facts. All numeric
// fields are best-effort: zero means "not recorded yet".
type StatusSessionInfo struct {
	Model            string
	Provider         string
	UpdatedAt        time.Time
	InputTokens      int64
	OutputTokens     int64
	LastPromptTokens int
	ContextWindow    int
	CompactionCount  int
}

// StatusProvider supplies gateway/session facts for /status. Implemented in
// cmd/ (gateway_status_provider.go) where server/scheduler/stores live; tests
// supply fakes.
type StatusProvider interface {
	Gateway(ctx context.Context) StatusGatewayInfo
	// Session returns the session facts for sessionKey (false = no session yet).
	Session(ctx context.Context, sessionKey string) (StatusSessionInfo, bool)
	// SessionCost returns the summed traces.total_cost for sessionKey
	// (false = no cost recorded yet).
	SessionCost(ctx context.Context, sessionKey string) (float64, bool)
}

// WithStatusProvider sets the status provider backing the /status command.
// Nil = /status falls back to an availability notice.
func WithStatusProvider(p StatusProvider) Option { return func(c *Channel) { c.statusProvider = p } }

// status verbosity defaults: DMs render the full card, groups the short one
// (spam safety). /status full|short overrides per chat.
const (
	statusVerbosityFull  = "full"
	statusVerbosityShort = "short"
)

// handleStatusCommand implements /status [full|short]. It renders runtime
// facts from the injected StatusProvider plus chat-level preferences
// (thinking override, dev mode) — no LLM call. Output is plain text.
func (c *Channel) handleStatusCommand(ctx context.Context, chatID int64, chatIDStr string, isGroup, isForum bool, messageThreadID, dmThreadID int, setThread func(*telego.SendMessageParams), arg string) {
	chatIDObj := tu.ID(chatID)
	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		setThread(msg)
		if _, err := c.bot.SendMessage(ctx, msg); err != nil {
			slog.Warn("status command: failed to send reply", "chat_id", chatIDStr, "error", err)
		}
	}

	if c.statusProvider == nil {
		send("Status is not available (no provider configured).")
		return
	}

	sessionKey := c.chatSessionKey(chatIDStr, isGroup, isForum, messageThreadID, dmThreadID)

	// Resolve verbosity: explicit argument (persisted) > stored pref > default.
	verbosity := strings.ToLower(strings.TrimSpace(arg))
	if verbosity == statusVerbosityFull || verbosity == statusVerbosityShort {
		c.setChatPrefs(ctx, sessionKey, map[string]string{MetaKeyStatusVerbosity: verbosity})
	} else {
		verbosity = c.chatPrefsValue(ctx, sessionKey, MetaKeyStatusVerbosity)
	}
	if verbosity != statusVerbosityFull && verbosity != statusVerbosityShort {
		verbosity = statusVerbosityShort
		if !isGroup {
			verbosity = statusVerbosityFull
		}
	}

	gw := c.statusProvider.Gateway(ctx)
	sess, hasSession := c.statusProvider.Session(ctx, sessionKey)

	var sb strings.Builder
	fmt.Fprintf(&sb, "🦊 GoClaw %s\n", gw.Version)
	if gw.StartedAt.IsZero() {
		sb.WriteString("⏱️ Uptime: gateway unknown")
	} else {
		sb.WriteString("⏱️ Uptime: gateway " + humanizeDuration(time.Since(gw.StartedAt)))
	}
	if gw.SystemUptime > 0 {
		sb.WriteString(" · system " + humanizeDuration(gw.SystemUptime))
	}
	sb.WriteString("\n")

	model := sess.Model
	if sess.Provider != "" && model != "" {
		model = sess.Provider + "/" + model
	}
	if model == "" {
		model = "unknown"
	}
	fmt.Fprintf(&sb, "🤖 Agent: %s · 🧠 Model: %s\n", c.AgentID(), model)

	if verbosity == statusVerbosityShort {
		if hasSession {
			fmt.Fprintf(&sb, "🧵 Session updated %s\n", humanizeDuration(time.Since(sess.UpdatedAt)))
		} else {
			sb.WriteString("🧵 No session yet\n")
		}
	} else {
		if hasSession {
			fmt.Fprintf(&sb, "🧵 Session: %s · updated %s\n", shortenSessionKey(sessionKey), humanizeDuration(time.Since(sess.UpdatedAt)))
		} else {
			sb.WriteString("🧵 No session yet\n")
		}

		if hasSession {
			if cost, ok := c.statusProvider.SessionCost(ctx, sessionKey); ok {
				fmt.Fprintf(&sb, "💵 Cost (session): $%.4f", cost)
			} else {
				sb.WriteString("💵 Cost (session): —")
			}
			fmt.Fprintf(&sb, " · 🔢 Tokens: %s in / %s out\n", humanizeTokens(sess.InputTokens), humanizeTokens(sess.OutputTokens))

			if sess.ContextWindow > 0 {
				if sess.LastPromptTokens > 0 {
					pct := 100 * sess.LastPromptTokens / sess.ContextWindow
					fmt.Fprintf(&sb, "📚 Context: %s/%s (%d%%)", humanizeTokens(int64(sess.LastPromptTokens)), humanizeTokens(int64(sess.ContextWindow)), pct)
				} else {
					fmt.Fprintf(&sb, "📚 Context: ?/%s", humanizeTokens(int64(sess.ContextWindow)))
				}
			} else {
				sb.WriteString("📚 Context: ?")
			}
			fmt.Fprintf(&sb, " · 🧹 Compactions: %d\n", sess.CompactionCount)
		} else {
			sb.WriteString("💵 Cost (session): — · 🧹 Compactions: 0\n")
		}

		// Chat-level runtime preferences.
		think := c.chatPrefsValue(ctx, sessionKey, MetaKeyThinkingLevel)
		if think == "" {
			if def := c.agentDefaultThinkingLevel(ctx); def != "" {
				think = def + " (agent)"
			} else {
				think = "auto"
			}
		}
		fmt.Fprintf(&sb, "⚙️ Think: %s · Mode: %s", think, devModeStatusLine(c.chatPrefsValue(ctx, sessionKey, MetaKeyChatMode)))
		if gw.LaneName != "" {
			fmt.Fprintf(&sb, " · 🪢 Queue: %s %d/%d active, %d pending", gw.LaneName, gw.LaneActive, gw.LaneConcurrency, gw.LanePending)
		}
		sb.WriteString("\n")
		sb.WriteString("📖 Docs: https://github.com/qkhalk/goclaw/blob/dev/docs/25-telegram-runtime-commands.md\n")
	}

	if verbosity == statusVerbosityShort && strings.TrimSpace(arg) == "" && isGroup {
		sb.WriteString("\n/status full — show everything")
	}

	send(strings.TrimRight(sb.String(), "\n"))
}

// shortenSessionKey drops the "agent:<key>:" prefix for display.
func shortenSessionKey(key string) string {
	if rest, ok := strings.CutPrefix(key, "agent:"); ok {
		if _, after, found := strings.Cut(rest, ":"); found {
			return after
		}
	}
	return key
}

// humanizeDuration renders coarse durations like "2d 3h", "5m", "just now".
func humanizeDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// humanizeTokens renders token counts compactly: 1234 → "1.2k", 37000 → "37k".
func humanizeTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
