package skills

// requires.go — SKILL.md frontmatter `requires:` gating (inheritance plan
// Phase 4). A skill may declare host prerequisites:
//
//	---
//	requires:
//	  bins:
//	    - ffmpeg
//	    - jq
//	  os:
//	    - linux
//	---
//
// At load time the loader evaluates the block: a required binary missing
// from PATH (exec.LookPath) or a mismatched OS marks the skill unavailable
// with a human-readable reason. Unavailable skills stay listed (Info JSON
// carries requires + unavailableReason) but are annotated in the prompt
// summary and skipped by auto-injection. Gating re-evaluates on every
// ListSkills scan, so hot-reload picks up newly installed binaries.
// JSON frontmatter is handled by encoding/json into Metadata.Requires via
// the same field names ("requires": {"bins": [...], "os": [...]}).

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// SkillRequires is the `requires:` frontmatter block. Both fields are
// optional; an empty block imposes no constraints.
type SkillRequires struct {
	Bins []string `json:"bins,omitempty"`
	OS   []string `json:"os,omitempty"`
}

// IsEmpty reports whether the block imposes no constraints.
func (r *SkillRequires) IsEmpty() bool {
	return r == nil || (len(r.Bins) == 0 && len(r.OS) == 0)
}

// lookPath is exec.LookPath by default; tests override to simulate a fake PATH.
var lookPath = exec.LookPath

// checkRequires evaluates a requires block against the host. Returns ""
// when all constraints are satisfied, otherwise a human-readable reason.
func checkRequires(req *SkillRequires, goos string) string {
	if req.IsEmpty() {
		return ""
	}
	var missing []string
	for _, bin := range req.Bins {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			continue
		}
		if _, err := lookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf("missing required binary: %s", strings.Join(missing, ", "))
	}
	if len(req.OS) > 0 && !osSupported(req.OS, goos) {
		return fmt.Sprintf("unsupported OS: %s (requires %s)", goos, strings.Join(req.OS, ", "))
	}
	return ""
}

// osSupported reports whether goos matches any entry. Accepts canonical Go
// GOOS values plus the "macos" alias for darwin.
func osSupported(wanted []string, goos string) bool {
	for _, w := range wanted {
		w = strings.ToLower(strings.TrimSpace(w))
		switch w {
		case "":
		case "macos", "mac":
			w = "darwin"
		}
		if w == goos {
			return true
		}
	}
	return false
}

// parseRequires extracts the `requires:` block from YAML frontmatter text
// (the full frontmatter, not the whole document). Supported shape — nested
// flat lists under bins/os, with a comma/space-separated scalar fallback:
//
//	requires:
//	  bins:
//	    - ffmpeg
//	  os: linux, darwin
//
// Returns nil when no (non-empty) requires block is present.
func parseRequires(frontmatter string) *SkillRequires {
	lines := strings.Split(normalizeLineEndings(frontmatter), "\n")
	req := &SkillRequires{}
	inRequires := false
	var current string // "bins" | "os"

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")

		if !indented {
			// Top-level key — the requires block ends here.
			if inRequires {
				break
			}
			if key, _ := splitYAMLKey(trimmed); key == "requires" {
				inRequires = true
			}
			continue
		}

		if !inRequires {
			continue
		}

		// List item line first — items may themselves contain colons
		// (e.g. "- bin:ffmpeg") and must not be mistaken for sub-keys.
		if val, ok := strings.CutPrefix(trimmed, "- "); ok {
			val = strings.Trim(strings.TrimSpace(val), "\"'")
			if val == "" {
				continue
			}
			switch current {
			case "bins":
				req.Bins = append(req.Bins, val)
			case "os":
				req.OS = append(req.OS, val)
			}
			continue
		}

		// Sub-key line: "bins:" / "os: a, b"
		key, val, ok := splitYAMLKeyValue(trimmed)
		if ok && (key == "bins" || key == "os") {
			current = key
			if val != "" {
				// Scalar fallback: comma/space separated.
				list := splitListValue(val)
				if key == "bins" {
					req.Bins = append(req.Bins, list...)
				} else {
					req.OS = append(req.OS, list...)
				}
			}
			continue
		}
		if ok {
			// Unknown sub-key inside requires (forward-compat) — stop
			// attributing list items to the previous sub-key.
			current = ""
		}
	}

	if req.IsEmpty() {
		return nil
	}
	return req
}

// splitYAMLKey splits "key: value" into its key part (colon included in the
// suffix test is handled by callers).
func splitYAMLKey(line string) (string, string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return line, ""
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
}

// splitYAMLKeyValue reports whether the line is a "key: value" pair.
func splitYAMLKeyValue(line string) (string, string, bool) {
	key, val := splitYAMLKey(line)
	if key == "" {
		return "", "", false
	}
	return key, val, true
}

// evaluateRequires annotates info with the parsed requires block and its
// availability for the current host. Called from applyMetadata so every
// discovery path (list scans, managed skills, builtin) is gated uniformly.
func evaluateRequires(info *Info, meta *Metadata) {
	info.Requires = meta.Requires
	if meta.Requires.IsEmpty() {
		info.UnavailableReason = ""
		return
	}
	info.UnavailableReason = checkRequires(meta.Requires, runtime.GOOS)
}
