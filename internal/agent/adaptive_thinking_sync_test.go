package agent

import "testing"

// highScoreMessage is a message that reliably scores >= 3 (meta keywords +
// multi-question + length) so the effort would be "high" without channel caps.
var highScoreMessage = "Suy nghĩ kỹ rồi phân tích giúp tôi thiết kế lại hệ thống? Cân nhắc trade-offs? " +
	"Phân tích root cause từng bước? " + string(repeatChar('x', 700))

// TestEstimateAdaptiveEffort_SyncDelegationUncapped is the Phase 7 narrowing:
// ASYNC/background delegation channels (cron, subagent:*) stay capped at low,
// while a channel type carrying the "sync" marker keeps the agent-level effort
// because its result returns straight into the caller's conversation.
func TestEstimateAdaptiveEffort_SyncDelegationUncapped(t *testing.T) {
	tests := []struct {
		name        string
		channelType string
		wantEffort  string
		wantCap     bool
	}{
		{"sync delegation keeps agent-level high", "subagent:sync", "high", false},
		{"async delegation still capped at low", "subagent:delegate", "low", true},
		{"async subagent lane still capped at low", "subagent", "low", true},
		{"cron still capped at low", "cron", "low", true},
		{"plain chat channel uncapped (never capped before or after)", "telegram", "high", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateAdaptiveEffort(AdaptiveSignals{
				UserMessage: highScoreMessage,
				Iteration:   3,
				ChannelType: tt.channelType,
			})
			if got.Effort != tt.wantEffort {
				t.Errorf("effort = %q (score %d, reasons %v), want %q",
					got.Effort, got.Score, got.Reasons, tt.wantEffort)
			}
			capped := false
			for _, r := range got.Reasons {
				if r == "background_channel_cap" {
					capped = true
				}
			}
			if capped != tt.wantCap {
				t.Errorf("background_channel_cap present = %v, want %v (reasons %v)", capped, tt.wantCap, got.Reasons)
			}
		})
	}
}
