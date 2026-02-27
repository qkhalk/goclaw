package providers

import (
	"encoding/json"
	"fmt"
	"strings"
)

// collapseToolCallsWithoutSig rewrites assistant messages whose tool_calls lack
// thought_signature (required by Gemini 2.5+) into plain text. This handles old
// session history stored before the thought_signature capture fix.
// Tool call cycles (assistant + tool results) are collapsed into:
//   - assistant: original content + "[Called tool_name({args})]"
//   - user: "[tool_name result]: content"
func collapseToolCallsWithoutSig(msgs []Message) []Message {
	// Collect tool_call IDs that need collapsing.
	collapseIDs := make(map[string]bool)
	for _, m := range msgs {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.Metadata["thought_signature"] == "" {
				// If any tool_call in the message is missing sig, collapse all of them.
				for _, tc2 := range m.ToolCalls {
					collapseIDs[tc2.ID] = true
				}
				break
			}
		}
	}
	if len(collapseIDs) == 0 {
		return msgs
	}

	result := make([]Message, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]

		// Collapse assistant with tool_calls → plain text assistant
		if m.Role == "assistant" && len(m.ToolCalls) > 0 && collapseIDs[m.ToolCalls[0].ID] {
			var sb strings.Builder
			if m.Content != "" {
				sb.WriteString(m.Content)
				sb.WriteString("\n\n")
			}
			for _, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				fmt.Fprintf(&sb, "[Called %s(%s)]\n", tc.Name, string(argsJSON))
			}
			result = append(result, Message{
				Role:    "assistant",
				Content: strings.TrimSpace(sb.String()),
			})

			// Collect consecutive tool results → one user message
			var toolBuf strings.Builder
			for i+1 < len(msgs) && msgs[i+1].Role == "tool" && collapseIDs[msgs[i+1].ToolCallID] {
				i++
				toolMsg := msgs[i]
				toolName := toolMsg.ToolCallID
				for _, tc := range m.ToolCalls {
					if tc.ID == toolMsg.ToolCallID {
						toolName = tc.Name
						break
					}
				}
				if toolBuf.Len() > 0 {
					toolBuf.WriteString("\n\n")
				}
				fmt.Fprintf(&toolBuf, "[%s result]: %s", toolName, toolMsg.Content)
			}
			if toolBuf.Len() > 0 {
				result = append(result, Message{
					Role:    "user",
					Content: toolBuf.String(),
				})
			}
			continue
		}

		// Skip orphaned tool results whose assistant was already collapsed
		if m.Role == "tool" && collapseIDs[m.ToolCallID] {
			continue
		}

		result = append(result, m)
	}
	return result
}
