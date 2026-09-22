package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerate_Node(t *testing.T) {
	out := t.TempDir()
	w, err := Generate(Options{
		Name:        "demo-node",
		Description: "test tool",
		Category:    "media",
		Runtime:     "node",
		OutRoot:     out,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, want := range []string{"manifest.json", "package.json", "src/index.js", "test/smoke.js", "README.md"} {
		p := filepath.Join(w.Dir, want)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("generated file missing: %s", want)
		}
	}
	raw, err := os.ReadFile(filepath.Join(w.Dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Runtime     string `json:"runtime"`
		Entry       string `json:"entry"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest unparseable: %v", err)
	}
	if m.Name != "demo-node" || m.DisplayName != "Demo Node" || m.Runtime != "node" || m.Entry != "src/index.js" {
		t.Errorf("manifest wrong: %+v", m)
	}
}

func TestGenerate_Python(t *testing.T) {
	out := t.TempDir()
	w, err := Generate(Options{Name: "demo-py", Runtime: "python", OutRoot: out})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, want := range []string{"manifest.json", "src/server.py", "test/smoke.py", "README.md"} {
		if _, err := os.Stat(filepath.Join(w.Dir, want)); err != nil {
			t.Errorf("generated file missing: %s", want)
		}
	}
	// Node-only files must not leak into python tools.
	if _, err := os.Stat(filepath.Join(w.Dir, "package.json")); err == nil {
		t.Error("python tool must not get a package.json")
	}
}

func TestGenerate_RejectsDuplicateAndBadSlug(t *testing.T) {
	out := t.TempDir()
	if _, err := Generate(Options{Name: "ok-tool", OutRoot: out}); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{Name: "ok-tool", OutRoot: out}); err == nil {
		t.Error("Generate over an existing folder must fail")
	}
	for _, bad := range []string{"", "-lead", "has space", "a/b", "../esc"} {
		if _, err := Generate(Options{Name: bad, OutRoot: out}); err == nil {
			t.Errorf("Generate(%q) must fail", bad)
		}
	}
}
