package skills

import (
	"errors"
	"reflect"
	"testing"
)

// depFixture builds a SKILL.md with frontmatter declaring version/deps/conflicts.
func depFixture(version string, depends, requires, conflicts []string) string {
	fm := "---\n"
	if version != "" {
		fm += "version: " + version + "\n"
	}
	writeList := func(key string, items []string) {
		if len(items) == 0 {
			return
		}
		fm += key + ":\n"
		for _, item := range items {
			fm += "  - " + item + "\n"
		}
	}
	writeList("depends", depends)
	writeList("requires", requires)
	writeList("conflicts", conflicts)
	fm += "---\n\n# Body\n"
	return fm
}

// universe is a lookup backed by slug → (content, version).
type universe map[string]struct {
	content string
	version string
}

func (u universe) lookup(slug string) (string, string, error) {
	e, ok := u[slug]
	if !ok {
		return "", "", ErrSkillNotFound
	}
	return e.content, e.version, nil
}

func installUniverse(u universe) func(string) (string, string, error) {
	return func(slug string) (string, string, error) {
		e, ok := u[slug]
		if !ok {
			return "", "", ErrSkillNotFound
		}
		return e.content, e.version, ErrSkillInstalled
	}
}

func TestParseDepends_BlockLists(t *testing.T) {
	content := "---\nname: app\ndepends:\n  - review-pr\n  - plan@>=1.2\nrequires:\n  - cook@==2.0.0\nconflicts:\n  - legacy-cook\n---\n\nbody"
	deps, conflicts, err := ParseDepends(content)
	if err != nil {
		t.Fatalf("ParseDepends: %v", err)
	}
	wantDeps := []string{"review-pr", "plan@>=1.2", "cook@==2.0.0"}
	if !reflect.DeepEqual(deps, wantDeps) {
		t.Fatalf("deps = %v, want %v", deps, wantDeps)
	}
	if !reflect.DeepEqual(conflicts, []string{"legacy-cook"}) {
		t.Fatalf("conflicts = %v, want [legacy-cook]", conflicts)
	}
}

func TestParseDepends_ScalarAndMixedForms(t *testing.T) {
	content := "---\nname: app\ndepends: plan, review-pr\nrequires: cook\n---\nbody"
	deps, conflicts, err := ParseDepends(content)
	if err != nil {
		t.Fatalf("ParseDepends: %v", err)
	}
	wantDeps := []string{"plan", "review-pr", "cook"}
	if !reflect.DeepEqual(deps, wantDeps) {
		t.Fatalf("deps = %v, want %v", deps, wantDeps)
	}
	if conflicts != nil {
		t.Fatalf("conflicts = %v, want nil", conflicts)
	}
}

func TestParseDepends_NoFrontmatter(t *testing.T) {
	deps, conflicts, err := ParseDepends("just markdown, no frontmatter")
	if err != nil || deps != nil || conflicts != nil {
		t.Fatalf("ParseDepends(no frontmatter) = (%v, %v, %v), want nils", deps, conflicts, err)
	}
}

func TestParseDepends_InvalidConstraint(t *testing.T) {
	for _, ref := range []string{"plan@~1.2", "plan@latest", "plan@", "plan extra"} {
		content := "---\ndepends:\n  - " + ref + "\n---\nbody"
		if _, _, err := ParseDepends(content); err == nil {
			t.Fatalf("ParseDepends(%q) = nil error, want invalid-reference error", ref)
		}
	}
	// Dash without space ("-plan") is outside the shared frontmatter list
	// grammar and is silently skipped by the loader's parser; pin that
	// documented behavior rather than an error.
	content := "---\ndepends:\n  - -plan\n---\nbody"
	if deps, _, _ := ParseDepends(content); len(deps) != 0 {
		t.Fatalf("deps = %v, want empty for dashless list item", deps)
	}
}

