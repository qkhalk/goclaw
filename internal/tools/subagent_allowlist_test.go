package tools

import "testing"

// Regression: a definition's allowedTools must also prune DEFERRED registry
// twins. Deferred tools are invisible to List() but callable by exact name —
// without PruneDeferred the allow list would not narrow the callable surface.
func TestApplyDefinitionAllowListPrunesDeferredTools(t *testing.T) {
	reg := NewRegistry()
	// "aaa_keep" sorts before "list_files", so with threshold 1 the deferred
	// suffix is exactly list_files (lexicographic prefix keeps the budget).
	reg.Register(&mockTool{name: "aaa_keep"})
	reg.Register(&mockTool{name: "list_files"})
	reg.SetDeferredThreshold(1)
	reg.ApplyDeferredMode()
	if _, ok := reg.Get("list_files"); ok {
		t.Fatal("precondition: list_files should be deferred (invisible via Get)")
	}

	sm := NewSubagentManager(nil, nil, "", nil, nil, SubagentConfig{})
	sm.applyDefinitionAllowList(reg, []string{"aaa_keep"})

	if _, ok := reg.Get("list_files"); ok {
		t.Error("pruned deferred tool still reachable via Get")
	}
	if reg.TryActivateDeferred("list_files") {
		t.Error("pruned deferred tool must not be activatable by name")
	}
	if _, ok := reg.Get("aaa_keep"); !ok {
		t.Error("allowed tool was removed by the allow list")
	}
}

// Without an allow list nothing is narrowed — including deferred twins.
func TestApplyDefinitionAllowListEmptyKeepsEverything(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{name: "aaa_keep"})
	reg.Register(&mockTool{name: "list_files"})
	reg.SetDeferredThreshold(1)
	reg.ApplyDeferredMode()

	sm := NewSubagentManager(nil, nil, "", nil, nil, SubagentConfig{})
	sm.applyDefinitionAllowList(reg, nil)

	if _, ok := reg.Get("aaa_keep"); !ok {
		t.Error("inline tool removed by an empty allow list")
	}
	if !reg.TryActivateDeferred("list_files") {
		t.Error("deferred tool should remain activatable with no allow list")
	}
}
