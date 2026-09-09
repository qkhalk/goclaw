package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// descTool is a mock tool with a configurable description (needed for BM25
// ranking tests — mockTool's description is fixed).
type descTool struct {
	name string
	desc string
}

func (m *descTool) Name() string               { return m.name }
func (m *descTool) Description() string        { return m.desc }
func (m *descTool) Parameters() map[string]any { return nil }
func (m *descTool) Execute(ctx context.Context, args map[string]any) *Result {
	return NewResult("ran " + m.name)
}

// registerDeferrable registers n distinctly-named tools whose lexicographic
// order matches registration order (zero-padded numeric suffixes).
func registerDeferrable(reg *Registry, n int) []string {
	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("deftool_%02d", i)
		reg.Register(&descTool{name: name, desc: "deftool utility number " + fmt.Sprintf("%02d", i)})
		names = append(names, name)
	}
	return names
}

// TestApplyDeferredMode_ThresholdCrossing verifies the core transition: over
// threshold → excess deferred, tool_search registered; under threshold → no-op.
func TestApplyDeferredMode_ThresholdCrossing(t *testing.T) {
	reg := NewRegistry()
	registerDeferrable(reg, 10)
	reg.SetDeferredThreshold(6)

	reg.ApplyDeferredMode()

	if !reg.IsDeferredMode() {
		t.Fatal("registry should be in deferred mode")
	}
	visible := reg.List()
	// 6 inline + tool_search
	if len(visible) != 7 {
		t.Fatalf("expected 7 visible tools (6 inline + tool_search), got %d: %v", len(visible), visible)
	}
	if !slices.Contains(visible, ToolSearchName) {
		t.Fatal("tool_search must be visible")
	}
	deferred := reg.DeferredToolNames()
	if len(deferred) != 4 {
		t.Fatalf("expected 4 deferred tools, got %d: %v", len(deferred), deferred)
	}
	// Deterministic excess: the lexicographic SUFFIX goes deferred.
	wantDeferred := []string{"deftool_06", "deftool_07", "deftool_08", "deftool_09"}
	if !slices.Equal(deferred, wantDeferred) {
		t.Errorf("deferred set = %v, want %v", deferred, wantDeferred)
	}
	// Deferred tools are not visible via Get/List/ProviderDefs.
	for _, name := range deferred {
		if _, ok := reg.Get(name); ok {
			t.Errorf("deferred tool %q must not be visible via Get", name)
		}
	}
	if defs := reg.ProviderDefs(); len(defs) != 7 {
		t.Errorf("ProviderDefs should expose 7 defs (6 inline + tool_search), got %d", len(defs))
	}
}

// TestApplyDeferredMode_UnderThreshold: below threshold nothing changes.
func TestApplyDeferredMode_UnderThreshold(t *testing.T) {
	reg := NewRegistry()
	registerDeferrable(reg, 5)
	reg.SetDeferredThreshold(60)

	reg.ApplyDeferredMode()

	if reg.IsDeferredMode() {
		t.Error("under threshold, deferred mode must not activate")
	}
	if got := reg.List(); len(got) != 5 {
		t.Errorf("expected 5 visible tools, got %d", len(got))
	}
	if _, ok := reg.Get(ToolSearchName); ok {
		t.Error("tool_search must not be registered under threshold")
	}
}

// TestApplyDeferredMode_Disabled: threshold 0 (never configured) → no-op even
// with a huge tool set. This is the default-off shipping guarantee.
func TestApplyDeferredMode_Disabled(t *testing.T) {
	reg := NewRegistry()
	registerDeferrable(reg, 20)
	// No SetDeferredThreshold call — disabled.

	reg.ApplyDeferredMode()

	if reg.IsDeferredMode() {
		t.Error("disabled deferred mode must stay inactive")
	}
	if got := reg.List(); len(got) != 20 {
		t.Errorf("expected all 20 tools inline, got %d", len(got))
	}
}

