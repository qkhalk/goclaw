package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const maxSubagentsInList = 30

// subagentStatusIcon returns an icon for each subagent task status.
func subagentStatusIcon(status string) string {
	switch status {
	case "queued":
		return "⏳"
	case "waiting_child":
		return "↪️"
	case "completed":
		return "✅"
	case "failed":
		return "❌"
	case "cancelled":
		return "⏹"
	default: // running
		return "🔄"
	}
}

// formatTokenCount formats token counts as "1.2k" for readability.
func formatTokenCount(n int64) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// handleSubagentsList handles /subagents — lists subagent tasks from DB.
func (c *Channel) handleSubagentsList(ctx context.Context, chatID int64, isGroup bool, setThread func(*telego.SendMessageParams)) {
	chatIDObj := tu.ID(chatID)

	send := func(text string) {
		msg := tu.Message(chatIDObj, text)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
	}

	if c.subagentTaskStore == nil {
		send("Subagent task tracking is not available.")
		return
	}

	rootAgentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		slog.Warn("subagents command: resolve agent UUID failed", "error", err)
		send("Subagent tasks are not available (agent could not be resolved).")
		return
	}

	tasks, err := c.subagentTaskStore.ListByParent(ctx, rootAgentID, "", false)
	if err != nil {
		slog.Warn("subagents command: ListByParent failed", "error", err)
		send("Failed to list subagent tasks. Please try again.")
		return
	}
	tasks = filterSelfCloneTasks(tasks)

	if len(tasks) == 0 {
		send("No subagent tasks found.")
		return
	}

	loc := c.subagentsListLocale(ctx, chatID, isGroup)
	text, rows := subagentsListContent(tasks, rootAgentID, loc)

	msg := tu.Message(chatIDObj, text)
	setThread(msg)
	if len(rows) > 0 {
		msg.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	c.bot.SendMessage(ctx, msg)
}

// subagentsListLocale resolves the best-effort locale for /subagents button
// labels: the per-chat /language override when the session store is
// configured, English otherwise (the command path carries no client language).
func (c *Channel) subagentsListLocale(ctx context.Context, chatID int64, isGroup bool) string {
	chatIDStr := fmt.Sprintf("%d", chatID)
	return c.chatLocale(ctx, c.chatSessionKey(chatIDStr, isGroup, false, 0, 0), "")
}

// subagentsListContent renders the /subagents list body and inline keyboard
// from an already filtered task list. Each row keeps the sa: detail button;
// terminal tasks (completed/failed/cancelled) add a 🗄 archive button with
// callback ar:<taskUUID> (39 bytes, within Telegram's 64-byte budget). When at
// least one terminal task exists a footer row archives all completed tasks of
// the root agent via ar:all:<agentUUID> (43 bytes). Status icons stay the
// shared subagentStatusIcon vocabulary. Shared by the /subagents command and
// the ar: callback re-render; loc localizes the archive labels ("" = English).
func subagentsListContent(tasks []store.SubagentTaskData, rootAgentID uuid.UUID, loc string) (string, [][]telego.InlineKeyboardButton) {
	if len(tasks) == 0 {
		return "No subagent tasks found.", nil
	}
	total := len(tasks)
	terminalCount := 0
	for i := range tasks {
		if store.IsTerminalSubagentTaskStatus(tasks[i].Status) {
			terminalCount++
		}
	}
	if total > maxSubagentsInList {
		tasks = tasks[:maxSubagentsInList]
	}

	var sb strings.Builder
	if total > maxSubagentsInList {
		sb.WriteString(fmt.Sprintf("Subagent tasks (showing %d of %d):\n\n", maxSubagentsInList, total))
	} else {
		sb.WriteString(fmt.Sprintf("Subagent tasks (%d):\n\n", total))
	}

	for i, t := range tasks {
		model := ""
		if t.Model != nil && *t.Model != "" {
			model = *t.Model
		}
		tokens := fmt.Sprintf("%s/%s tokens", formatTokenCount(t.InputTokens), formatTokenCount(t.OutputTokens))
		if model != "" {
			sb.WriteString(fmt.Sprintf("%d. %s %s (%s, %s)\n", i+1, subagentStatusIcon(t.Status), truncateStr(t.Subject, 40), model, tokens))
		} else {
			sb.WriteString(fmt.Sprintf("%d. %s %s (%s)\n", i+1, subagentStatusIcon(t.Status), truncateStr(t.Subject, 40), tokens))
		}
	}
	sb.WriteString("\nTap a button below to view details.")

	var rows [][]telego.InlineKeyboardButton
	for i, t := range tasks {
		label := fmt.Sprintf("%d. %s %s", i+1, subagentStatusIcon(t.Status), truncateStr(t.Subject, 35))
		row := []telego.InlineKeyboardButton{
			{Text: label, CallbackData: "sa:" + t.ID.String()},
		}
		if store.IsTerminalSubagentTaskStatus(t.Status) {
			row = append(row, telego.InlineKeyboardButton{
				Text:         i18n.T(loc, i18n.MsgTGSubagentArchiveBtn),
				CallbackData: "ar:" + t.ID.String(),
			})
		}
		rows = append(rows, row)
	}
	if terminalCount >= 1 {
		rows = append(rows, []telego.InlineKeyboardButton{{
			Text:         i18n.T(loc, i18n.MsgTGSubagentArchiveAllBtn, terminalCount),
			CallbackData: "ar:all:" + rootAgentID.String(),
		}})
	}
	return sb.String(), rows
}

