package video

import (
	"strings"
	"testing"
)

// TestDesignerIdentityLayerContract guards the composed persona: every
// Replace in the designerIdentity builder must actually hit, or the agent
// ships without the layers contract.
func TestDesignerIdentityLayerContract(t *testing.T) {
	for _, want := range []string{
		"Overlays: highlight key moments",
		`"kind": "text"|"shape"|"image"`,
		`"layers":[{"kind":"text"`,
		"SALE 50%",
	} {
		if !strings.Contains(designerIdentity, want) {
			t.Errorf("designerIdentity missing layers contract fragment: %q", want)
		}
	}
	// The migration source must NOT contain the additions (byte-for-byte
	// matching of pre-layers deployments depends on it).
	if strings.Contains(designerIdentityV3, "Overlays: highlight") {
		t.Error("designerIdentityV3 must stay the pre-layers persona")
	}
	for _, v := range designerIdentityHistory {
		if v == "" {
			t.Error("empty persona version in history")
		}
	}
}
