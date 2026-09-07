package agent

import (
	"reflect"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

func TestEstimateAdaptiveEffort(t *testing.T) {
	tests := []struct {
		name   string
		sign   AdaptiveSignals
		effort string
	}{
		{
			name:   "short greeting stays off",
			sign:   AdaptiveSignals{UserMessage: "hi"},
			effort: "off",
		},
		{
			name:   "single simple question stays off",
			sign:   AdaptiveSignals{UserMessage: "what time is it?"},
			effort: "off",
		},
		{
			name: "long message escalates to low",
			sign: AdaptiveSignals{
				UserMessage: string(repeatChar('a', 700)),
			},
			effort: "low",
		},
		{
			name: "long message with code block reaches medium",
			sign: AdaptiveSignals{
				UserMessage: string(repeatChar('a', 700)) + "\n```go\nfmt.Println(1)\n```",
			},
			effort: "medium",
		},
		{
			name: "explicit reasoning request with multiple questions reaches medium",
			sign: AdaptiveSignals{
				UserMessage: "Phân tích giúp mình? Lỗi này tại sao xảy ra?",
			},
			effort: "medium",
		},
		{
			name: "deep tool loop escalates independent of message",
			sign: AdaptiveSignals{
				UserMessage: "fix it",
				Iteration:   3,
			},
			effort: "low",
		},
		{
			name: "very deep tool loop escalates to medium",
			sign: AdaptiveSignals{
				UserMessage: "fix it",
				Iteration:   8,
			},
			effort: "medium",
		},
		{
			name: "complex request reaches high",
			sign: AdaptiveSignals{
				UserMessage: "Suy nghĩ kỹ rồi phân tích giúp tôi thiết kế lại hệ thống? Cân nhắc trade-offs? " + string(repeatChar('x', 700)),
				Iteration:   3,
			},
			effort: "high",
		},
		{
			name: "cron channel caps at low",
			sign: AdaptiveSignals{
				UserMessage: "Suy nghĩ kỹ rồi phân tích toàn bộ hệ thống? " + string(repeatChar('x', 700)),
				Iteration:   3,
				ChannelType: "cron",
			},
			effort: "low",
		},
		{
			name: "subagent channel caps at low",
			sign: AdaptiveSignals{
				UserMessage: "debug step by step carefully why this fails? and refactor? " + string(repeatChar('x', 700)),
				ChannelType: "subagent:delegate",
			},
			effort: "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateAdaptiveEffort(tt.sign)
			if got.Effort != tt.effort {
				t.Errorf("effort = %q (score %d, reasons %v), want %q",
					got.Effort, got.Score, got.Reasons, tt.effort)
			}
		})
	}
}

func TestEstimateAdaptiveEffortReasons(t *testing.T) {
	got := EstimateAdaptiveEffort(AdaptiveSignals{
		UserMessage: "```py\nprint(1)\n```",
	})
	want := []string{"code_block"}
	if !reflect.DeepEqual(got.Reasons, want) {
		t.Errorf("reasons = %v, want %v", got.Reasons, want)
	}
	if got.Score != 1 {
		t.Errorf("score = %d, want 1", got.Score)
	}
}

func TestLastUserMessage(t *testing.T) {
	msgs := []providers.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "hi"},
		{Role: "tool", Content: "tool output"},
		{Role: "user", Content: "  latest user  "},
	}
	if got := lastUserMessage(msgs); got != "  latest user  " {
		t.Errorf("lastUserMessage = %q, want %q", got, "latest user")
	}
	if got := lastUserMessage(nil); got != "" {
		t.Errorf("lastUserMessage(nil) = %q, want empty", got)
	}
}

func repeatChar(c byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return b
}
