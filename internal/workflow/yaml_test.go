package workflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type mapRunner struct {
	mu      sync.Mutex
	calls   []string
	failKey map[string]bool
}

func (m *mapRunner) RunAgent(_ context.Context, agentKey, prompt string) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, agentKey+"|"+prompt)
	_, fail := m.failKey[agentKey]
	m.mu.Unlock()
	if fail {
		return "", errors.New("agent " + agentKey + " failed")
	}
	return "output-of(" + agentKey + ")", nil
}

const validYAML = `
name: build_feature
input: add checkout
steps:
  - id: plan
    agent: planner
    prompt: "draft a plan for {{input}}"
  - id: be
    type: parallel
    agent: backend
    prompt: "backend for {{step:plan}}"
    deps: [plan]
  - id: fe
    type: parallel
    agent: frontend
    prompt: "frontend for {{step:plan}}"
    deps: [plan]
  - id: review
    type: conditional
    deps: [be]
    when: {output_of: be, contains: "output-of"}
    agent: reviewer
    prompt: "review {{step:be}}"
  - id: tests
    type: retry
    agent: tester
    prompt: "test {{step:be}}"
    deps: [be]
    retry: {max_attempts: 3, backoff_ms: 1}
    on_error: {agent: fixer, prompt: "fix the build"}
`

func TestParseWorkflowYAMLBuildsRunnableDAG(t *testing.T) {
	runner := &mapRunner{}
	dag, err := ParseWorkflowYAML([]byte(validYAML), runner)
	if err != nil {
		t.Fatalf("ParseWorkflowYAML: %v", err)
	}
	if dag.Name() != "build_feature" {
		t.Fatalf("name = %q", dag.Name())
	}
	if err := dag.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	joined := strings.Join(runner.calls, "\n")
	for _, want := range []string{
		"planner|draft a plan for add checkout",
		"backend|backend for {{step:plan}}", // rendered from stored output below? no: substituted
	} {
		_ = want
	}
	// Prompt substitution: the plan output reached the backend prompt.
	if !strings.Contains(joined, "backend for output-of(planner)") {
		t.Fatalf("plan output not forwarded: %s", joined)
	}
	// Conditional gate passed (backend output contains "output-of").
	if !strings.Contains(joined, "reviewer|review output-of(backend)") {
		t.Fatalf("conditional step did not run: %s", joined)
	}
	// Retry step succeeded on first attempt (no fixer).
	for _, c := range runner.calls {
		if strings.HasPrefix(c, "fixer|") {
			t.Fatalf("fixer must not run when tests pass: %v", runner.calls)
		}
	}
}

func TestParseWorkflowYAMLRetryAndOnError(t *testing.T) {
	yaml := `
name: retry_flow
steps:
  - id: flaky
    type: retry
    agent: flaky
    prompt: go
    retry: {max_attempts: 3}
    on_error: {agent: fixer, prompt: fix it}
  - id: after
    agent: next
    prompt: continue
    deps: [flaky]
`
	runner := &mapRunner{failKey: map[string]bool{"flaky": true}}
	dag, err := ParseWorkflowYAML([]byte(yaml), runner)
	if err != nil {
		t.Fatalf("ParseWorkflowYAML: %v", err)
	}
	if err := dag.Run(context.Background()); err != nil {
		// The on_error recovery agent ran but also failed (fixer is not a
		// failing key) — recovery swallows the step failure, so Run must
		// succeed.
		t.Fatalf("on_error recovery should swallow the step failure: %v", err)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	flaky := 0
	fixer := 0
	for _, c := range runner.calls {
		if strings.HasPrefix(c, "flaky|") {
			flaky++
		}
		if strings.HasPrefix(c, "fixer|") {
			fixer++
		}
	}
	if flaky != 3 {
		t.Fatalf("flaky attempts = %d, want 3", flaky)
	}
	if fixer != 1 {
		t.Fatalf("fixer runs = %d, want 1", fixer)
	}
	// The downstream step still ran: recovery let the DAG continue.
	if len(runner.calls) < 4 {
		t.Fatalf("downstream step missing after recovery: %v", runner.calls)
	}
}

func TestParseWorkflowYAMLValidation(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing name", "steps:\n- id: a\n  agent: a\n  prompt: p", "name is required"},
		{"no steps", "name: w", "at least one step"},
		{"missing id", "name: w\nsteps:\n- agent: a\n  prompt: p", "id is required"},
		{"duplicate id", "name: w\nsteps:\n- id: a\n  agent: a\n  prompt: p\n- id: a\n  agent: b\n  prompt: p", "duplicate step id"},
		{"missing agent", "name: w\nsteps:\n- id: a\n  prompt: p", "agent is required"},
		{"unknown type", "name: w\nsteps:\n- id: a\n  type: banana\n  agent: a\n  prompt: p", "unknown type"},
		{"retry attempts", "name: w\nsteps:\n- id: a\n  type: retry\n  agent: a\n  prompt: p\n  retry: {max_attempts: 0}", "must be positive"},
		{"cycle", "name: w\nsteps:\n- id: a\n  agent: a\n  prompt: p\n  deps: [b]\n- id: b\n  agent: b\n  prompt: p\n  deps: [a]", "cycle"},
		{"unknown dep", "name: w\nsteps:\n- id: a\n  agent: a\n  prompt: p\n  deps: [ghost]", "unknown step"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseWorkflowYAML([]byte(tc.yaml), &mapRunner{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
	if _, err := ParseWorkflowYAML([]byte(validYAML), nil); err == nil || !strings.Contains(err.Error(), "nil agent runner") {
		t.Fatalf("nil runner error = %v", err)
	}
}
