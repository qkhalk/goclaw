// Package scaffold generates new MCP tool-server folders for the catalog
// (mcp/<name>/): a valid manifest, a working zero-dependency stdio server
// skeleton with one example tool, a README, and a runnable smoke test.
//
// `goclaw mcp new <name>` is the entry point. Generated tools are
// self-contained on purpose — each installs and ships independently, so the
// JSON-RPC plumbing is inlined into every tool rather than shared.
package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

//go:embed templates
var templates embed.FS

// Options describes one tool to generate.
type Options struct {
	// Name is the tool slug (mcp/<name>/). Lowercase [a-z0-9._-], not empty,
	// no leading dash.
	Name string
	// DisplayName defaults to a title-cased Name ("media-utils" → "Media Utils").
	DisplayName string
	// Description goes into the manifest and README; keep it one sentence.
	Description string
	// Category groups the Store card (default "tools").
	Category string
	// Runtime is "node" or "python".
	Runtime string
	// OutRoot is the mcp/ folder to create <Name>/ under (usually the repo
	// root's mcp/; tests pass a temp dir).
	OutRoot string
}

// Written describes the generated files plus how to smoke-test them.
type Written struct {
	Dir      string
	Files    []string
	SmokeCmd []string // argv to run the smoke test, e.g. ["node", "test/smoke.js"]
}

// entryFile is the server entry path per runtime.
func entryFile(runtime string) string {
	if runtime == "python" {
		return "src/server.py"
	}
	return "src/index.js"
}

// Validate normalises and checks the options before any file is written.
func (o *Options) Validate() error {
	o.Name = strings.ToLower(strings.TrimSpace(o.Name))
	if o.Name == "" || len(o.Name) > 64 {
		return fmt.Errorf("tool name must be 1-64 characters")
	}
	if strings.HasPrefix(o.Name, "-") || strings.ContainsAny(o.Name, " /\\") {
		return fmt.Errorf("tool name must be a slug (a-z, 0-9, dash, underscore, dot)")
	}
	if o.Runtime == "" {
		o.Runtime = "node"
	}
	if o.Runtime != "node" && o.Runtime != "python" {
		return fmt.Errorf("runtime must be node or python (got %q)", o.Runtime)
	}
	if o.DisplayName == "" {
		o.DisplayName = titleFromSlug(o.Name)
	}
	if o.Description == "" {
		o.Description = o.DisplayName + " — MCP tool server for agents."
	}
	if o.Category == "" {
		o.Category = "tools"
	}
	if o.OutRoot == "" {
		return fmt.Errorf("output root (mcp/ folder) is required")
	}
	return nil
}

// Generate writes the tool folder. It never overwrites an existing tool.
func Generate(o Options) (*Written, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	dir := filepath.Join(o.OutRoot, o.Name)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("%s already exists — pick another name", dir)
	}

	subdir := "node"
	if o.Runtime == "python" {
		subdir = "python"
	}
	data := map[string]string{
		"Name":        o.Name,
		"DisplayName": o.DisplayName,
		"Description": o.Description,
		"Category":    o.Category,
		"Entry":       entryFile(o.Runtime),
		"Runtime":     o.Runtime,
	}

	tmplNames, err := fsList("templates/" + subdir)
	if err != nil {
		return nil, err
	}
	w := &Written{Dir: dir, SmokeCmd: smokeCmd(o.Runtime)}
	for _, name := range tmplNames {
		raw, err := templates.ReadFile("templates/" + subdir + "/" + name)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", name, err)
		}
		tmpl, err := template.New(name).Parse(string(raw))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		// Strip ".tmpl"; "index.js.tmpl" → "index.js", and put templates in
		// their real locations ("src/index.js.tmpl" → "src/index.js").
		target := filepath.Join(dir, strings.TrimSuffix(name, ".tmpl"))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		err = tmpl.Execute(f, data)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", name, err)
		}
		if o.Runtime == "python" && strings.HasSuffix(target, ".py") {
			// Keep the executable bit meaningful on unix hosts.
			_ = os.Chmod(target, 0o755)
		}
		w.Files = append(w.Files, filepath.ToSlash(strings.TrimPrefix(filepath.ToSlash(target), filepath.ToSlash(o.OutRoot)+"/")))
	}
	sort.Strings(w.Files)
	return w, nil
}

// smokeCmd returns the smoke-test argv relative to the tool dir.
func smokeCmd(runtime string) []string {
	if runtime == "python" {
		return []string{"python3", "test/smoke.py"}
	}
	return []string{"node", "test/smoke.js"}
}

// fsList lists every template file under dir (recursive), sorted, with
// dir-relative slash paths ("manifest.json.tmpl", "src/index.js.tmpl").
func fsList(dir string) ([]string, error) {
	var names []string
	err := fs.WalkDir(templates, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		names = append(names, strings.TrimPrefix(strings.TrimPrefix(p, dir), "/"))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("template dir %s: %w", dir, err)
	}
	sort.Strings(names)
	return names, nil
}

// titleFromSlug turns "media-utils" into "Media Utils".
func titleFromSlug(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, p := range parts {
		r := []rune(p)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
			parts[i] = string(r)
		}
	}
	return strings.Join(parts, " ")
}
