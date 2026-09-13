package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

// --- ask_options: agent asks the user a clarifying question with buttons ---

// askOptionsMax is the maximum number of option buttons (Telegram rows of 2
// plus a dedicated "Other" row keep the keyboard compact).
const askOptionsMax = 4

// askOptionsLabelMax truncates button labels (Telegram caps button text at
// 64 chars; we stay well under so two fit per row).
const askOptionsLabelMax = 48

// MetaAskOptions is the OutboundMessage.Metadata key carrying the button
// labels as a JSON array. The Telegram channel Send path renders the inline
// keyboard from it (metadata convention, precedent: placeholder_update).
const MetaAskOptions = "ask_options"

// MetaOutboundLocalKey mirrors the channel-side "local_key" outbound metadata
// key (send.go localKey lookup) — declared here to avoid importing the
// telegram channel package.
const MetaOutboundLocalKey = "local_key"

// AskOptionsTool lets the agent ask the user a clarifying question with 1-4
// tappable options plus an "Other" free-text path. Telegram-only in v1: the
// channel attaches an inline keyboard to the question and routes button
// presses and replies back into the session as user turns.
type AskOptionsTool struct {
	msgBus *bus.MessageBus
}

// NewAskOptionsTool builds the tool; wire the message bus via SetMessageBus.
func NewAskOptionsTool() *AskOptionsTool { return &AskOptionsTool{} }

func (t *AskOptionsTool) SetMessageBus(b *bus.MessageBus) { t.msgBus = b }

func (t *AskOptionsTool) Name() string { return "ask_options" }

func (t *AskOptionsTool) Description() string {
	return "Ask the user a clarifying question with tappable option buttons (Telegram and web chat). " +
		"Use when the request is ambiguous and 2-4 distinct interpretations exist, or when a key " +
		"decision (scope, target, approach) must be confirmed before proceeding. " +
		"The question is sent to the chat with one button per option plus an Other button for free-text. " +
		"After calling this tool, END YOUR TURN and wait for the user's reply — the answer arrives " +
		"as their next message in this session."
}

func (t *AskOptionsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The question to show the user. One short, specific question — no preamble.",
			},
			"options": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"minItems":    1,
				"maxItems":    askOptionsMax,
				"description": "1-4 mutually exclusive answer options, each a short button label (<=48 chars).",
			},
		},
		"required": []string{"question", "options"},
	}
}

func (t *AskOptionsTool) Execute(ctx context.Context, args map[string]any) *Result {
	question, _ := args["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return ErrorResult("question is required")
	}

	rawOptions, _ := args["options"].([]any)
	if len(rawOptions) == 0 {
		return ErrorResult("options must contain 1-4 choices")
	}
	if len(rawOptions) > askOptionsMax {
		return ErrorResult(fmt.Sprintf("too many options: %d (max %d)", len(rawOptions), askOptionsMax))
	}
	options := make([]string, 0, len(rawOptions))
	seen := make(map[string]struct{}, len(rawOptions))
	for _, raw := range rawOptions {
		label, _ := raw.(string)
		label = strings.TrimSpace(label)
		if label == "" {
			return ErrorResult("options must not contain empty labels")
		}
		if len(label) > askOptionsLabelMax {
			return ErrorResult(fmt.Sprintf("option too long (max %d chars): %q", askOptionsLabelMax, label))
		}
		if _, dup := seen[label]; dup {
			return ErrorResult(fmt.Sprintf("duplicate option: %q", label))
		}
		seen[label] = struct{}{}
		options = append(options, label)
	}

	channel := ToolChannelFromCtx(ctx)
	chatID := ToolChatIDFromCtx(ctx)
	if channel == "" || chatID == "" || channel == ChannelTeammate || channel == ChannelSystem || channel == ChannelDashboard {
		return ErrorResult("ask_options is only available in an active user chat; no channel/chat in this context")
	}

	switch channel {
	case "telegram":
		if t.msgBus == nil {
			return ErrorResult("ask_options: message bus unavailable")
		}
		// Prefer the composite local key (e.g. "-100123:topic:42"): the Telegram
		// channel's Send path parses thread routing + the placeholder key from it.
		// With the bare chat ID the question would land in the General topic.
		target := ToolLocalKeyFromCtx(ctx)
		metadata := map[string]string{MetaAskOptions: string(mustJSON(options))}
		if target != "" {
			metadata[MetaOutboundLocalKey] = target
			if idx := strings.Index(target, ":topic:"); idx > 0 {
				metadata[MetaMessageThreadID] = target[idx+len(":topic:"):]
			} else if idx := strings.Index(target, ":thread:"); idx > 0 {
				metadata[MetaMessageThreadID] = target[idx+len(":thread:"):]
			}
		} else {
			target = chatID
		}
		t.msgBus.PublishOutbound(bus.OutboundMessage{
			Channel:  channel,
			ChatID:   target,
			Content:  "❓ " + question,
			Metadata: metadata,
		})
	case ChannelWeb:
		// The web chat renders the interactive question card from this tool
		// call's arguments (already carried by the tool.result event) and
		// injects the picked option back as the next user message — no
		// outbound publish needed here.
	default:
		return ErrorResult(fmt.Sprintf("ask_options is not supported on channel %q (telegram and web chat only)", channel))
	}
	return NewResult("Question sent to the user with option buttons. End your turn now and wait for their reply — " +
		"their answer (button press or typed reply) will arrive as the next user message in this session.")
}

// mustJSON marshals option labels; the inputs are validated strings so the
// error path is unreachable — kept for signature honesty.
func mustJSON(v []string) []byte {
	encoded, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return encoded
}
