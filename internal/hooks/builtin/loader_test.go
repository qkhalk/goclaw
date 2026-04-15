package builtin

import (
	"testing"

	"github.com/google/uuid"
)

// TestLoad_EmptyYaml ensures the Phase 04 ship state (empty builtins list +
// _placeholder.js) loads without error and leaves the registry empty.
func TestLoad_EmptyYaml(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := RegisteredSpecs(); len(got) != 0 {
		t.Errorf("registered=%d, want 0 (empty builtins.yaml)", len(got))
	}
}

// TestBuiltinNamespace_Stable guards against a silent namespace change that
// would orphan every already-seeded row at the next boot.
func TestBuiltinNamespace_Stable(t *testing.T) {
	want := uuid.MustParse("082ab084-a25f-52b4-a4a4-eb8a816bd9a8")
	if BuiltinNamespace != want {
		t.Fatalf("namespace drifted: have %s, want %s", BuiltinNamespace, want)
	}
}

// TestBuiltinID_Deterministic locks in the per-id hash so that a renamed id
// gets caught in review — downgrade-safety depends on stable IDs.
func TestBuiltinID_Deterministic(t *testing.T) {
	a := BuiltinID("pii-redactor")
	b := BuiltinID("pii-redactor")
	if a != b {
		t.Fatalf("BuiltinID non-deterministic: %s vs %s", a, b)
	}
	c := BuiltinID("sql-guard")
	if a == c {
		t.Fatal("distinct ids produced equal UUID — namespace collision")
	}
}

// TestBuiltinEventID_Distinct ensures each event of a multi-event spec gets
// its own row id so per-event enabled toggles don't clobber siblings.
func TestBuiltinEventID_Distinct(t *testing.T) {
	a := BuiltinEventID("pii-redactor", "user_prompt_submit")
	b := BuiltinEventID("pii-redactor", "pre_tool_use")
	if a == b {
		t.Fatal("event UUIDs collided for different events")
	}
	if a == BuiltinID("pii-redactor") {
		t.Fatal("per-event UUID must differ from bare spec id")
	}
}

// TestAllowlistFor_Unknown returns nil for ids that aren't registered.
// Dispatcher treats nil as strip → defense-in-depth against scripts with
// forged source='builtin' on a random UUID.
func TestAllowlistFor_Unknown(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := AllowlistFor(uuid.New()); got != nil {
		t.Errorf("unknown id → allowlist=%v, want nil", got)
	}
}

func TestIsBuiltin_Unknown(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if IsBuiltin(uuid.New()) {
		t.Error("random UUID reported as builtin")
	}
}

// TestMetadataVersion covers the JSONB float64 decode path plus int fast path.
func TestMetadataVersion(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		want int
	}{
		{"nil", nil, 0},
		{"missing", map[string]any{}, 0},
		{"int", map[string]any{"version": 3}, 3},
		{"int64", map[string]any{"version": int64(4)}, 4},
		{"float64", map[string]any{"version": float64(5)}, 5},
		{"string-ignored", map[string]any{"version": "oops"}, 0},
	}
	for _, tc := range cases {
		if got := metadataVersion(tc.meta); got != tc.want {
			t.Errorf("%s: got %d want %d", tc.name, got, tc.want)
		}
	}
}
