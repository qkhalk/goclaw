package mcpcatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// TestToolFoldersWellFormed guards catalog quality at test time (i.e. before
// a release tag ships it): every mcp/<tool>/ folder must have a valid
// manifest whose Name matches the folder, an existing entry file and a
// README, and the embedded catalog must match the folder set exactly — a
// folder without a manifest would be silently invisible in the Store.
func TestToolFoldersWellFormed(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read mcp/: %v", err)
	}

	folders := map[string]string{} // name -> manifest path
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "templates" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		manifestPath := filepath.Join(e.Name(), "manifest.json")
		if _, err := os.Stat(manifestPath); err != nil {
			t.Errorf("mcp/%s/ has no manifest.json — it would be invisible in the Store", e.Name())
			continue
		}
		folders[e.Name()] = manifestPath
	}

	embedded := map[string]bool{}
	for _, m := range Entries() {
		embedded[m.Name] = true
		dir := m.Name
		// The embedded manifest must equal the on-disk one (embed drift
		// would serve stale metadata).
		raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			t.Errorf("embedded tool %s missing on disk: %v", m.Name, err)
			continue
		}
		var onDisk Manifest
		if err := json.Unmarshal(raw, &onDisk); err != nil {
			t.Errorf("mcp/%s/manifest.json unparseable: %v", dir, err)
			continue
		}
		if onDisk != m {
			t.Errorf("mcp/%s/manifest.json drifts from the embedded copy", dir)
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(m.Entry))); err != nil {
			t.Errorf("mcp/%s/: entry %s not found", dir, m.Entry)
		}
		if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
			t.Errorf("mcp/%s/: README.md required (contributors need the dev/ship doc)", dir)
		}
		if pj, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil && m.Runtime == "node" {
			var pkg struct {
				Dependencies    map[string]string `json:"dependencies"`
				DevDependencies map[string]string `json:"devDependencies"`
			}
			if err := json.Unmarshal(pj, &pkg); err != nil {
				t.Errorf("mcp/%s/package.json unparseable: %v", dir, err)
			} else if len(pkg.Dependencies) > 0 || len(pkg.DevDependencies) > 0 {
				t.Errorf("mcp/%s/: catalog tools must be zero-dependency (installer supports deps, but curated ones stay supply-chain-free)", dir)
			}
		}
	}

	for name := range folders {
		if !embedded[name] {
			t.Errorf("mcp/%s/ has a manifest but is not embedded — check go:embed patterns", name)
		}
	}
	for _, m := range Entries() {
		if _, ok := folders[m.Name]; !ok {
			t.Errorf("embedded tool %s has no mcp/%s/ folder", m.Name, m.Name)
		}
	}
	if len(Entries()) == 0 {
		t.Error("catalog is empty — the Store MCP section would render nothing")
	}
}
