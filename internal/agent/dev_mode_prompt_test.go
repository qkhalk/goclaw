package agent

import (
	"strings"
	"testing"
)

func TestApplyDevMode(t *testing.T) {
	const marker = "DEV MODE ACTIVE"

	if got := ApplyDevMode(false, "existing"); got != "existing" {
		t.Errorf("disabled mode must pass extra through unchanged, got %q", got)
	}
	if got := ApplyDevMode(true, ""); got != DevModePromptSection {
		t.Errorf("empty extra should yield just the section, got %q", got)
	}
	got := ApplyDevMode(true, "existing")
	if !strings.HasPrefix(got, DevModePromptSection) {
		t.Errorf("dev section must be prepended, got %q", got)
	}
	if !strings.HasSuffix(got, "existing") {
		t.Errorf("existing extra must be preserved at the end, got %q", got)
	}
	if !strings.Contains(got, marker) {
		t.Errorf("marker missing, got %q", got)
	}
}
