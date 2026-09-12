package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/mail"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- cloud mail tools: search / read / archive / unsubscribe Gmail ---

// mailArchiveBatchMax bounds mail_archive batch size (tool runs stay bounded).
const mailArchiveBatchMax = 50

// CloudMailProvider is the narrow surface the mail tools need from the cloud
// subsystem (implemented by *cloud.MailService, wired in cmd).
type CloudMailProvider interface {
	Accounts(ctx context.Context) ([]store.CloudAccount, error)
	MailClient(ctx context.Context, account string) (*mail.Client, error)
}

// CloudMailTools groups the Gmail agent tools sharing one provider.
type CloudMailTools struct {
	provider CloudMailProvider
	readCap  int // mail_read body truncation (bytes)
}

// NewCloudMailTools builds the mail toolset.
func NewCloudMailTools(provider CloudMailProvider, readCap int) *CloudMailTools {
	if readCap <= 0 {
		readCap = 8192
	}
	return &CloudMailTools{provider: provider, readCap: readCap}
}

// Tools returns the individual tools for registry registration.
func (t *CloudMailTools) Tools() []Tool {
	return []Tool{
		&cloudAccountsTool{parent: t},
		&mailSearchTool{parent: t},
		&mailReadTool{parent: t},
		&mailArchiveTool{parent: t},
		&mailUnsubscribeTool{parent: t},
	}
}

// --- cloud_accounts ---

type cloudAccountsTool struct{ parent *CloudMailTools }

func (t *cloudAccountsTool) Name() string { return "cloud_accounts" }
func (t *cloudAccountsTool) Description() string {
	return "List the user's connected cloud accounts (e.g. Gmail) with their capabilities. " +
		"Call this first when the user asks about mail or cloud files and no account was specified. " +
		"The returned email values are the `account` argument for the other cloud_* / mail_* tools."
}
func (t *cloudAccountsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
func (t *cloudAccountsTool) Execute(ctx context.Context, _ map[string]any) *Result {
	accounts, err := t.parent.provider.Accounts(ctx)
	if err != nil {
		return ErrorResult("failed to list cloud accounts: " + err.Error())
	}
	type acctOut struct {
		Email    string `json:"email"`
		Provider string `json:"provider"`
		Status   string `json:"status"`
		Mail     bool   `json:"mail"`
		Storage  bool   `json:"storage"`
	}
	out := make([]acctOut, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, acctOut{
			Email: a.Email, Provider: a.Provider, Status: a.Status,
			Mail: true, Storage: true,
		})
	}
	data, _ := json.Marshal(out)
	return NewResult(string(data))
}

// --- mail_search ---

type mailSearchTool struct{ parent *CloudMailTools }

func (t *mailSearchTool) Name() string { return "mail_search" }
func (t *mailSearchTool) Description() string {
	return "Search a connected Gmail account using Gmail search-box syntax (e.g. \"from:news@example.com is:unread\", \"newsletter newer_than:7d\"). " +
		"Returns a compact list: id, from, subject, date, snippet. Use mail_read to open a message."
}
func (t *mailSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Gmail search query (same syntax as the Gmail search box). Empty = recent mail."},
			"account":     map[string]any{"type": "string", "description": "Account email (from cloud_accounts). Optional when the user has exactly one account."},
			"max_results": map[string]any{"type": "integer", "description": "1-100, default 25."},
		},
		"required": []string{"query"},
	}
}
func (t *mailSearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	query, _ := args["query"].(string)
	account, _ := args["account"].(string)
	maxResults, _ := args["max_results"].(float64)

	client, err := t.parent.provider.MailClient(ctx, account)
	if err != nil {
		return ErrorResult(err.Error())
	}
	summaries, _, err := client.Search(ctx, query, int(maxResults), "")
	if err != nil {
		return ErrorResult("mail search failed: " + err.Error())
	}
	if summaries == nil {
		return NewResult("[]")
	}
	data, _ := json.Marshal(summaries)
	return NewResult(string(data))
}

