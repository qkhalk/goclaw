package providers

// collapseToolCallsWithoutSig strips tool_call cycles that lack thought_signature
// (required by Gemini 2.5+). Old session history stored before the thought_signature
// capture fix doesn't have it, and Gemini rejects those messages with HTTP 400.
//
// The assistant's original text content (if any) is preserved; only the tool_calls
// and their corresponding tool-result messages are dropped.
func collapseToolCallsWithoutSig(msgs []Message) []Message {
	// Collect tool_call IDs that need collapsing.
	collapseIDs := make(map[string]bool)
	for _, m := range msgs {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.Metadata["thought_signature"] == "" {
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

		// Strip tool_calls from assistant message, keep original content only.
		if m.Role == "assistant" && len(m.ToolCalls) > 0 && collapseIDs[m.ToolCalls[0].ID] {
			if m.Content != "" {
				result = append(result, Message{
					Role:    "assistant",
					Content: m.Content,
				})
			}

			// Skip consecutive tool results belonging to these tool_calls.
			for i+1 < len(msgs) && msgs[i+1].Role == "tool" && collapseIDs[msgs[i+1].ToolCallID] {
				i++
			}
			continue
		}

		// Skip orphaned tool results whose assistant was already collapsed.
		if m.Role == "tool" && collapseIDs[m.ToolCallID] {
			continue
		}

		result = append(result, m)
	}
	return result
}
