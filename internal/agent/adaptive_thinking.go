package agent

import (
	"strings"
	"unicode/utf8"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// Adaptive thinking: per-request reasoning-effort estimation for agents whose
// reasoning effort is providers.ReasoningEffortAdaptive. The estimator is a
// deterministic heuristic over the latest user message and live run signals —
// no extra LLM call — so it stays cheap enough for constrained hosts.
//
// Effort ladder: score 0 → off, 1 → low, 2 → medium, ≥3 → high.
// Background channels (cron, subagent delegation) are capped at "low" because
// unattended jobs rarely justify deep reasoning spend.

// AdaptiveSignals carries the inputs for one effort estimation.
type AdaptiveSignals struct {
	// UserMessage is the latest user-visible message text (may be empty when
	// the run was triggered without one, e.g. cron heartbeat).
	UserMessage string
	// Iteration is the current pipeline iteration (1-based); deeper tool
	// loops hint at harder tasks and escalate the effort.
	Iteration int
	// ChannelType is the originating platform type ("telegram", "web",
	// "cron", ...). "cron" and delegation channels cap the effort at low.
	ChannelType string
}

// AdaptiveDecision is the estimator output, kept observable for logs/tests.
type AdaptiveDecision struct {
	Effort  string   `json:"effort"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons,omitempty"`
}

// adaptiveMetaKeywords are phrases (vi/en) that explicitly ask for deeper
// reasoning. Matched case-insensitively as substrings.
var adaptiveMetaKeywords = []string{
	"suy nghĩ kỹ", "suy nghĩ cẩn thận", "cân nhắc", "phân tích", "chứng minh",
	"tối ưu", "tại sao", "tình huống", "architect", "analyze", "analysis",
	"prove", "optimize", "refactor", "debug", "root cause", "step by step",
	"think hard", "think carefully", "carefully", "why", "plan",
}

// EstimateAdaptiveEffort maps run signals to a concrete provider effort.
func EstimateAdaptiveEffort(s AdaptiveSignals) AdaptiveDecision {
	msg := strings.TrimSpace(s.UserMessage)
	lower := strings.ToLower(msg)
	score := 0
	var reasons []string

	add := func(n int, reason string) {
		score += n
		reasons = append(reasons, reason)
	}

	if n := utf8.RuneCountInString(msg); n >= 1500 {
		add(2, "very_long_message")
	} else if n >= 600 {
		add(1, "long_message")
	}
	if strings.Contains(msg, "```") {
		add(1, "code_block")
	}
	if strings.Count(msg, "?") >= 2 {
		add(1, "multi_question")
	}
	for _, kw := range adaptiveMetaKeywords {
		if strings.Contains(lower, kw) {
			add(1, "explicit_reasoning_request")
			break
		}
	}
	if s.Iteration >= 8 {
		add(2, "very_deep_tool_loop")
	} else if s.Iteration >= 3 {
		add(1, "deep_tool_loop")
	}

	effort := "off"
	switch {
	case score >= 3:
		effort = "high"
	case score == 2:
		effort = "medium"
	case score == 1:
		effort = "low"
	}

	// Background jobs run unattended; cap the effort at low.
	ch := strings.ToLower(s.ChannelType)
	if ch == "cron" || strings.Contains(ch, "subagent") {
		if effortRank(effort) > effortRank("low") {
			effort = "low"
			reasons = append(reasons, "background_channel_cap")
		}
	}

	return AdaptiveDecision{Effort: effort, Score: score, Reasons: reasons}
}

// lastUserMessage returns the content of the most recent role=user message
// that carries text. Tool-result payloads are delivered as role="tool", so a
// plain user scan is enough.
func lastUserMessage(msgs []providers.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}

func effortRank(effort string) int {
	switch effort {
	case "medium":
		return 2
	case "high", "xhigh":
		return 3
	default: // off, low, none, minimal
		if effort == "low" {
			return 1
		}
		return 0
	}
}