// --- mail_read ---

type mailReadTool struct{ parent *CloudMailTools }

func (t *mailReadTool) Name() string { return "mail_read" }
func (t *mailReadTool) Description() string {
	return "Read one email by id (from mail_search). Returns from/to/subject/date, the plain-text body " +
		"(truncated), the attachment list (names only), and the List-Unsubscribe headers when present."
}
func (t *mailReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{"type": "string", "description": "Message id from mail_search."},
			"account":    map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"message_id"},
	}
}
func (t *mailReadTool) Execute(ctx context.Context, args map[string]any) *Result {
	id, _ := args["message_id"].(string)
	account, _ := args["account"].(string)
	if strings.TrimSpace(id) == "" {
		return ErrorResult("message_id is required")
	}
	client, err := t.parent.provider.MailClient(ctx, account)
	if err != nil {
		return ErrorResult(err.Error())
	}
	detail, err := client.Get(ctx, id, false)
	if err != nil {
		return ErrorResult("mail read failed: " + err.Error())
	}
	if len(detail.Body) > t.parent.readCap {
		detail.Body = detail.Body[:t.parent.readCap] + "\n…[truncated]"
	}
	data, _ := json.Marshal(detail)
	return NewResult(string(data))
}

// --- mail_archive ---

type mailArchiveTool struct{ parent *CloudMailTools }

func (t *mailArchiveTool) Name() string { return "mail_archive" }
func (t *mailArchiveTool) Description() string {
	return "Apply label actions to emails: archive (remove from INBOX), trash, untrash, mark_read, " +
		"add_label, remove_label. Accepts up to 50 message ids per call. " +
		"There is NO permanent delete — trash is recoverable. " +
		"When the user asks to 'clean up' or 'get rid of' mail, confirm the selection with them (ask_options) " +
		"before archiving/trashing more than a handful of messages."
}
func (t *mailArchiveTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Message ids to act on (max 50).",
			},
			"action": map[string]any{
				"type": "string",
				"enum": []string{"archive", "trash", "untrash", "mark_read", "add_label", "remove_label"},
			},
			"label_name": map[string]any{"type": "string", "description": "User label name (required for add_label/remove_label)."},
			"account":    map[string]any{"type": "string", "description": "Account email (optional with one account)."},
		},
		"required": []string{"message_ids", "action"},
	}
}
func (t *mailArchiveTool) Execute(ctx context.Context, args map[string]any) *Result {
	rawIDs, _ := args["message_ids"].([]any)
	action, _ := args["action"].(string)
	labelName, _ := args["label_name"].(string)
	account, _ := args["account"].(string)

	ids := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		if id, ok := raw.(string); ok && strings.TrimSpace(id) != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ErrorResult("message_ids must contain at least one id")
	}
	if len(ids) > mailArchiveBatchMax {
		return ErrorResult(fmt.Sprintf("too many message ids: %d (max %d) — split into multiple calls", len(ids), mailArchiveBatchMax))
	}

	add, remove, err := mailActionLabels(action, labelName)
	if err != nil {
		return ErrorResult(err.Error())
	}

	client, err := t.parent.provider.MailClient(ctx, account)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Resolve a user label name to its id (system labels like INBOX/TRASH/UNREAD
	// are already ids).
	if action == "add_label" || action == "remove_label" {
		if labels, lerr := client.ListLabels(ctx); lerr == nil {
			for _, l := range labels {
				if strings.EqualFold(l["name"], labelName) {
					labelName = l["id"]
					break
				}
			}
		}
	}
	for i := range add {
		if add[i] == "__LABEL__" {
			add[i] = labelName
		}
	}
	for i := range remove {
		if remove[i] == "__LABEL__" {
			remove[i] = labelName
		}
	}

	failed := 0
	for _, id := range ids {
		if err := client.Modify(ctx, id, add, remove); err != nil {
			failed++
		}
	}
	if failed == len(ids) {
		return ErrorResult("all modifications failed (check account status)")
	}
	return NewResult(fmt.Sprintf("modified %d/%d messages (action=%s)", len(ids)-failed, len(ids), action))
}