func TestResolve_ChainClosureTopologicalOrder(t *testing.T) {
	// a → b → c: installing a must order c before b before a.
	u := universe{
		"aaa": {depFixture("", []string{"bbb"}, nil, nil), ""},
		"bbb": {depFixture("", []string{"ccc"}, nil, nil), ""},
		"ccc": {depFixture("", nil, nil, nil), ""},
	}
	plan, err := Resolve([]string{"aaa"}, u.lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(plan.Install, []string{"ccc", "bbb", "aaa"}) {
		t.Fatalf("Install = %v, want [c b a]", plan.Install)
	}
	if len(plan.Missing)+len(plan.Conflicts)+len(plan.Cycles) != 0 {
		t.Fatalf("unexpected findings: %+v", plan)
	}
}

func TestResolve_DiamondDedup(t *testing.T) {
	// a → b,c; both b and c → d: d installed exactly once, first.
	u := universe{
		"aaa": {depFixture("", []string{"bbb", "ccc"}, nil, nil), ""},
		"bbb": {depFixture("", []string{"ddd"}, nil, nil), ""},
		"ccc": {depFixture("", []string{"ddd"}, nil, nil), ""},
		"ddd": {depFixture("", nil, nil, nil), ""},
	}
	plan, err := Resolve([]string{"aaa"}, u.lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := []string{"ddd", "bbb", "ccc", "aaa"}
	if !reflect.DeepEqual(plan.Install, want) {
		t.Fatalf("Install = %v, want %v", plan.Install, want)
	}
}

func TestResolve_MissingChainAttributesRequirers(t *testing.T) {
	// a → b → missing-c, plus a → missing-c directly: RequiredBy merges both.
	u := universe{
		"aaa": {depFixture("", []string{"bbb", "missing-c"}, nil, nil), ""},
		"bbb": {depFixture("", []string{"missing-c"}, nil, nil), ""},
	}
	plan, err := Resolve([]string{"aaa"}, u.lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantMissing := []MissingDep{{Slug: "missing-c", RequiredBy: []string{"aaa", "bbb"}}}
	if !reflect.DeepEqual(plan.Missing, wantMissing) {
		t.Fatalf("Missing = %+v, want %+v", plan.Missing, wantMissing)
	}
	// Every subtree is broken; nothing may be queued for install.
	if len(plan.Install) != 0 {
		t.Fatalf("Install = %v, want empty", plan.Install)
	}
}

func TestResolve_RootMissingHasEmptyRequiredBy(t *testing.T) {
	u := universe{"present": {depFixture("", nil, nil, nil), ""}}
	plan, err := Resolve([]string{"ghost", "present"}, u.lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := []MissingDep{{Slug: "ghost"}}
	if !reflect.DeepEqual(plan.Missing, want) {
		t.Fatalf("Missing = %+v, want %+v", plan.Missing, want)
	}
	if !reflect.DeepEqual(plan.Install, []string{"present"}) {
		t.Fatalf("Install = %v, want [present]", plan.Install)
	}
}

func TestResolve_ConflictWithinClosure(t *testing.T) {
	u := universe{
		"aaa":   {depFixture("", []string{"bbb"}, nil, []string{"ccc"}), ""},
		"bbb":   {depFixture("", nil, nil, nil), ""},
		"ccc":   {depFixture("", nil, nil, nil), ""},
		"far": {depFixture("", nil, nil, []string{"ccc"}), ""},
	}
	plan, err := Resolve([]string{"aaa", "ccc"}, u.lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(plan.Conflicts, []string{"ccc"}) {
		t.Fatalf("Conflicts = %v, want [c]", plan.Conflicts)
	}
	// Conflicting declaration outside the closure is not reported.
	plan2, err := Resolve([]string{"aaa"}, u.lookup)
	if err != nil {
		t.Fatalf("Resolve(a): %v", err)
	}
	if len(plan2.Conflicts) != 0 {
		t.Fatalf("Conflicts = %v, want none when c absent from closure", plan2.Conflicts)
	}
}

func TestResolve_SelfCycle(t *testing.T) {
	u := universe{"selfish": {depFixture("", []string{"selfish"}, nil, nil), ""}}
	plan, err := Resolve([]string{"selfish"}, u.lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantCycles := [][]string{{"selfish"}}
	if !reflect.DeepEqual(plan.Cycles, wantCycles) {
		t.Fatalf("Cycles = %v, want %v", plan.Cycles, wantCycles)
	}
	if len(plan.Install) != 0 {
		t.Fatalf("Install = %v, want empty for cyclic root", plan.Install)
	}
}

func TestResolve_TwoNodeCycle(t *testing.T) {
	u := universe{
		"xxx": {depFixture("", []string{"yyy"}, nil, nil), ""},
		"yyy": {depFixture("", []string{"xxx"}, nil, nil), ""},
		"zzz": {depFixture("", []string{"xxx"}, nil, nil), ""},
	}
	plan, err := Resolve([]string{"zzz"}, u.lookup)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantCycles := [][]string{{"xxx", "yyy"}}
	if !reflect.DeepEqual(plan.Cycles, wantCycles) {
		t.Fatalf("Cycles = %v, want [[x y]]", plan.Cycles)
	}
	if len(plan.Install) != 0 {
		t.Fatalf("Install = %v, want empty — z depends on a cycle", plan.Install)
	}
}

func TestResolve_ExactPinSatisfiedAndViolated(t *testing.T) {
	base := universe{
		"app":    {depFixture("", nil, []string{"lib@==1.2.3"}, nil), "1.0.0"},
		"lib":    {depFixture("1.2.3", nil, nil, nil), "1.2.3"},
		"appbad": {depFixture("", nil, []string{"lib@==9.9.9"}, nil), "1.0.0"},
	}
	plan, err := Resolve([]string{"app"}, base.lookup)
	if err != nil {
		t.Fatalf("Resolve(app): %v", err)
	}
	if !reflect.DeepEqual(plan.Install, []string{"lib", "app"}) {
		t.Fatalf("Install = %v, want [lib app]", plan.Install)
	}
	plan, err = Resolve([]string{"appbad"}, base.lookup)
	if err != nil {
		t.Fatalf("Resolve(appbad): %v", err)
	}
	want := []MissingDep{{Slug: "lib", RequiredBy: []string{"appbad"}}}
	if !reflect.DeepEqual(plan.Missing, want) {
		t.Fatalf("Missing = %+v, want %+v (unsatisfiable pin)", plan.Missing, want)
	}
	if len(plan.Install) != 0 {
		t.Fatalf("Install = %v, want empty under unsatisfiable pin", plan.Install)
	}
}

func TestResolve_MinimumPinBoundary(t *testing.T) {
	cases := []struct {
		libVersion string
		satisfies  bool
	}{
		{"1.1", false},
		{"1.2", true},   // boundary inclusive
		{"1.2.5", true}, // patch-level above
	}
	for _, tc := range cases {
		u := universe{
			"app": {depFixture("", nil, []string{"lib@>=1.2"}, nil), "1.0.0"},
			"lib": {depFixture(tc.libVersion, nil, nil, nil), tc.libVersion},
		}
		plan, err := Resolve([]string{"app"}, u.lookup)
		if err != nil {
			t.Fatalf("Resolve(lib=%s): %v", tc.libVersion, err)
		}
		if tc.satisfies && len(plan.Missing) != 0 {
			t.Fatalf("lib %s should satisfy >=1.2, got Missing %+v", tc.libVersion, plan.Missing)
		}
		if !tc.satisfies && len(plan.Missing) != 1 {
			t.Fatalf("lib %s should NOT satisfy >=1.2, got Missing %+v", tc.libVersion, plan.Missing)
		}
	}
}

func TestResolve_AlreadySatisfiedSkipsInstallButStillResolvesDeps(t *testing.T) {
	// Both b and its dependency a are installed; neither re-installs, but
	// both still appear as satisfied and constraints keep resolving.
	u := universe{
		"bbb": {depFixture("", []string{"aaa"}, nil, nil), "2.0.0"},
		"aaa": {depFixture("", nil, nil, nil), "1.0.0"},
	}
	plan, err := Resolve([]string{"bbb"}, installUniverse(u))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(plan.AlreadySatisfied, []string{"aaa", "bbb"}) {
		t.Fatalf("AlreadySatisfied = %v, want [a b]", plan.AlreadySatisfied)
	}
	if len(plan.Install) != 0 {
		t.Fatalf("Install = %v, want empty — everything already installed", plan.Install)
	}
}

func TestResolve_LookupErrorAborts(t *testing.T) {
	boom := errors.New("disk on fire")
	plan, err := Resolve([]string{"app"}, func(slug string) (string, string, error) {
		return "", "", boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped %v", err, boom)
	}
	if plan.Install != nil {
		t.Fatalf("plan returned on error: %+v", plan)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2", "1.2.0", 0},
		{"1.10", "1.9", 1}, // numeric, not lexicographic
		{"1.2", "1.10", -1},
		{"2", "1.9.9", 1},
		{"1.2.3-beta", "1.2.3", 0}, // suffix ignored in comparison
	}
	for _, tc := range cases {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestVersionSatisfies_InvalidConstraintFalse(t *testing.T) {
	if versionSatisfies("1.0", "~1.0") {
		t.Fatal("~1.0 must never satisfy")
	}
	if !versionSatisfies("anything", "") {
		t.Fatal("empty constraint accepts everything")
	}
}
