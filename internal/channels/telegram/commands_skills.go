package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// --- Skills command + bot skill menu ---

// SkillsLister lists the skills available to the agent loop. Implemented by
// *skills.Loader; kept narrow so the channel does not depend on loader
// internals and tests can supply a fake.
type SkillsLister interface {
	ListSkills(ctx context.Context) []skills.Info
}

// WithSkillsLister sets the skills lister backing the /skills command and the
// skill shortcuts in the Telegram bot command menu.
func WithSkillsLister(l SkillsLister) Option { return func(c *Channel) { c.skillsLister = l } }

// defaultMenuSkills are the skill shortcuts registered in the Telegram "/"
// command menu when channels.telegram.menu_skills is not configured.
var defaultMenuSkills = []string{"cook", "plan", "fix", "review", "test"}

// telegramCommandRE matches the subset of skill slugs Telegram accepts as bot
// commands: 1-32 chars of [a-z0-9_]. telego does not validate this client-side
// (types.go doc comment only) — the Bot API rejects anything else at runtime,
// so slugs like "ui-ux-pro-max" must be filtered before SetMyCommands.
var telegramCommandRE = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

const (
	// skillMenuDescMaxLen leaves room for truncateStr's appended "…" while
	// staying under Telegram's 256-char BotCommand.Description hard limit.
	skillMenuDescMaxLen = 255
	skillListDescMaxLen = 120 // per-line description truncation in /skills
	maxSkillsInList     = 50
)

// menuSkillSlugs resolves the configured menu skill slugs (nil = defaults).
func menuSkillSlugs(configured []string) []string {
	if configured == nil {
		return defaultMenuSkills
	}
	return configured
}

// skillMenuCommands builds bot-command menu entries for the given skill slugs.
// Slugs Telegram cannot accept are skipped with a warning; descriptions come
// from the lister when one is wired (truncated to one line), otherwise a
// generic "Run skill: <slug>" fallback is used.
func skillMenuCommands(ctx context.Context, slugs []string, lister SkillsLister) []telego.BotCommand {
	descriptions := map[string]string{}
	if lister != nil {
		for _, info := range lister.ListSkills(ctx) {
			descriptions[info.Slug] = firstLine(info.Description)
		}
	}

	commands := make([]telego.BotCommand, 0, len(slugs))
	for _, slug := range slugs {
		if !telegramCommandRE.MatchString(slug) {
			slog.Warn("telegram: skill menu entry skipped (not a valid bot command)", "slug", slug)
			continue
		}
		desc := truncateStr(descriptions[slug], skillMenuDescMaxLen)
		if desc == "" {
			desc = fmt.Sprintf("Run skill: %s", slug)
		}
		commands = append(commands, telego.BotCommand{Command: slug, Description: desc})
	}
	return commands
}

// handleSkillsList handles the /skills command — lists available skills with
// descriptions directly from the loader, without spending an LLM call.
func (c *Channel) handleSkillsList(ctx context.Context, chatID int64, chatIDStr string, messageThreadID int, setThread func(*telego.SendMessageParams)) {
	chatIDObj := tu.ID(chatID)

	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		setThread(msg)
		if _, err := c.bot.SendMessage(ctx, msg); err != nil {
			slog.Warn("skills command: failed to send reply", "chat_id", chatIDStr, "error", err)
		}
	}

	if c.skillsLister == nil {
		send("Skills are not available.")
		return
	}

	infos := c.skillsLister.ListSkills(ctx)
	if len(infos) == 0 {
		send("No skills are installed yet.")
		return
	}

	// Mirror the agent-side skill filter: group/topic whitelists restrict which
	// skills may run in this chat (nil = all skills allowed).
	if whitelist := resolveTopicConfig(c.config, chatIDStr, messageThreadID).skills; whitelist != nil {
		infos = filterSkillsByWhitelist(infos, whitelist)
	}

	// Chunk upfront like the normal reply path (Send → chunkHTML): sendHTML
	// only splits reactively on the API's "message too long" error.
	threadID := resolveThreadIDForSend(messageThreadID)
	for _, chunk := range chunkHTML(buildSkillsListHTML(infos), telegramMaxMessageLen) {
		if err := c.sendHTML(ctx, chatID, chunk, 0, threadID); err != nil {
			slog.Warn("skills command: failed to send list", "chat_id", chatIDStr, "error", err)
			return
		}
	}
}

// filterSkillsByWhitelist keeps only skills whose slug is listed. A nil
// whitelist means no restriction. An empty whitelist (non-nil) removes
// everything, matching TelegramGroupConfig.Skills semantics (nil = all,
// [] = none).
func filterSkillsByWhitelist(infos []skills.Info, whitelist []string) []skills.Info {
	if whitelist == nil {
		return infos
	}
	allowed := make(map[string]struct{}, len(whitelist))
	for _, slug := range whitelist {
		allowed[slug] = struct{}{}
	}
	filtered := make([]skills.Info, 0, len(whitelist))
	for _, info := range infos {
		if _, ok := allowed[info.Slug]; ok {
			filtered = append(filtered, info)
		}
	}
	return filtered
}

// buildSkillsListHTML renders the /skills reply as Telegram HTML: one
// "<code>/slug</code> — description" line per skill, sorted by slug, capped at
// maxSkillsInList entries with a "+N more" note.
func buildSkillsListHTML(infos []skills.Info) string {
	sorted := make([]skills.Info, len(infos))
	copy(sorted, infos)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slug < sorted[j].Slug })

	hidden := 0
	if len(sorted) > maxSkillsInList {
		hidden = len(sorted) - maxSkillsInList
		sorted = sorted[:maxSkillsInList]
	}

	var sb strings.Builder
	sb.WriteString("<b>Available skills</b>\n\n")
	for _, info := range sorted {
		desc := firstLine(info.Description)
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(&sb, "<code>/%s</code> — %s\n", escapeHTML(info.Slug), escapeHTML(truncateStr(desc, skillListDescMaxLen)))
	}
	if hidden > 0 {
		fmt.Fprintf(&sb, "\n… and %d more", hidden)
	}
	sb.WriteString("\n\nRun a skill by starting a message with its command: <code>/skill-slug</code> your request")
	return sb.String()
}

// firstLine returns the first non-empty trimmed line of a multi-line string.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
