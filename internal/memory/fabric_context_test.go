package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeFabricStore struct {
	results []store.ScoredMemory
	err     error
	query   store.MemoryQuery
}

func (f *fakeFabricStore) WriteMemory(context.Context, *store.Memory) error { return nil }
func (f *fakeFabricStore) GetMemory(context.Context, string) (*store.Memory, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeFabricStore) SearchMemories(_ context.Context, q store.MemoryQuery) ([]store.ScoredMemory, error) {
	f.query = q
	return f.results, f.err
}
func (f *fakeFabricStore) SupersedeMemory(context.Context, string, *store.Memory) error {
	return errors.New("not implemented")
}
func (f *fakeFabricStore) ArchiveMemory(context.Context, string) error {
	return errors.New("not implemented")
}

func mem(id, content string) *store.Memory {
	return &store.Memory{ID: id, Content: content, Scope: store.MemoryScopeUser, Kind: store.MemoryKindFact}
}

func TestBuildFabricContextProvenanceSuffix(t *testing.T) {
	ref := "session:sess_123"
	results := []store.ScoredMemory{
		{Memory: &store.Memory{ID: "1", Content: "User prefers React over Vue", Scope: store.MemoryScopeUser, Kind: store.MemoryKindPreference, SourceType: "session", SourceRef: &ref, Confidence: 0.94}, Score: 0.5},
		{Memory: &store.Memory{ID: "2", Content: "Standup is at 09:30", Scope: store.MemoryScopeUser, Kind: store.MemoryKindFact, SourceType: "manual"}, Score: 0.4},
	}
	f := &fakeFabricStore{results: results}
	res, err := BuildFabricContext(context.Background(), f, FabricContextParams{UserID: "u1"})
	if err != nil {
		t.Fatalf("BuildFabricContext: %v", err)
	}
	if res.Injected != 2 {
		t.Fatalf("injected = %d, want 2", res.Injected)
	}
	if !strings.Contains(res.Section, "(source: session:sess_123, confidence 0.94)") {
		t.Fatalf("missing session provenance: %s", res.Section)
	}
	// Manual-source rows render as "user", and the 0.8 write-default
	// confidence is omitted (it carries no signal).
	if !strings.Contains(res.Section, "(source: user)") {
		t.Fatalf("missing manual provenance: %s", res.Section)
	}
	if strings.Contains(res.Section, "confidence 0.80") {
		t.Fatalf("default confidence should be omitted: %s", res.Section)
	}
	// Identity fields are forwarded as the same hard gates.
	if f.query.UserID != "u1" {
		t.Fatalf("query user = %q", f.query.UserID)
	}
}

func TestBuildFabricContextEmptyAndErrors(t *testing.T) {
	res, err := BuildFabricContext(context.Background(), nil, FabricContextParams{})
	if err != nil || res.Section != "" {
		t.Fatalf("nil store = (%q, %v), want empty-no-error", res.Section, err)
	}
	f := &fakeFabricStore{err: errors.New("db down")}
	if _, err := BuildFabricContext(context.Background(), f, FabricContextParams{}); err == nil {
		t.Fatal("store error must propagate")
	}
	empty := &fakeFabricStore{}
	res, err = BuildFabricContext(context.Background(), empty, FabricContextParams{})
	if err != nil || res.Section != "" || res.Matched != 0 {
		t.Fatalf("empty results = (%+v, %v)", res, err)
	}
}

func TestBuildFabricContextTokenBudget(t *testing.T) {
	var results []store.ScoredMemory
	for i := 0; i < 50; i++ {
		results = append(results, store.ScoredMemory{
			Memory: mem(string(rune('a'+i%26))+"-"+string(rune('a'+i/26)), "very long fact number that takes tokens "+string(rune('a'+i%26))+string(rune('a'+i%26))+string(rune('a'+i%26))),
		})
	}
	f := &fakeFabricStore{results: results}
	res, err := BuildFabricContext(context.Background(), f, FabricContextParams{MaxTokens: 120})
	if err != nil {
		t.Fatalf("BuildFabricContext: %v", err)
	}
	if res.Injected == 0 || res.Injected >= 50 {
		t.Fatalf("injected = %d, want a budget-limited subset", res.Injected)
	}
}

func TestFilterContradictedMemories(t *testing.T) {
	a := mem("a", "favorite editor is Vim")
	b := &store.Memory{ID: "b", Content: "favorite editor is VS Code", Scope: store.MemoryScopeUser, Kind: store.MemoryKindFact, ContradictsID: strPtr("a")}
	c := mem("c", "unrelated fact")

	got := store.FilterContradictedMemories([]store.ScoredMemory{
		{Memory: a}, {Memory: b}, {Memory: c},
	})
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (a dropped)", len(got))
	}
	for _, r := range got {
		if r.ID == "a" {
			t.Fatal("contradicted row a must be dropped")
		}
	}

	// Contradiction pointing outside the result set drops nothing.
	dangling := &store.Memory{ID: "b", Content: "x", ContradictsID: strPtr("missing")}
	got = store.FilterContradictedMemories([]store.ScoredMemory{{Memory: mem("z", "kept")}, {Memory: dangling}})
	if len(got) != 2 {
		t.Fatalf("dangling contradiction must not drop rows, got %d", len(got))
	}

	// Tiny inputs pass through untouched.
	in := []store.ScoredMemory{{Memory: mem("only", "kept")}}
	if got := store.FilterContradictedMemories(in); len(got) != 1 {
		t.Fatalf("single row = %d, want 1", len(got))
	}
}

func strPtr(s string) *string { return &s }
