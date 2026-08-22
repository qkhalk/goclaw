// Package qualitygate implements the Wave-1 quality-gate engine: a small,
// fail-closed evaluator registry driven by the quality-gates entries parsed
// from SKILL.md frontmatter (skills.Metadata.QualityGates).
//
// A gate spec string follows the grammar:
//
//	[name:]type[(key=value,key=value)]
//
// Examples:
//
//	test                          → {Name: "test", Type: "test"}       (defaults apply)
//	tests:test(timeout_ms=5000)   → {Name: "tests", Type: "test", Params{"timeout_ms": 5000}}
//	docs(file(path=README.md))    → {Name: "docs", Type: "file", Params{"path": "README.md"}}
//
// Param values parse as bool, number, or string. Unknown gate TYPES never
// pass: Registry.Evaluate returns Passed=false with detail "unknown gate
// type" so a typo'd or unsupported gate blocks the run instead of silently
// degrading verification.
//
// Security posture: every shell-out validates its command against the shared
// shell deny groups (tools.ResolveDenyPatterns, honoring per-agent overrides
// carried on the context exactly like the exec tool) and never interpolates
// parameters into command strings — commands are taken verbatim from gate
// params, which are authored in SKILL.md frontmatter, not model output.
package qualitygate

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Builtin gate type names. These are the values accepted by
// NewBuiltinRegistry; see evaluator.go for their semantics.
const (
	TypeTest     = "test"
	TypeBuild    = "build"
	TypeLint     = "lint"
	TypeFile     = "file"
	TypeArtifact = "artifact"
	TypeSchema   = "schema"
	TypeApproval = "approval"
)

// GateSpec is the parsed shape of one skills.Metadata.QualityGates list
// entry (loader.go parseSimpleYAMLLists format: flat `- item` lines under
// `quality-gates:`).
type GateSpec struct {
	// Name identifies the gate instance (defaults to the type when the
	// entry carries no explicit name).
	Name string
	// Type selects the registered evaluator ("test", "file", ...).
	Type string
	// Params carries the parsed key=value tuning for the gate. May be nil.
	Params map[string]any
}

// String renders the canonical spec spelling (round-trips through
// ParseGateSpec).
func (s GateSpec) String() string {
	var b strings.Builder
	b.WriteString(s.Name)
	if s.Name != s.Type {
		b.WriteString(":")
		b.WriteString(s.Type)
	}
	if len(s.Params) > 0 {
		b.WriteString("(")
		first := true
		for _, k := range sortedKeys(s.Params) {
			if !first {
				b.WriteString(",")
			}
			first = false
			fmt.Fprintf(&b, "%s=%v", k, s.Params[k])
		}
		b.WriteString(")")
	}
	return b.String()
}

// GateResult is the outcome of evaluating one gate.
type GateResult struct {
	// Gate echoes GateSpec.Name so multi-gate reports stay attributable.
	Gate string
	// Passed reports whether the gate's verification held.
	Passed bool
	// Detail explains the verdict: captured command output tail, the missing
	// artifact path, the schema violation path, or the denial reason.
	Detail string
}

// result is the single constructor for verdicts, keeping the fail-closed
// shape uniform.
func result(spec GateSpec, passed bool, detail string) GateResult {
	return GateResult{Gate: spec.Name, Passed: passed, Detail: detail}
}

// Subject is the evaluation input shared by all gates: the workspace the run
// executed in and the artifact/output paths it claims to have produced.
type Subject struct {
	// Workspace is the absolute working directory commands run in and
	// relative paths resolve against.
	Workspace string
	// Artifacts lists produced output paths (relative to Workspace unless
	// absolute). Populated by the run harness after execution.
	Artifacts []string
}

// Evaluator verifies one gate spec against a subject.
type Evaluator interface {
	Evaluate(ctx context.Context, spec GateSpec, subject Subject) GateResult
}