// mailActionLabels maps the action enum to Gmail label operations. The
// __LABEL__ sentinel is substituted with the resolved label id afterwards.
// Deliberately NO mapping to messages.delete/batchDelete exists.
func mailActionLabels(action, labelName string) (add, remove []string, err error) {
	switch action {
	case "archive":
		return nil, []string{"INBOX"}, nil
	case "trash":
		return []string{"TRASH"}, nil, nil
	case "untrash":
		return nil, []string{"TRASH"}, nil
	case "mark_read":
		return nil, []string{"UNREAD"}, nil
	case "add_label":
		if strings.TrimSpace(labelName) == "" {
			return nil, nil, fmt.Errorf("label_name is required for add_label")
		}
		return []string{"__LABEL__"}, nil, nil
	case "remove_label":
		if strings.TrimSpace(labelName) == "" {
			return nil, nil, fmt.Errorf("label_name is required for remove_label")
		}
		return nil, []string{"__LABEL__"}, nil
	default:
		return nil, nil, fmt.Errorf("unknown action %q", action)
	}
}

// --- mail_unsubscribe ---

type mailUnsubscribeTool struct{ parent *CloudMailTools }

func (t *mailUnsubscribeTool) Name() string { return "mail_unsubscribe" }
func (t *mailUnsubscribeTool) Description() string {
	return "Analyze or execute email unsubscription for a newsletter. " +
		"Call WITHOUT execute=true first: it returns a plan (mechanism + target) and performs nothing. " +
		"Always present the plan and get explicit user consent (ask_options) BEFORE calling with execute=true. " +
		"Executing a one-click (RFC 8058) unsubscribe POSTs to the sender; the user must have agreed. " +
		"mailto-only senders cannot be unsubscribed automatically — inform the user instead."
}
func (t *mailUnsubscribeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{"type": "string", "description": "Message id from mail_search."},
			"account":    map[string]any{"type": "string", "description": "Account email (optional with one account)."},
			"execute":    map[string]any{"type": "boolean", "description": "false (default) = analyze only; true = perform the unsubscribe (requires prior user consent)."},
		},
		"required": []string{"message_id"},
	}
}
func (t *mailUnsubscribeTool) Execute(ctx context.Context, args map[string]any) *Result {
	id, _ := args["message_id"].(string)
	account, _ := args["account"].(string)
	execute, _ := args["execute"].(bool)
	if strings.TrimSpace(id) == "" {
		return ErrorResult("message_id is required")
	}

	client, err := t.parent.provider.MailClient(ctx, account)
	if err != nil {
		return ErrorResult(err.Error())
	}
	detail, err := client.Get(ctx, id, true)
	if err != nil {
		return ErrorResult("mail read failed: " + err.Error())
	}
	plan, err := mail.AnalyzeUnsubscribe(detail)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if !execute {
		data, _ := json.Marshal(map[string]any{
			"plan":        plan,
			"from":        detail.From,
			"subject":     detail.Subject,
			"executed":    false,
			"next_action": "present this plan to the user and ask for consent before execute=true",
		})
		return NewResult(string(data))
	}

	// Consent gate: the tool description + system prompt demand prior consent.
	// The execution itself is logged at warn level (security.mail_unsubscribe).
	httpClient := &http.Client{Timeout: 20 * time.Second}
	if err := mail.ExecuteUnsubscribe(ctx, httpClient, plan); err != nil {
		return ErrorResult("unsubscribe failed: " + err.Error())
	}
	data, _ := json.Marshal(map[string]any{
		"executed":  true,
		"mechanism": plan.Mechanism,
		"url":       plan.URL,
		"note":      "unsubscribe request sent; also archive/trash the message if the user wants",
	})
	return NewResult(string(data))
}
