package eval

import (
	"strings"
	"testing"
)

// TestListSuitesParsesRepoSuites validates every shipped suite file without a
// database: YAML shape, unique case names, known severities and drivers. This
// is the fast feedback loop for eval authoring (make eval-list equivalent).
func TestListSuitesParsesRepoSuites(t *testing.T) {
	suites, err := ListSuites("../../evals")
	if err != nil {
		t.Fatalf("ListSuites: %v", err)
	}
	byName := map[string]SuiteReport{}
	for _, s := range suites {
		if _, dup := byName[s.Suite]; dup {
			t.Errorf("duplicate suite name %q", s.Suite)
		}
		byName[s.Suite] = s
	}
	for _, want := range []string{"memory-isolation", "run-state-machine", "tool-security"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("suite %q missing from evals/", want)
		}
	}
	if s := byName["memory-isolation"]; s.Total < 5 {
		t.Errorf("memory-isolation has only %d cases — isolation matrix shrank", s.Total)
	}
}

func TestParseSuiteValidation(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing suite", "driver: memory\ncases:\n- name: a", "suite name is required"},
		{"missing driver", "suite: s\ncases:\n- name: a", "driver is required"},
		{"no cases", "suite: s\ndriver: memory\nverdicts: []", "at least one case"},
		{"dup case names", "suite: s\ndriver: memory\ncases:\n- name: a\n- name: a", "duplicate name"},
		{"bad severity", "suite: s\ndriver: memory\ncases:\n- name: a\n  severity: P9", "severity"},
		{"missing case name", "suite: s\ndriver: memory\ncases:\n- severity: P0", "name is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSuite([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parseSuite(%s) error = %v, want containing %q", tc.name, err, tc.want)
			}
		})
	}
}

func TestParseSuiteDefaults(t *testing.T) {
	s, err := parseSuite([]byte("suite: s\ndriver: memory\ncases:\n- name: a"))
	if err != nil {
		t.Fatalf("parseSuite: %v", err)
	}
	if s.Cases[0].Severity != "P1" {
		t.Fatalf("severity default = %q, want P1", s.Cases[0].Severity)
	}
}

func TestSuiteReportScore(t *testing.T) {
	r := SuiteReport{Total: 4, Passed: 3}
	if r.Score() != 75 {
		t.Fatalf("Score() = %d, want 75", r.Score())
	}
	empty := SuiteReport{}
	if empty.Score() != 0 {
		t.Fatal("empty suite score must be 0, not a divide-by-zero panic")
	}
}

func TestWordWrap(t *testing.T) {
	in := strings.Repeat("word ", 30)
	got := wordWrap(strings.TrimSpace(in), 40, "  ")
	for _, line := range strings.Split(got, "\n") {
		// Continuation lines carry the 2-space indent; nothing exceeds 42.
		if len(line) > 42 {
			t.Fatalf("wrapped line too long: %q", line)
		}
	}
	if wordWrap("short", 40, "  ") != "short" {
		t.Fatal("short messages must pass through unwrapped")
	}
}
