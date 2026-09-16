package tools

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// planTestStore wraps mockSessionStore with working metadata methods (the base
// mock's are no-ops) plus a Save counter, so plan persistence is observable.
type planTestStore struct {
	*mockSessionStore
	metadata map[string]map[string]string
	saves    int
}

func newPlanTestStore() *planTestStore {
	return &planTestStore{
		mockSessionStore: newMockSessionStore(),
		metadata:         make(map[string]map[string]string),
	}
}

func (p *planTestStore) GetSessionMetadata(_ context.Context, key string) map[string]string {
	return p.metadata[key]
}

func (p *planTestStore) SetSessionMetadata(_ context.Context, key string, metadata map[string]string) {
	dst := p.metadata[key]
	if dst == nil {
		dst = make(map[string]string, len(metadata))
	}
	maps.Copy(dst, metadata)
	p.metadata[key] = dst
}

func (p *planTestStore) Save(_ context.Context, _ string) error {
	p.saves++
	return nil
}

func planContext() context.Context {
	return WithToolSandboxKey(context.Background(), "agent:tester:ws:direct:plan-session")
}

func planStoredState(t *testing.T, st *planTestStore, key string) planState {
	t.Helper()
	raw := st.metadata[key][planMetaKey]
	if raw == "" {
		t.Fatal("no plan persisted in session metadata")
	}
	var state planState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("stored plan is not valid JSON: %v", err)
	}
	return state
}

func TestPlanSetPersistsChecklist(t *testing.T) {
	t.Parallel()
	st := newPlanTestStore()
	tool := NewPlanTool()
	tool.SetSessionStore(st)

	res := tool.Execute(planContext(), map[string]any{
		"action": "set",
		"items":  []any{"survey codebase", "write failing test", "implement fix"},
	})
	if res.IsError {
		t.Fatalf("set returned error: %s", res.ForLLM)
	}
	if st.saves != 1 {
		t.Errorf("Save calls = %d, want 1", st.saves)
	}
	state := planStoredState(t, st, "agent:tester:ws:direct:plan-session")
	if len(state.Steps) != 3 {
		t.Fatalf("stored steps = %d, want 3", len(state.Steps))
	}
	for i, want := range []string{"survey codebase", "write failing test", "implement fix"} {
		if state.Steps[i].Text != want || state.Steps[i].Status != "pending" {
			t.Errorf("step %d = %+v, want text=%q status=pending", i+1, state.Steps[i], want)
		}
	}
	// ForLLM carries the canonical JSON state the web card renders from.
	if !strings.Contains(res.ForLLM, `"text":"survey codebase"`) || !strings.Contains(res.ForLLM, `"status":"pending"`) {
		t.Errorf("ForLLM missing canonical state:\n%s", res.ForLLM)
	}
	if strings.Contains(res.ForLLM, "\n") {
		t.Errorf("ForLLM should be a single compact JSON line, got:\n%s", res.ForLLM)
	}
}

func TestPlanUpdateStepStatus(t *testing.T) {
	t.Parallel()
	st := newPlanTestStore()
	tool := NewPlanTool()
	tool.SetSessionStore(st)
	tool.Execute(planContext(), map[string]any{"action": "set", "items": []any{"a", "b", "c"}})

	res := tool.Execute(planContext(), map[string]any{"action": "update", "step": float64(2), "status": "in_progress"})
	if res.IsError {
		t.Fatalf("update returned error: %s", res.ForLLM)
	}
	state := planStoredState(t, st, "agent:tester:ws:direct:plan-session")
	if state.Steps[1].Status != "in_progress" || state.Steps[0].Status != "pending" {
		t.Errorf("statuses after update = %v/%v, want in_progress/pending", state.Steps[1].Status, state.Steps[0].Status)
	}
	if !strings.Contains(res.ForLLM, `"status":"in_progress"`) {
		t.Errorf("ForLLM missing updated canonical state:\n%s", res.ForLLM)
	}
}