// handleSubagentDetail handles /subagent <id> — shows detail for a subagent task.
func (c *Channel) handleSubagentDetail(ctx context.Context, chatID int64, text string, isGroup bool, setThread func(*telego.SendMessageParams)) {
	chatIDObj := tu.ID(chatID)

	send := func(t string) {
		for _, chunk := range chunkPlainText(t, telegramMaxMessageLen) {
			msg := tu.Message(chatIDObj, chunk)
			setThread(msg)
			c.bot.SendMessage(ctx, msg)
		}
	}

	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		send("Usage: /subagent <task_id>")
		return
	}
	idArg := strings.TrimSpace(parts[1])

	if c.subagentTaskStore == nil {
		send("Subagent task tracking is not available.")
		return
	}

	taskID, err := uuid.Parse(idArg)
	if err != nil {
		send(fmt.Sprintf("Invalid task ID %q. Use /subagents to list tasks.", idArg))
		return
	}

	rootAgentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		slog.Warn("subagent command: resolve agent UUID failed", "error", err)
		send("Subagent tasks are not available (agent could not be resolved).")
		return
	}
	task, err := c.subagentTaskStore.Get(ctx, rootAgentID, taskID)
	if err != nil {
		slog.Warn("subagent command: Get failed", "id", idArg, "error", err)
		send("Failed to load subagent task. Please try again.")
		return
	}
	if task == nil {
		send(fmt.Sprintf("Task %q not found. Use /subagents to see available tasks.", idArg[:8]))
		return
	}
	if isDelegationCompletion(task) {
		send(fmt.Sprintf("Task %q is a delegation result; retrieve it with the delegate tool.", idArg[:8]))
		return
	}

	send(formatSubagentDetail(task))
}

// handleSubagentCallback handles "sa:" callback prefix from inline keyboard buttons.
func (c *Channel) handleSubagentCallback(ctx context.Context, query *telego.CallbackQuery) {
	taskIDStr := strings.TrimPrefix(query.Data, "sa:")

	chat := query.Message.GetChat()
	chatIDObj := tu.ID(chat.ID)

	send := func(text string) {
		for _, chunk := range chunkPlainText(text, telegramMaxMessageLen) {
			msg := tu.Message(chatIDObj, chunk)
			c.bot.SendMessage(ctx, msg)
		}
	}

	if c.subagentTaskStore == nil {
		send("Subagent task tracking is not available.")
		return
	}

	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		send("Invalid task ID.")
		return
	}

	rootAgentID, err := c.resolveAgentUUID(ctx)
	if err != nil {
		slog.Warn("subagent callback: resolve agent UUID failed", "error", err)
		send("Subagent tasks are not available (agent could not be resolved).")
		return
	}
	task, err := c.subagentTaskStore.Get(ctx, rootAgentID, taskID)
	if err != nil {
		slog.Warn("subagent callback: Get failed", "id", taskIDStr, "error", err)
		send("Failed to load subagent task.")
		return
	}
	if task == nil {
		send(fmt.Sprintf("Task %s not found.", taskIDStr[:8]))
		return
	}
	if isDelegationCompletion(task) {
		send("This task is a delegation result and is not part of /subagents.")
		return
	}

	send(formatSubagentDetail(task))
}

