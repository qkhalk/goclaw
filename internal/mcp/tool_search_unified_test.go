package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// putRegistryInDeferredMode registers n stub tools and activates native
// deferred mode with the given threshold.
func putRegistryInDeferredMode(t *testing.T, reg *tools.Registry, n, threshold int) []string {
	t.Helper()
	names := make([]string, 0, n)
	for i := range n {
		name := "native_" + string(rune('a'+i)) + "tool"
		reg.Register(&nativeStubTool{name: name})
		names = append(names, name)
	}
	reg.SetDeferredThreshold(threshold)
	reg.ApplyDeferredMode()
	if !reg.IsDeferredMode() {
		t.Fatalf("registry should be in deferred mode (n=%d threshold=%d)", n, threshold)
	}
	return names
}

// nativeStubTool is a minimal tools.Tool for registry seeding in mcp tests.
type nativeStubTool struct {
	name string
}

func (m *nativeStubTool) Name() string { return m.name }
func (m *nativeStubTool) Description() string {
	return "native stub tool for unified search testing"
}
func (m *nativeStubTool) Parameters() map[string]any { return nil }
func (m *nativeStubTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	return tools.NewResult("ran " + m.name)
}

// TestNewMCPToolSearchTool_ClassicWhenNoNativeDeferred: without native deferred
// mode, the classic mcp_tool_search is returned (backward-compat contract).
func TestNewMCPToolSearchTool_ClassicWhenNoNativeDeferred(t *testing.T) {
	m, reg := setupSearchModeManager(t, "svc", []string{"get_data", "list_items"})
	_ = reg

	got := NewMCPToolSearchTool(m)
	if got.Name() != "mcp_tool_search" {
		t.Errorf("classic mode name = %q, want mcp_tool_search", got.Name())
	}
}

// TestNewMCPToolSearchTool_UnifiedWhenNativeDeferred: with native deferred mode
// active, the unified tool_search is returned and covers BOTH native and MCP
// deferred entries — one meta-tool instead of two.
func TestNewMCPToolSearchTool_UnifiedWhenNativeDeferred(t *testing.T) {
	m, reg := setupSearchModeManager(t, "svc", []string{"get_data", "list_items"})
	putRegistryInDeferredMode(t, reg, 6, 2)

	got := NewMCPToolSearchTool(m)
	if got.Name() != "tool_search" {
		t.Fatalf("unified mode name = %q, want tool_search", got.Name())
	}

	res := got.Execute(context.Background(), map[string]any{
		"query":       "tool",
		"max_results": float64(10),
	})
	if res == nil || res.IsError {
		t.Fatalf("unified execute failed: %+v", res)
	}
	// Both kinds must appear: native stub + MCP deferred tool (registered name
	// mcp_svc__get_data) and both must be activated afterwards.
	if !strings.Contains(res.ForLLM, "native_") {
		t.Errorf("unified result must include native deferred tools, got: %s", res.ForLLM)
	}
	if !strings.Contains(res.ForLLM, "mcp_svc__get_data") {
		t.Errorf("unified result must include MCP deferred tools, got: %s", res.ForLLM)
	}
	if _, ok := reg.Get("mcp_svc__get_data"); !ok {
		t.Error("MCP deferred hit must be activated in the registry")
	}
	if _, ok := reg.Get("mcp_svc__list_items"); !ok {
		t.Error("all matched MCP deferred tools must be activated")
	}
}

// TestUnifiedToolSearch_KindPartition: entries carry kind builtin/mcp so the
// LLM can tell the two worlds apart.
func TestUnifiedToolSearch_KindPartition(t *testing.T) {
	m, reg := setupSearchModeManager(t, "svc", []string{"get_data"})
	putRegistryInDeferredMode(t, reg, 4, 1)

	got := NewMCPToolSearchTool(m)
	res := got.Execute(context.Background(), map[string]any{"query": "tool", "max_results": float64(10)})
	if res == nil || res.IsError {
		t.Fatalf("unified execute failed: %+v", res)
	}
	if !strings.Contains(res.ForLLM, `"kind": "builtin"`) {
		t.Errorf("expected builtin kind entries, got: %s", res.ForLLM)
	}
	if !strings.Contains(res.ForLLM, `"kind": "mcp"`) {
		t.Errorf("expected mcp kind entries, got: %s", res.ForLLM)
	}
}
