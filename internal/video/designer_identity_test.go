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
		// Transitions era (v7): valid transition enum in the motion bullet.
		`"fade", "crossfade", "slide_left", "slide_up"`,
		// Caption-zone era (v8): centered caption vs composed-frame content.
		"fills y = 0.36-0.64",
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
	if strings.Contains(designerIdentityPolish, `"slide_left"`) {
		t.Error("designerIdentityPolish must stay the pre-transitions persona")
	}
	if strings.Contains(designerIdentityTransitions, "fills y = 0.36-0.64") {
		t.Error("designerIdentityTransitions must stay the pre-caption-zone persona")
	}
	// v6 must sit in the history so v6 deployments migrate to v7.
	foundPolish := false
	for _, v := range designerIdentityHistory {
		if v == designerIdentityPolish {
			foundPolish = true
		}
	}
	if !foundPolish {
		t.Error("designerIdentityPolish missing from history — v6 deployments would never migrate")
	}
	for _, v := range designerIdentityHistory {
		if v == "" {
			t.Error("empty persona version in history")
		}
	}
}
