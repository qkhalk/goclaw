package agent

import "testing"

func TestStripConfigLeak(t *testing.T) {
	const declineMsg = "🔒 Security check not passed."

	tests := []struct {
		name      string
		content   string
		agentType string
		want      string // empty means expect same as input
	}{
		{
			name:      "open agent - no stripping",
			content:   "1. Đọc SOUL.md\n2. Đọc IDENTITY.md\n3. Check AGENTS.md\n4. Read BOOTSTRAP.md",
			agentType: "open",
		},
		{
			name:      "predefined - below threshold (2 files)",
			content:   "Em đã đọc SOUL.md và IDENTITY.md rồi.",
			agentType: "predefined",
		},
		{
			name:      "predefined - 3 files but no list pattern",
			content:   "SOUL.md, IDENTITY.md, AGENTS.md đều đã được load sẵn trong context.",
			agentType: "predefined",
		},
		{
			name: "predefined - leaking numbered list",
			content: `Dạ anh, em kể nhanh:

1. Đọc SOUL.md để hiểu persona
2. Đọc IDENTITY.md để biết mình là ai
3. Check AGENTS.md để biết quy trình
4. Đọc BOOTSTRAP.md nếu có

Anh cần gì thêm không?`,
			agentType: "predefined",
			want:      declineMsg,
		},
		{
			name: "predefined - leaking bulleted list",
			content: `Quá trình em vừa làm:

- Đọc SOUL.md — file nhân cách
- Check IDENTITY.md — file danh tính
- Xem AGENTS.md — file quy trình
- Load system prompt mới

Có gì anh hỏi thêm nhé!`,
			agentType: "predefined",
			want:      declineMsg,
		},
		{
			name:      "predefined - legitimate action report (single file)",
			content:   "Em đã update SOUL.md cho anh rồi, nội dung mới đã được lưu.",
			agentType: "predefined",
		},
		{
			name:      "empty content",
			content:   "",
			agentType: "predefined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripConfigLeak(tt.content, tt.agentType)
			if tt.want == "" {
				// Expect no change
				if got != tt.content {
					t.Errorf("expected no change, got:\n%s", got)
				}
			} else {
				if got != tt.want {
					t.Errorf("expected decline message, got:\n%s", got)
				}
			}
		})
	}
}