// handleArchiveCallback handles the "ar:" callback prefix: ar:<taskUUID>
// archives one terminal subagent task, ar:all:<agentUUID> archives every
// completed task of the root agent. Stateless like handleSubagentCallback —
// the callback carries only UUIDs and all state is re-queried from the store.
// Unlike other prefixes the shared dispatcher does NOT pre-answer: the
// outcome toast (non-terminal rejection) needs the single allowed
// answerCallbackQuery, so this handler answers exactly once via the guarded
// answer closure (deferred empty answer dismisses the spinner on silent paths).
func (c *Channel) handleArchiveCallback(ctx context.Context, query *telego.CallbackQuery) {
	loc := normalizeTGLocale(query.From.LanguageCode)
	chat := query.Message.GetChat()
	chatID := chat.ID
	chatIDObj := tu.ID(chatID)
	chatIDStr := fmt.Sprintf("%d", chatID)
	isGroup := chat.Type == "group" || chat.Type == "supergroup"
	senderID := fmt.Sprintf("%d", query.From.ID)
	// The callback carries the thread of the pressed message — keep replies
	// (writer-gate denial, fallback confirmation) in the same forum topic.
	setThread := func(msg *telego.SendMessageParams) {
		threadID := 0
		if m, ok := query.Message.(*telego.Message); ok {
			threadID = m.MessageThreadID
		}
		if th := resolveThreadIDForSend(threadID); th > 0 {
			msg.MessageThreadID = th
		}
	}

	answered := false
	answer := func(text string, alert bool) {
		if answered {
			return
		}
		answered = true
		params := &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID, ShowAlert: alert}
		if text != "" {
			params.Text = text
		}
		if err := c.bot.AnswerCallbackQuery(ctx, params); err != nil {
			slog.Debug("archive callback: answer failed", "error", err)
		}
	}
	defer func() { answer("", false) }()

	send := func(text string) {
		for _, chunk := range chunkPlainText(text, telegramMaxMessageLen) {
			msg := tu.Message(chatIDObj, chunk)
			setThread(msg)
			c.bot.SendMessage(ctx, msg)
		}
	}

	if c.subagentTaskStore == nil {
		send("Subagent task tracking is not available.")
		return
	}

	// --- archive every completed task of the root agent ---
	if agentStr, ok := strings.CutPrefix(query.Data, "ar:all:"); ok {
		rootAgentID, err := uuid.Parse(agentStr)
		if err != nil {
			answer("", false)
			send("Invalid agent ID.")
			return
		}
		// Group chats: only file writers may archive (session_prefs gate).
		if !c.requireChatWriter(ctx, chatID, isGroup, chatIDStr, senderID, setThread) {
			return // denial message already sent by the gate
		}
		n, err := c.subagentTaskStore.ArchiveCompletedForParent(ctx, rootAgentID)
		if err != nil {
			slog.Warn("archive callback: ArchiveCompletedForParent failed", "error", err)
			send("Failed to archive subagent tasks. Please try again.")
			return
		}
		slog.Info("telegram: subagent tasks archived (all)", "count", n, "chat_id", chatIDStr, "group", isGroup)
		c.rerenderSubagentsList(ctx, query, rootAgentID, setThread, loc,
			i18n.T(loc, i18n.MsgTGSubagentArchiveAllConfirm, int(n)))
		return
	}

	// --- archive a single task ---
	taskIDStr, ok := strings.CutPrefix(query.Data, "ar:")
	if !ok {
		return // not our prefix — the router only sends ar: here
	}
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		answer("", false)
		send("Invalid task ID.")
		return
	}
	task, err := c.subagentTaskStore.GetByID(ctx, taskID)
	if err != nil {
		slog.Warn("archive callback: GetByID failed", "id", taskIDStr, "error", err)
		send("Failed to load subagent task.")
		return
	}
	if task == nil {
		answer("", false)
		send(fmt.Sprintf("Task %s not found.", taskIDStr[:8]))
		return
	}
	// Only terminal tasks (completed/failed/cancelled) may be archived.
	if !store.IsTerminalSubagentTaskStatus(task.Status) {
		answer(i18n.T(loc, i18n.MsgTGSubagentNotTerminal), true)
		return
	}
	// Group chats: only file writers may archive (session_prefs gate).
	if !c.requireChatWriter(ctx, chatID, isGroup, chatIDStr, senderID, setThread) {
		return // denial message already sent by the gate
	}
	if err := c.subagentTaskStore.ArchiveByID(ctx, taskID); err != nil {
		switch {
		case errors.Is(err, store.ErrSubagentTaskNotTerminal):
			// Status changed since the re-query (raced a still-running task).
			answer(i18n.T(loc, i18n.MsgTGSubagentNotTerminal), true)
		case errors.Is(err, store.ErrSubagentTaskNotFound):
			answer("", false)
			send(fmt.Sprintf("Task %s not found.", taskIDStr[:8]))
		default:
			slog.Warn("archive callback: ArchiveByID failed", "id", taskIDStr, "error", err)
			send("Failed to archive subagent task. Please try again.")
		}
		return
	}
	slog.Info("telegram: subagent task archived", "task_id", taskIDStr, "chat_id", chatIDStr, "group", isGroup)
	c.rerenderSubagentsList(ctx, query, task.RootAgentID, setThread, loc,
		i18n.T(loc, i18n.MsgTGSubagentArchiveDone))
}

