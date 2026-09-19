// Package mcpcatalog embeds the MCP tool-server catalog that ships with the
// gateway. Each subfolder of mcp/ is one installable tool server described by
// its manifest.json (see Manifest for the schema). Server code lives next to
// its manifest in this repo and is fetched at install time from the repo's
// release tag — the gateway binary embeds only the manifests, never the code.
//
// Adding a tool to the catalog = adding a folder with a manifest.json here.
// No codegen step, no second place to update.
package mcpcatalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"sync"
)

//go:embed */manifest.json
var manifests embed.FS

// DefaultRepo is the git source used when a manifest does not pin its own
// repo: the gateway's own repository. Installs clone a release tag and
// sparse-checkout only the tool's subfolder.
const DefaultRepo = "https://github.com/qkhalk/goclaw"

// Manifest describes one installable MCP tool server (mcp/<tool>/manifest.json).
type Manifest struct {
	// Name is the install slug and the MCP server registry name (valid slug).
	Name string `json:"name"`
	// DisplayName is shown in the Tool Store card.
	DisplayName string `json:"display_name"`
	// Description is shown in the Tool Store card (English, product copy).
	Description string `json:"description"`
	// Category groups catalog cards in the Store (e.g. "media", "files").
	Category string `json:"category,omitempty"`
	// Runtime is the process runtime: "node" or "python".
	Runtime string `json:"runtime"`
	// Entry is the server entry file relative to the tool folder, spawned as
	// `<runtime> <folder>/<entry>` over stdio.
	Entry string `json:"entry"`
	// Repo overrides DefaultRepo when the tool ships from another repository.
	// Only github.com URLs are accepted by the installer.
	Repo string `json:"repo,omitempty"`
	// Ref pins the git tag/commit to install. Empty = the gateway's own
	// running version tag (e.g. v4.9.4), so catalog installs always match
	// the shipped manifests.
	Ref string `json:"ref,omitempty"`
	// Subdir is the tool folder inside the repo. Empty = "mcp/<name>".
	Subdir string `json:"subdir,omitempty"`
	// RAMNote is a rough idle-RSS hint shown on the Store card.
	RAMNote string `json:"ram_note,omitempty"`
}

// Entries returns the embedded catalog, sorted by name. It parses once at
// first use; a malformed manifest panics at startup (fail fast — the catalog
// ships with the binary and must be valid).
func Entries() []Manifest {
	entriesOnce.Do(func() {
		names, err := fs.Glob(manifests, "*/manifest.json")
		if err != nil {
			panic(fmt.Sprintf("mcpcatalog: glob manifests: %v", err))
		}
		for _, n := range names {
			raw, err := manifests.ReadFile(n)
			if err != nil {
				panic(fmt.Sprintf("mcpcatalog: read %s: %v", n, err))
			}
			var m Manifest
			if err := json.Unmarshal(raw, &m); err != nil {
				panic(fmt.Sprintf("mcpcatalog: parse %s: %v", n, err))
			}
			if err := m.Validate(); err != nil {
				panic(fmt.Sprintf("mcpcatalog: invalid %s: %v", n, err))
			}
			parsed = append(parsed, m)
		}
		// fs.Glob is already name-sorted; keep it stable anyway.
		for i := 1; i < len(parsed); i++ {
			for j := i; j > 0 && parsed[j].Name < parsed[j-1].Name; j-- {
				parsed[j], parsed[j-1] = parsed[j-1], parsed[j]
			}
		}
	})
	return parsed
}

var (
	entriesOnce sync.Once
	parsed      []Manifest
)

// Find returns the manifest with the given name, or nil.
func Find(name string) *Manifest {
	for i, m := range Entries() {
		if m.Name == name {
			return &Entries()[i]
		}
	}
	return nil
}

// Validate checks fields the installer relies on.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.ContainsAny(m.Name, " /\\") {
		return fmt.Errorf("name must be a slug: %q", m.Name)
	}
	if m.DisplayName == "" || m.Description == "" {
		return fmt.Errorf("display_name and description are required")
	}
	switch m.Runtime {
	case "node", "python":
	default:
		return fmt.Errorf("runtime must be node or python, got %q", m.Runtime)
	}
	if m.Entry == "" {
		return fmt.Errorf("entry is required")
	}
	if strings.HasPrefix(m.Entry, "/") || strings.Contains(m.Entry, "..") {
		return fmt.Errorf("entry must be a relative path inside the tool folder: %q", m.Entry)
	}
	return nil
}
