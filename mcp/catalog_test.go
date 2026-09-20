package mcpcatalog

import (
	"strings"
	"testing"
)

// Entries must parse every embedded manifest and validate it — a malformed
// manifest ships with the binary, so this failing at test time is the
// cheapest place to catch it.
func TestEntries_ParseAndValidate(t *testing.T) {
	entries := Entries()
	if len(entries) == 0 {
		t.Fatal("catalog embeds no manifests")
	}
	seen := map[string]bool{}
	for _, m := range entries {
		if seen[m.Name] {
			t.Errorf("duplicate manifest name %q", m.Name)
		}
		seen[m.Name] = true
		if err := m.Validate(); err != nil {
			t.Errorf("manifest %s invalid: %v", m.Name, err)
		}
	}
	// Sorted by name for stable API output.
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Name > entries[i].Name {
			t.Errorf("entries not sorted: %s before %s", entries[i-1].Name, entries[i].Name)
		}
	}
}

func TestFind(t *testing.T) {
	if Find("media-probe") == nil {
		t.Fatal("media-probe manifest not embedded — check mcp/media-probe/manifest.json")
	}
	if Find("does-not-exist") != nil {
		t.Fatal("Find returned a manifest for an unknown name")
	}
}

func TestManifestValidate_Rejects(t *testing.T) {
	cases := []Manifest{
		{Name: "", DisplayName: "d", Description: "x", Runtime: "node", Entry: "a.js"},
		{Name: "has space", DisplayName: "d", Description: "x", Runtime: "node", Entry: "a.js"},
		{Name: "ok", DisplayName: "", Description: "x", Runtime: "node", Entry: "a.js"},
		{Name: "ok", DisplayName: "d", Description: "", Runtime: "node", Entry: "a.js"},
		{Name: "ok", DisplayName: "d", Description: "x", Runtime: "deno", Entry: "a.js"},
		{Name: "ok", DisplayName: "d", Description: "x", Runtime: "node", Entry: ""},
		{Name: "ok", DisplayName: "d", Description: "x", Runtime: "node", Entry: "/abs/path.js"},
		{Name: "ok", DisplayName: "d", Description: "x", Runtime: "node", Entry: "../escape.js"},
	}
	for i, m := range cases {
		if err := m.Validate(); err == nil {
			t.Errorf("case %d (%s): expected error, got nil", i, m.Name)
		} else if strings.TrimSpace(err.Error()) == "" {
			t.Errorf("case %d: empty error message", i)
		}
	}
}