// TestApplyDeferredMode_AlwaysInline: protected group members stay inline even
// when lexicographically first (they would otherwise fill, not trail, the
// budget) — and the deferred suffix shrinks accordingly.
func TestApplyDeferredMode_AlwaysInline(t *testing.T) {
	reg := NewRegistry()
	registerDeferrable(reg, 8)
	// Group zz_protected contains three tools; with threshold 6, budget left
	// for unprotected tools is 6-3 = 3, so exactly 3 unprotected go deferred.
	reg.RegisterToolGroup("zz_protected", []string{"deftool_00", "deftool_01", "deftool_02"})
	reg.SetDeferredThreshold(6)
	reg.SetDeferredAlwaysInline([]string{"group:zz_protected"})

	reg.ApplyDeferredMode()

	deferred := reg.DeferredToolNames()
	for _, name := range []string{"deftool_00", "deftool_01", "deftool_02"} {
		if slices.Contains(deferred, name) {
			t.Errorf("always_inline tool %q must not be deferred", name)
		}
	}
	if len(deferred) != 2 {
		t.Fatalf("expected 2 deferred tools, got %d: %v", len(deferred), deferred)
	}
	// Fill-up consumes the budget with 03-05; the suffix defers.
	wantDeferred := []string{"deftool_06", "deftool_07"}
	if !slices.Equal(deferred, wantDeferred) {
		t.Errorf("deferred set = %v, want %v", deferred, wantDeferred)
	}
}

// TestApplyDeferredMode_AllProtected: when always_inline covers everything,
// deferred mode does not activate (nothing to defer, no search tool needed).
func TestApplyDeferredMode_AllProtected(t *testing.T) {
	reg := NewRegistry()
	names := registerDeferrable(reg, 10)
	reg.RegisterToolGroup("everything", names)
	reg.SetDeferredThreshold(4)
	reg.SetDeferredAlwaysInline([]string{"group:everything"})
	reg.ApplyDeferredMode()

	if reg.IsDeferredMode() {
		t.Error("all-protected registry must not enter deferred mode")
	}
	if got := reg.List(); len(got) != 10 {
		t.Errorf("all tools must stay inline, got %d", len(got))
	}
	if _, ok := reg.Get(ToolSearchName); ok {
		t.Error("tool_search must not be registered when nothing is deferred")
	}
}

// TestApplyDeferredMode_DeterministicOrder: two identical registries produce
// identical deferred sets (prompt-cache stability).
func TestApplyDeferredMode_DeterministicOrder(t *testing.T) {
	build := func() *Registry {
		reg := NewRegistry()
		registerDeferrable(reg, 12)
		reg.SetDeferredThreshold(5)
		reg.ApplyDeferredMode()
		return reg
	}
	a := build().DeferredToolNames()
	b := build().DeferredToolNames()
	if !slices.Equal(a, b) {
		t.Errorf("deferred sets differ between identical builds:\n%v\n%v", a, b)
	}
	if !slices.IsSorted(a) {
		t.Errorf("deferred names must be sorted, got %v", a)
	}
}

// TestDeferredActivation: Get misses while deferred; exact-name call path
// (TryActivateDeferred) promotes the tool; it then executes and disappears
// from the deferred set.
func TestDeferredActivation(t *testing.T) {
	reg := NewRegistry()
	registerDeferrable(reg, 8)
	reg.SetDeferredThreshold(4)
	reg.ApplyDeferredMode()

	if _, ok := reg.Get("deftool_06"); ok {
		t.Fatal("deftool_06 should be deferred (invisible)")
	}
	if !reg.TryActivateDeferred("deftool_06") {
		t.Fatal("TryActivateDeferred should activate a deferred native tool")
	}
	got, ok := reg.Get("deftool_06")
	if !ok {
		t.Fatal("activated tool must be visible via Get")
	}
	res := got.Execute(context.Background(), nil)
	if res == nil || !strings.Contains(res.ForLLM, "ran deftool_06") {
		t.Errorf("activated tool execution failed: %+v", res)
	}
	if slices.Contains(reg.DeferredToolNames(), "deftool_06") {
		t.Error("activated tool must leave the deferred set")
	}
	// Unknown names still report false.
	if reg.TryActivateDeferred("deftool_nonexistent") {
		t.Error("TryActivateDeferred must fail for unknown tools")
	}
}

// TestDeferredMetadataRestored: capability metadata parked on deferral is
// restored on activation.
func TestDeferredMetadataRestored(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterWithMetadata(&descTool{name: "meta_tool", desc: "metadata carrier"}, ToolMetadata{Group: "fs"})
	registerDeferrable(reg, 4)
	reg.SetDeferredThreshold(2)
	reg.ApplyDeferredMode()

	if !slices.Contains(reg.DeferredToolNames(), "meta_tool") {
		t.Fatalf("meta_tool should be deferred, deferred set: %v", reg.DeferredToolNames())
	}

	reg.TryActivateDeferred("meta_tool")

	meta := reg.GetMetadata("meta_tool")
	if meta.Group != "fs" {
		t.Errorf("metadata must be restored after activation, got group=%q caps=%v", meta.Group, meta.Capabilities)
	}
}