// rerenderSubagentsList rewrites the button's own message with the refreshed
// /subagents list (archived rows are filtered out by default) headed by the
// localized archive confirmation. When the edit fails (message older than
// Telegram's edit window, deleted, identical content) it falls back to a
// fresh confirmation message so the tap never looks like a no-op.
func (c *Channel) rerenderSubagentsList(ctx context.Context, query *telego.CallbackQuery, rootAgentID uuid.UUID, setThread func(*telego.SendMessageParams), loc, confirm string) {
	chatID := query.Message.GetChat().ID
	messageID := query.Message.GetMessageID()

	text := confirm
	var markup *telego.InlineKeyboardMarkup
	tasks, err := c.subagentTaskStore.ListByParent(ctx, rootAgentID, "", false)
	if err != nil {
		slog.Warn("archive callback: ListByParent failed", "error", err)
	} else {
		body, rows := subagentsListContent(filterSelfCloneTasks(tasks), rootAgentID, loc)
		text = confirm + "\n\n" + body
		if rows == nil {
			// Explicit empty array clears the stale keyboard on the edit.
			rows = [][]telego.InlineKeyboardButton{}
		}
		markup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	}

	var editErr error
	if markup != nil {
		_, editErr = c.bot.EditMessageText(ctx, &telego.EditMessageTextParams{
			ChatID:      tu.ID(chatID),
			MessageID:   messageID,
			Text:        text,
			ReplyMarkup: markup,
		})
	} else {
		editErr = errors.New("subagent list unavailable")
	}
	if editErr != nil {
		slog.Warn("archive callback: list edit failed, sending fresh confirmation", "message_id", messageID, "error", editErr)
		msg := tu.Message(tu.ID(chatID), confirm)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
	}
}

// formatSubagentDetail formats a single subagent task for display.
func formatSubagentDetail(t *store.SubagentTaskData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Subagent: %s\n", t.Subject))
	sb.WriteString(fmt.Sprintf("ID: %s\n", t.ID.String()))
	sb.WriteString(fmt.Sprintf("Status: %s %s\n", subagentStatusIcon(t.Status), t.Status))
	if t.Model != nil && *t.Model != "" {
		sb.WriteString(fmt.Sprintf("Model: %s\n", *t.Model))
	}
	sb.WriteString(fmt.Sprintf("Depth: %d\n", t.Depth))
	sb.WriteString(fmt.Sprintf("Iterations: %d\n", t.Iterations))
	sb.WriteString(fmt.Sprintf("Tokens: %s in / %s out\n", formatTokenCount(t.InputTokens), formatTokenCount(t.OutputTokens)))
	if !t.CreatedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Created: %s\n", t.CreatedAt.Format("2006-01-02 15:04")))
	}
	if t.Description != "" {
		sb.WriteString(fmt.Sprintf("\nPrompt:\n%s\n", truncateStr(t.Description, 500)))
	}
	if t.Result != nil && *t.Result != "" {
		sb.WriteString(fmt.Sprintf("\nResult:\n%s\n", truncateStr(*t.Result, 1000)))
	}
	return sb.String()
}

func filterSelfCloneTasks(tasks []store.SubagentTaskData) []store.SubagentTaskData {
	filtered := tasks[:0]
	for i := range tasks {
		if !isDelegationCompletion(&tasks[i]) {
			filtered = append(filtered, tasks[i])
		}
	}
	return filtered
}

func isDelegationCompletion(task *store.SubagentTaskData) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	kind, _ := task.Metadata["completion_kind"].(string)
	return kind == "delegate"
}
