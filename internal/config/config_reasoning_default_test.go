package config

import "testing"

// TestNormalizeReasoningDefault locks the agents.reasoning_default vocabulary
// (Phase 7): empty resolves to the "auto" default, the documented values pass
// through case/whitespace-insensitively, and anything unknown degrades to
// "inherit" so a config typo never silently enables paid reasoning on new
// agents.
func TestNormalizeReasoningDefault(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", DefaultReasoningEffort}, // default "auto"
		{"inherit", "inherit"},
		{"off", "off"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"auto", "auto"},
		{"  Auto  ", "auto"},
		{"OFF", "off"},
		{"banana", "inherit"},
		{"adaptive", "inherit"}, // not part of this vocabulary
	}
	for _, tt := range cases {
		if got := NormalizeReasoningDefault(tt.in); got != tt.want {
			t.Errorf("NormalizeReasoningDefault(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestAgentReasoningDefault proves the accessor: Default() config and a
// zero-value config both resolve to "auto"; an explicit "inherit" (or unknown
// value) means "stamp nothing" at creation time.
func TestAgentReasoningDefault(t *testing.T) {
	if got := Default().AgentReasoningDefault(); got != "auto" {
		t.Errorf("Default().AgentReasoningDefault() = %q, want auto", got)
	}

	var zero Config
	if got := zero.AgentReasoningDefault(); got != "auto" {
		t.Errorf("zero Config.AgentReasoningDefault() = %q, want auto (empty resolves to default)", got)
	}

	zero.Agents.ReasoningDefault = "inherit"
	if got := zero.AgentReasoningDefault(); got != "inherit" {
		t.Errorf("explicit inherit = %q, want inherit", got)
	}

	zero.Agents.ReasoningDefault = "medium"
	if got := zero.AgentReasoningDefault(); got != "medium" {
		t.Errorf("explicit medium = %q, want medium", got)
	}
}
