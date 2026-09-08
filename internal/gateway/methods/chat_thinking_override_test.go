package methods

import "testing"

// TestThinkingOverrideFor proves the chat.send thinkingLevel param accepts
// "adaptive" verbatim, the standard effort levels in any case, and rejects
// unknown values to "" (agent config applies).
func TestThinkingOverrideFor(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"adaptive", "adaptive"},
		{"off", "off"},
		{"low", "low"},
		{"HIGH", "high"},
		{"xhigh", "xhigh"},
		{"minimal", "minimal"},
		{"auto", "auto"},
		{"none", "none"},
		{" bogus ", ""},
		{"turbo", ""},
	}
	for _, tc := range cases {
		if got := thinkingOverrideFor(tc.in); got != tc.want {
			t.Fatalf("thinkingOverrideFor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