// ParseGateSpec parses one quality-gates list entry into a GateSpec.
// Returns an error for empty or syntactically malformed entries — callers
// must treat a parse failure as a failing gate, never skip it.
func ParseGateSpec(entry string) (GateSpec, error) {
	raw := strings.TrimSpace(entry)
	if raw == "" {
		return GateSpec{}, fmt.Errorf("qualitygate: empty gate spec")
	}

	head, paramStr := raw, ""
	if i := strings.IndexByte(raw, '('); i >= 0 {
		if !strings.HasSuffix(raw, ")") {
			return GateSpec{}, fmt.Errorf("qualitygate: unclosed params in gate spec %q", raw)
		}
		head, paramStr = raw[:i], raw[i+1:len(raw)-1]
		// The head (name[:type]) must not itself contain parentheses.
		if strings.ContainsAny(head, "()") {
			return GateSpec{}, fmt.Errorf("qualitygate: malformed gate spec %q", raw)
		}
	}

	name, typ := head, head
	if i := strings.IndexByte(head, ':'); i >= 0 {
		name, typ = strings.TrimSpace(head[:i]), strings.TrimSpace(head[i+1:])
	}
	if typ == "" || name == "" {
		return GateSpec{}, fmt.Errorf("qualitygate: missing name/type in gate spec %q", raw)
	}

	spec := GateSpec{Name: name, Type: typ}
	if paramStr == "" {
		return spec, nil
	}
	spec.Params = make(map[string]any)
	for _, pair := range splitParams(paramStr) {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, found := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		if k == "" || strings.Contains(k, "(") {
			return GateSpec{}, fmt.Errorf("qualitygate: invalid param key in gate spec %q", raw)
		}
		if !found {
			// Flag form: bare key means true.
			spec.Params[k] = true
			continue
		}
		val, err := parseParamValue(strings.TrimSpace(v))
		if err != nil {
			return GateSpec{}, fmt.Errorf("qualitygate: %w in gate spec %q", err, raw)
		}
		spec.Params[k] = val
	}
	return spec, nil
}

// splitParams splits the param body on commas at paren depth 0 so values may
// contain balanced parens/commas (e.g. command=make build(x)).
func splitParams(s string) []string {
	var (
		out   []string
		depth int
		cur   strings.Builder
	)
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, cur.String())
				cur.Reset()
				continue
			}
		}
		cur.WriteRune(r)
	}
	out = append(out, cur.String())
	return out
}

// ParseGateSpecs parses a whole QualityGates list. The returned slice keeps
// input order so gate reports are deterministic.
func ParseGateSpecs(entries []string) ([]GateSpec, error) {
	specs := make([]GateSpec, 0, len(entries))
	for _, e := range entries {
		s, err := ParseGateSpec(e)
		if err != nil {
			return nil, err
		}
		specs = append(specs, s)
	}
	return specs, nil
}

// parseParamValue converts a scalar param token to bool, float64, or string.
func parseParamValue(v string) (any, error) {
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f, nil
	}
	if v == "" {
		return nil, fmt.Errorf("empty param value")
	}
	return v, nil
}

// sortedKeys orders param keys for deterministic rendering.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// --- Param accessors shared by builtin evaluators ---

// paramString returns params[key] as a string, or def when absent/not a string.
func paramString(params map[string]any, key, def string) string {
	if v, ok := params[key].(string); ok && v != "" {
		return v
	}
	return def
}

// paramInt64 returns params[key] as an int64 (accepts float64 from
// ParseGateSpec and native ints from programmatic specs), or def when
// absent/unparseable/non-positive.
func paramInt64(params map[string]any, key string, def int64) int64 {
	switch v := params[key].(type) {
	case float64:
		if v > 0 {
			return int64(v)
		}
	case int:
		if v > 0 {
			return int64(v)
		}
	case int64:
		if v > 0 {
			return v
		}
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// paramPathList collects path-like params: singular "path" and/or plural
// "paths" (comma-separated string or native list).
func paramPathList(params map[string]any) []string {
	var out []string
	appendPath := func(p string) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if p, ok := params["path"].(string); ok {
		appendPath(p)
	}
	switch v := params["paths"].(type) {
	case string:
		for _, p := range strings.Split(v, ",") {
			appendPath(p)
		}
	case []string:
		for _, p := range v {
			appendPath(p)
		}
	case []any:
		for _, item := range v {
			if p, ok := item.(string); ok {
				appendPath(p)
			}
		}
	}
	return out
}
