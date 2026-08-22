package agent

import (
	"strings"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/pipeline"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// writeSkillFiles materializes slug→SKILL.md content into root/slug/.
func writeSkillFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for slug, content := range files {
		dir := filepath.Join(root, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// newSkillTestLoop builds a Loop whose skills loader discovers the given
// SKILL.md files (as global skills) and whose tool registry is a real
// registry seeded with builtin groups. Personal skills are disabled so the
// discovery surface is exactly the fixtures.
func newSkillTestLoop(t *testing.T, files map[string]string, registry *tools.Registry) *Loop {
	t.Helper()
	t.Setenv("GOCLAW_DISABLE_PERSONAL_SKILLS", "1")
	root := t.TempDir()
	writeSkillFiles(t, root, files)
	return NewLoop(LoopConfig{
		SkillsLoader: skills.NewLoader("", root, ""),
		Tools:        registry,
	})
}

// fsOnlySkill declares only the fs builtin tool group.
const fsOnlySkill = "---\nname: scoped\ndescription: fs-only skill\nallowed-tools:\n  - fs\n---\n\nbody\n"

func authorizeState(filter []string, allowed map[string]bool) *pipeline.RunState {
	st := &pipeline.RunState{Input: &pipeline.RunInput{SkillFilter: filter}}
	st.Tool.AllowedTools = allowed
	return st
}

func TestAuthorizeToolCall_SkillAllowedToolsDeniesUndeclaredTool(t *testing.T) {
	loop := newSkillTestLoop(t, map[string]string{"scoped": fsOnlySkill}, tools.NewRegistry())
	gate := loop.makeAuthorizeToolCall()

	state := authorizeState([]string{"scoped"}, map[string]bool{
		"exec":       true,
		"web_search": true,
		"read_file":  true,
		"write_file": true,
	})

	ok, detail := gate(context.Background(), state, providers.ToolCall{Name: "exec"})
	if ok {
		t.Fatal("exec must be denied: filtered skill declares only allowed-tools [fs]")
	}
	if !strings.Contains(detail, "skill permissions") {
		t.Errorf("deny detail %q missing skill-permission context", detail)
	}
	if !strings.Contains(detail, "fs") {
		t.Errorf("deny detail %q missing declared scope (fs)", detail)
	}

	// Declared group members that survive the policy-filtered intersection pass.
	for _, name := range []string{"read_file", "write_file"} {
		ok, detail := gate(context.Background(), state, providers.ToolCall{Name: name})
		if !ok || detail != "" {
			t.Errorf("%s: ok=%v detail=%q, want allowed with empty detail", name, ok, detail)
		}
	}
}

func TestAuthorizeToolCall_UnknownEntriesFailClosed(t *testing.T) {
	broken := "---\nname: broken\ndescription: bad decls\nallowed-tools:\n  - group:nope\n  - totally_unknown_tool\n---\n\nbody\n"
	loop := newSkillTestLoop(t, map[string]string{"broken": broken}, tools.NewRegistry())
	gate := loop.makeAuthorizeToolCall()

	// Policy-filtered allowlist is generous; the skill scope still denies all.
	state := authorizeState([]string{"broken"}, map[string]bool{
		"exec": true, "read_file": true, "web_search": true, "memory_search": true,
	})

	for _, name := range []string{"read_file", "exec", "memory_search"} {
		ok, detail := gate(context.Background(), state, providers.ToolCall{Name: name})
		if ok {
			t.Errorf("%s: expected fail-closed deny for unresolvable allowed-tools entries", name)
		}
		if !strings.Contains(detail, "skill permissions") {
			t.Errorf("%s: deny detail %q missing skill-permission context", name, detail)
		}
	}
}

func TestAuthorizeToolCall_LegacyAliasExpandsToCanonical(t *testing.T) {
	legacy := "---\nname: ops\ndescription: legacy alias\nallowed-tools:\n  - bash\n---\n\nbody\n"
	loop := newSkillTestLoop(t, map[string]string{"ops": legacy}, tools.NewRegistry())
	gate := loop.makeAuthorizeToolCall()

	state := authorizeState([]string{"ops"}, map[string]bool{"exec": true, "web_search": true})

	if ok, _ := gate(context.Background(), state, providers.ToolCall{Name: "exec"}); !ok {
		t.Error("bash alias should expand to canonical exec and be allowed")
	}
	if ok, _ := gate(context.Background(), state, providers.ToolCall{Name: "web_search"}); ok {
		t.Error("web_search is outside the skill declaration and must be denied")
	}
}

func TestAuthorizeToolCall_DeferredActivationCannotBypassSkillScope(t *testing.T) {
	reg := tools.NewRegistry()
	reg.SetDeferredActivator(func(name string) bool { return true })
	loop := newSkillTestLoop(t, map[string]string{"scoped": fsOnlySkill}, reg)
	gate := loop.makeAuthorizeToolCall()

	state := authorizeState([]string{"scoped"}, map[string]bool{"read_file": true})

	ok, detail := gate(context.Background(), state, providers.ToolCall{Name: "mcp_late__query"})
	if ok {
		t.Fatal("lazy-activated tool must not bypass skill-declared permissions")
	}
	if !strings.Contains(detail, "skill permissions") {
		t.Errorf("deny detail %q missing skill-permission context", detail)
	}
	if state.Tool.AllowedTools["mcp_late__query"] {
		t.Error("denied tool must not be added to the runtime allowlist")
	}
}

func TestAuthorizeToolCall_NoSkillFilterIsZeroBehaviorChange(t *testing.T) {
	loop := newSkillTestLoop(t, map[string]string{"scoped": fsOnlySkill}, tools.NewRegistry())
	gate := loop.makeAuthorizeToolCall()

	// Nil input entirely.
	state := authorizeState(nil, map[string]bool{"exec": true})
	if ok, detail := gate(context.Background(), state, providers.ToolCall{Name: "exec"}); !ok || detail != "" {
		t.Errorf("nil Input: ok=%v detail=%q, want unchanged allow", ok, detail)
	}

	// Present input but no skill narrowing.
	state = authorizeState([]string{}, map[string]bool{"exec": true})
	if ok, detail := gate(context.Background(), state, providers.ToolCall{Name: "exec"}); !ok || detail != "" {
		t.Errorf("empty SkillFilter: ok=%v detail=%q, want unchanged allow", ok, detail)
	}

	// Outside the policy-filtered set still denied with the original wording.
	state = authorizeState(nil, map[string]bool{"read_file": true})
	ok, detail := gate(context.Background(), state, providers.ToolCall{Name: "exec"})
	if ok {
		t.Fatal("exec outside policy-filtered allowlist must stay denied")
	}
	if detail != "tool not allowed by policy: exec" {
		t.Errorf("detail = %q, want original policy denial wording", detail)
	}
}

func TestAuthorizeToolCall_SkillsWithoutAllowedToolsAreZeroBehaviorChange(t *testing.T) {
	plain := "---\nname: plain\ndescription: no tool restrictions\n---\n\nbody\n"
	loop := newSkillTestLoop(t, map[string]string{"plain": plain}, tools.NewRegistry())
	gate := loop.makeAuthorizeToolCall()

	for _, filter := range [][]string{{"plain"}, {"ghost-skill"}, {"plain", "ghost-skill"}} {
		state := authorizeState(filter, map[string]bool{"exec": true})
		ok, detail := gate(context.Background(), state, providers.ToolCall{Name: "exec"})
		if !ok || detail != "" {
			t.Errorf("filter %v: ok=%v detail=%q, want unchanged allow (no AllowedTools declared)", filter, ok, detail)
		}

		state = authorizeState(filter, map[string]bool{"read_file": true})
		ok, _ = gate(context.Background(), state, providers.ToolCall{Name: "exec"})
		if ok {
			t.Errorf("filter %v: exec outside policy-filtered allowlist must stay denied", filter)
		}
	}
}

func TestNarrowAllowedForSkill_NilWhenNotApplicable(t *testing.T) {
	loop := newSkillTestLoop(t, map[string]string{"scoped": fsOnlySkill}, tools.NewRegistry())

	cases := []struct {
		name  string
		state *pipeline.RunState
	}{
		{"nil input", &pipeline.RunState{}},
		{"no filter", authorizeState(nil, map[string]bool{"exec": true})},
	}
	for _, tc := range cases {
		narrowed, detail := loop.narrowAllowedForSkill(context.Background(), tc.state)
		if narrowed != nil || detail != "" {
			t.Errorf("%s: narrowed=%v detail=%q, want nil/empty", tc.name, narrowed, detail)
		}
	}

	// No skills loader on the loop: narrowing never activates.
	bare := NewLoop(LoopConfig{})
	state := authorizeState([]string{"scoped"}, map[string]bool{"exec": true})
	narrowed, detail := bare.narrowAllowedForSkill(context.Background(), state)
	if narrowed != nil || detail != "" {
		t.Errorf("nil loader: narrowed=%v detail=%q, want nil/empty", narrowed, detail)
	}
}

func TestExpandSkillToolEntries_GroupPrefixAndBareGroupNames(t *testing.T) {
	loop := newSkillTestLoop(t, nil, tools.NewRegistry())
	got := loop.expandSkillToolEntries([]string{"group:web", "runtime", "bash", "  EXEC  ", "", "group:missing", "no_such_tool"})
	want := []string{
		// group:web expands
		"web_search", "web_fetch",
		// bare builtin group name expands
		"exec", "wait",
		// legacy alias maps to canonical
		"exec",
		// trimmed/lowercased canonical passthrough
		"exec",
		// unknown group expands to nothing; unrecognized name survives as a
		// candidate but can never match the runtime allowlist (fail-closed).
		"no_such_tool",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expand = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expand = %v, want %v", got, want)
		}
	}

	// Without a concrete registry, groups contribute nothing but names survive.
	bare := NewLoop(LoopConfig{})
	got = bare.expandSkillToolEntries([]string{"group:fs", "exec"})
	if len(got) != 1 || got[0] != "exec" {
		t.Errorf("bare-loop expand = %v, want [exec]", got)
	}
}
