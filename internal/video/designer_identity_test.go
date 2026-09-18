package video

import (
	"strings"
	"testing"
)

// TestDesignerIdentityLayerContract guards the composed persona: every
// Replace in the designerIdentity builder must actually hit, or the agent
// ships without the layers/frames contract.
func TestDesignerIdentityLayerContract(t *testing.T) {
	for _, want := range []string{
		"Overlays: highlight key moments",
		`"kind": "text"|"shape"|"image"`,
		// Frames era (v5): the example storyboard leads with a composed frame.
		`"layers":[{"kind":"card"`,
		`"anim":"up"`,
		"SALE 50%",
		// Frames wire contract made it into the rules paragraph.
		`"anim": "fade"|"up"|"down"|"left"|"right"|"pop"`,
		// Polish era (v6): font hierarchy, chips, borders.
		`"font": "body"|"display"|"mono"`,
		`"chip": true`,
		`"border": true`,
		`"font":"display"`,
		"14 characters per line",
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
	if strings.Contains(designerIdentityVisuals, `"anim": "fade"`) {
		t.Error("designerIdentityVisuals must stay the pre-frames persona")
	}
	if strings.Contains(designerIdentityFrames, `"font": "body"`) {
		t.Error("designerIdentityFrames must stay the pre-polish persona")
	}
	for _, v := range designerIdentityHistory {
		if v == "" {
			t.Error("empty persona version in history")
		}
	}
}
