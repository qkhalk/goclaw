package tools_test

// Proves the fail-closed guarantee behind the video designer agent: the
// designer's tool policy (video.DesignerToolPolicy) must filter a registry
// that also carries dangerous tools down to EXACTLY its knowledge allowlist.
// If a refactor ever widens the designer's effective surface, this fails.

import (
	"context"
	"slices"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	videopkg "github.com/nextlevelbuilder/goclaw/internal/video"
)

type designerPolicyMockTool struct{ name string }

func (m *designerPolicyMockTool) Name() string               { return m.name }
func (m *designerPolicyMockTool) Description() string        { return "mock " + m.name }
func (m *designerPolicyMockTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (m *designerPolicyMockTool) Execute(context.Context, map[string]any) *tools.Result {
	return &tools.Result{ForLLM: "ok"}
}

func TestFilterTools_DesignerAgentExposesOnlyKnowledgeTools(t *testing.T) {
	reg := tools.NewRegistry()
	for _, name := range []string{
		// designer allowlist
		"skill_search", "use_skill", "session_status",
		// system-touching tools the designer must never see
		"exec", "write_file", "edit_file", "read_file", "apply_patch",
		"web_fetch", "web_search", "browser_open",
		"render_video", "delegate_task", "message", "cron_create",
		"memory_search", "session_spawn",
	} {
		reg.Register(&designerPolicyMockTool{name: name})
	}

	pe := tools.NewPolicyEngine(&config.ToolsConfig{})
	defs := pe.FilterTools(reg, videopkg.DesignerAgentKey, "openai", videopkg.DesignerToolPolicy(), nil, false, false)

	got := make([]string, 0, len(defs))
	for _, d := range defs {
		if d.Function == nil {
			t.Fatalf("non-function definition in filtered set: %+v", d)
		}
		got = append(got, d.Function.Name)
	}
	slices.Sort(got)

	want := []string{"session_status", "skill_search", "use_skill"}
	if !slices.Equal(got, want) {
		t.Errorf("designer tool surface = %v, want exactly %v", got, want)
	}
}