// TestDeferredMode_CloneCarriesState: a cloned registry (per-agent path)
// inherits deferred state and gets its OWN tool_search bound to the clone.
func TestDeferredMode_CloneCarriesState(t *testing.T) {
	reg := NewRegistry()
	registerDeferrable(reg, 8)
	reg.SetDeferredThreshold(4)
	reg.ApplyDeferredMode()

	clone := reg.Clone()
	if !clone.IsDeferredMode() {
		t.Fatal("clone must inherit deferred mode")
	}
	if !slices.Equal(clone.DeferredToolNames(), reg.DeferredToolNames()) {
		t.Errorf("clone deferred set mismatch: %v vs %v", clone.DeferredToolNames(), reg.DeferredToolNames())
	}
	// Clone activation is independent of the parent.
	if !clone.TryActivateDeferred("deftool_05") {
		t.Fatal("clone must activate its own deferred tools")
	}
	if slices.Contains(clone.DeferredToolNames(), "deftool_05") {
		t.Error("clone deferred set should shrink after activation")
	}
	if !slices.Contains(reg.DeferredToolNames(), "deftool_05") {
		t.Error("parent deferred set must be unaffected by clone activation")
	}
	// The clone's tool_search is bound to the CLONE, not the parent.
	st, ok := clone.Get(ToolSearchName)
	if !ok {
		t.Fatal("clone must carry tool_search")
	}
	search, ok := st.(*ToolSearchTool)
	if !ok {
		t.Fatalf("tool_search must be *ToolSearchTool, got %T", st)
	}
	if search.reg != clone {
		t.Error("clone's tool_search must reference the clone, not the parent")
	}
}

// TestDeferredDirectCallActivates: executing a deferred tool by exact name via
// the registry (no-policy path — no allowlist check ran) activates it instead
// of returning "unknown tool".
func TestDeferredDirectCallActivates(t *testing.T) {
	reg := NewRegistry()
	registerDeferrable(reg, 6)
	reg.SetDeferredThreshold(3)
	reg.ApplyDeferredMode()

	res := reg.Execute(context.Background(), "deftool_05", nil)
	if res == nil || res.IsError {
		t.Fatalf("direct call of deferred tool must activate and run, got: %+v", res)
	}
	if !strings.Contains(res.ForLLM, "ran deftool_05") {
		t.Errorf("unexpected result: %s", res.ForLLM)
	}
	if slices.Contains(reg.DeferredToolNames(), "deftool_05") {
		t.Error("tool must leave the deferred set after direct-call activation")
	}
	// Truly unknown tools still error.
	if r2 := reg.Execute(context.Background(), "deftool_missing", nil); r2 == nil || !r2.IsError {
		t.Error("unknown tool must still be an error result")
	}
}

// TestToolSearchTool_RankingAndActivation: BM25 ranking puts the best
// description match first, and Execute activates the hits.
func TestToolSearchTool_RankingAndActivation(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&descTool{name: "pdf_reader", desc: "Read and extract text from PDF documents"})
	reg.Register(&descTool{name: "audio_mixer", desc: "Mix and convert audio tracks"})
	reg.Register(&descTool{name: "cron_scheduler", desc: "Schedule recurring cron jobs"})
	reg.Register(&descTool{name: "zz_filler", desc: "misc helper"})
	reg.SetDeferredThreshold(1)
	reg.ApplyDeferredMode()

	st, ok := reg.Get(ToolSearchName)
	if !ok {
		t.Fatal("tool_search must be registered in deferred mode")
	}
	res := st.Execute(context.Background(), map[string]any{
		"query": "read pdf document",
	})
	if res == nil || res.IsError {
		t.Fatalf("tool_search execution failed: %+v", res)
	}
	if !strings.Contains(res.ForLLM, "pdf_reader") {
		t.Errorf("expected pdf_reader in results, got: %s", res.ForLLM)
	}
	// Execution must have activated the hit.
	if _, ok := reg.Get("pdf_reader"); !ok {
		t.Error("tool_search must activate discovered tools")
	}
	if slices.Contains(reg.DeferredToolNames(), "pdf_reader") {
		t.Error("activated tool must leave the deferred set")
	}

	// Ranking sanity: the dedicated match outranks the filler.
	hits := reg.searchDeferredTools("schedule recurring cron", 5)
	if len(hits) == 0 {
		t.Fatal("expected hits for cron query")
	}
	if hits[0].Name != "cron_scheduler" {
		t.Errorf("expected cron_scheduler first, got %q (hits: %+v)", hits[0].Name, hits)
	}

	// Empty query errors.
	if r := st.Execute(context.Background(), map[string]any{}); r == nil || !r.IsError {
		t.Error("empty query must be an error result")
	}
}