func TestPlanUpdateValidation(t *testing.T) {
	t.Parallel()
	st := newPlanTestStore()
	tool := NewPlanTool()
	tool.SetSessionStore(st)

	// update before any set exists
	if res := tool.Execute(planContext(), map[string]any{"action": "update", "step": float64(1), "status": "completed"}); !res.IsError {
		t.Error("update on empty plan should error")
	}
	tool.Execute(planContext(), map[string]any{"action": "set", "items": []any{"only step"}})

	cases := []struct {
		name string
		args map[string]any
	}{
		{"step zero", map[string]any{"action": "update", "step": float64(0), "status": "completed"}},
		{"step beyond range", map[string]any{"action": "update", "step": float64(2), "status": "completed"}},
		{"missing step", map[string]any{"action": "update", "status": "completed"}},
		{"bad status", map[string]any{"action": "update", "step": float64(1), "status": "done"}},
		{"missing status", map[string]any{"action": "update", "step": float64(1)}},
		{"fractional step", map[string]any{"action": "update", "step": 1.5, "status": "completed"}},
		{"string step numeric", map[string]any{"action": "update", "step": "1", "status": "completed"}},
	}
	for _, tc := range cases {
		res := tool.Execute(planContext(), tc.args)
		wantErr := tc.name != "string step numeric"
		if res.IsError != wantErr {
			t.Errorf("%s: IsError = %v, want %v (result: %s)", tc.name, res.IsError, wantErr, res.ForLLM)
		}
	}
}

func TestPlanGetDoesNotPersist(t *testing.T) {
	t.Parallel()
	st := newPlanTestStore()
	tool := NewPlanTool()
	tool.SetSessionStore(st)
	tool.Execute(planContext(), map[string]any{"action": "set", "items": []any{"step one"}})

	savesBefore := st.saves
	res := tool.Execute(planContext(), map[string]any{"action": "get"})
	if res.IsError {
		t.Fatalf("get returned error: %s", res.ForLLM)
	}
	if st.saves != savesBefore {
		t.Errorf("get should not persist: Save calls went %d -> %d", savesBefore, st.saves)
	}
	if !strings.Contains(res.ForLLM, "step one") {
		t.Errorf("get missing current plan:\n%s", res.ForLLM)
	}
}

func TestPlanSetPreservesOtherMetadata(t *testing.T) {
	t.Parallel()
	st := newPlanTestStore()
	key := "agent:tester:ws:direct:plan-session"
	st.SetSessionMetadata(planContext(), key, map[string]string{"chat_mode": "dev"})

	tool := NewPlanTool()
	tool.SetSessionStore(st)
	tool.Execute(planContext(), map[string]any{"action": "set", "items": []any{"step"}})

	meta := st.metadata[key]
	if meta["chat_mode"] != "dev" {
		t.Errorf("chat_mode clobbered: %q", meta["chat_mode"])
	}
	if meta[planMetaKey] == "" {
		t.Error("plan key not persisted")
	}
}

func TestPlanSetValidation(t *testing.T) {
	t.Parallel()
	st := newPlanTestStore()
	tool := NewPlanTool()
	tool.SetSessionStore(st)

	cases := []struct {
		name string
		args map[string]any
	}{
		{"missing items", map[string]any{"action": "set"}},
		{"empty items", map[string]any{"action": "set", "items": []any{}}},
		{"non-string item", map[string]any{"action": "set", "items": []any{"ok", float64(2)}}},
		{"blank item", map[string]any{"action": "set", "items": []any{"ok", "   "}}},
	}
	for _, tc := range cases {
		if res := tool.Execute(planContext(), tc.args); !res.IsError {
			t.Errorf("%s: expected error, got %q", tc.name, res.ForLLM)
		}
	}

	tooMany := make([]any, planMaxSteps+1)
	for i := range tooMany {
		tooMany[i] = "step"
	}
	if res := tool.Execute(planContext(), map[string]any{"action": "set", "items": tooMany}); !res.IsError {
		t.Error("too many items: expected error")
	}
}

func TestPlanUnknownActionAndNilStore(t *testing.T) {
	t.Parallel()
	tool := NewPlanTool()
	if res := tool.Execute(planContext(), map[string]any{"action": "set", "items": []any{"x"}}); !res.IsError {
		t.Error("nil session store should error")
	}

	st := newPlanTestStore()
	tool.SetSessionStore(st)
	if res := tool.Execute(planContext(), map[string]any{"action": "delete"}); !res.IsError {
		t.Error("unknown action should error")
	}
	if res := tool.Execute(context.Background(), map[string]any{"action": "get"}); !res.IsError {
		t.Error("missing session context should error")
	}
}

// Compile-time guard: the store satisfies the interfaces the tool relies on.
var (
	_ store.SessionStore = (*planTestStore)(nil)
)
