package workflow

// YAML bridge: declarative workflow files -> the native DAG executor.
//
// The executor in this package (dag.go/executor.go) is Go-native: steps are
// SpecFunc closures. This file adds the declaration layer so operators can
// define a multi-agent pipeline in YAML and the gateway/CLI can build the
// same DAG without Go code.
//
// Step fields map as:
//
//	id, type, deps            -> Step.ID / Type / Deps (topological order)
//	agent, prompt             -> Run closure calling AgentRunner.RunAgent;
//	                             prompt renders {{input}} and {{step:<id>}}
//	                             from the RunCtx outputs of earlier steps
//	timeout_seconds           -> Step.Timeout (per attempt)
//	retry.max_attempts,       -> Step.Retry
//	retry.backoff_ms
//	on_error.agent/prompt     -> Step.OnError (runs the named agent; a nil
//	                             error return swallows the step failure)
//	when.contains_of          -> StepConditional Cond: run only when the
//	                             output of an earlier step contains text
//
// The runner is injected by the caller (gateway wiring maps an agent key to
// a real execution - native loop, ACP subprocess, remote worker), keeping
// the DSL free of provider knowledge.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentRunner executes one agent step. Gateway wiring maps an agent key to a
// real execution.
type AgentRunner interface {
	RunAgent(ctx context.Context, agentKey, prompt string) (string, error)
}

// workflowYAML is the raw file shape.
type workflowYAML struct {
	Name  string      `yaml:"name"`
	Input string      `yaml:"input"`
	Steps []stepYAML  `yaml:"steps"`
}

type stepYAML struct {
	ID             string        `yaml:"id"`
	Type           string        `yaml:"type,omitempty"` // sequential (default) | parallel | conditional | retry
	Agent          string        `yaml:"agent,omitempty"`
	Prompt         string        `yaml:"prompt,omitempty"`
	Deps           []string      `yaml:"deps,omitempty"`
	TimeoutSeconds int           `yaml:"timeout_seconds,omitempty"`
	Retry          *retryYAML    `yaml:"retry,omitempty"`
	OnError        *onErrorYAML  `yaml:"on_error,omitempty"`
	When           *whenYAML     `yaml:"when,omitempty"`
}

type retryYAML struct {
	MaxAttempts int `yaml:"max_attempts"`
	BackoffMs   int `yaml:"backoff_ms,omitempty"`
}

type onErrorYAML struct {
	Agent  string `yaml:"agent"`
	Prompt string `yaml:"prompt,omitempty"`
}

// whenYAML gates a conditional step on an earlier step's output content.
type whenYAML struct {
	OutputOf string `yaml:"output_of"`
	Contains string `yaml:"contains"`
}

// ParseWorkflowYAML validates the declaration and builds the executable DAG.
// The returned DAG runs against the given runner; RunAgent errors surface as
// step failures exactly like native SpecFunc errors.
func ParseWorkflowYAML(data []byte, runner AgentRunner) (*DAG, error) {
	if runner == nil {
		return nil, fmt.Errorf("workflow: nil agent runner")
	}
	var w workflowYAML
	if err := yaml.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("parse workflow yaml: %w", err)
	}
	if strings.TrimSpace(w.Name) == "" {
		return nil, fmt.Errorf("workflow: name is required")
	}
	if len(w.Steps) == 0 {
		return nil, fmt.Errorf("workflow: at least one step is required")
	}

	seen := map[string]bool{}
	for i, s := range w.Steps {
		if strings.TrimSpace(s.ID) == "" {
			return nil, fmt.Errorf("workflow: steps[%d] id is required", i)
		}
		if seen[s.ID] {
			return nil, fmt.Errorf("workflow: duplicate step id %q", s.ID)
		}
		seen[s.ID] = true
	}

	d := NewDAG(w.Name)
	for i, s := range w.Steps {
		step, err := buildStep(w, s, i, runner)
		if err != nil {
			return nil, err
		}
		if err := d.AddStep(step); err != nil {
			return nil, fmt.Errorf("workflow: steps[%d]: %w", i, err)
		}
	}
	// Structural checks (cycles, unknown deps) before handing the DAG out.
	if _, err := d.TopoOrder(); err != nil {
		return nil, fmt.Errorf("workflow %q: %w", w.Name, err)
	}
	return d, nil
}

func buildStep(w workflowYAML, s stepYAML, i int, runner AgentRunner) (*Step, error) {
	st := &Step{ID: s.ID}
	switch s.Type {
	case "", "sequential":
		st.Type = StepSequential
	case "parallel":
		st.Type = StepParallel
	case "conditional":
		st.Type = StepConditional
	case "retry":
		st.Type = StepRetry
	default:
		return nil, fmt.Errorf("steps[%d] (%s): unknown type %q", i, s.ID, s.Type)
	}
	st.Deps = s.Deps
	if s.TimeoutSeconds > 0 {
		st.Timeout = time.Duration(s.TimeoutSeconds) * time.Second
	}

	agent := s.Agent
	if agent == "" {
		return nil, fmt.Errorf("steps[%d] (%s): agent is required in the YAML DSL", i, s.ID)
	}
	prompt := renderInput(s.Prompt, w.Input)

	st.Run = func(ctx context.Context, rc *RunCtx) error {
		rendered := renderStepRefs(prompt, rc)
		output, err := runner.RunAgent(ctx, agent, rendered)
		if err != nil {
			return err
		}
		// The executor does not persist outputs; later templates
		// ({{step:<id>}}) and when-guards read them from the RunCtx.
		rc.SetOutput(s.ID, output)
		return nil
	}

	if s.Retry != nil {
		if s.Retry.MaxAttempts <= 0 {
			return nil, fmt.Errorf("steps[%d] (%s): retry.max_attempts must be positive", i, s.ID)
		}
		st.Retry = &RetryPolicy{MaxAttempts: s.Retry.MaxAttempts}
		if s.Retry.BackoffMs > 0 {
			st.Retry.Backoff = time.Duration(s.Retry.BackoffMs) * time.Millisecond
		}
	}

	if s.OnError != nil {
		if strings.TrimSpace(s.OnError.Agent) == "" {
			return nil, fmt.Errorf("steps[%d] (%s): on_error.agent is required", i, s.ID)
		}
		onErrAgent := s.OnError.Agent
		onErrPrompt := renderInput(s.OnError.Prompt, w.Input)
		st.OnError = func(ctx context.Context, rc *RunCtx, stepErr error) error {
			if _, runErr := runner.RunAgent(ctx, onErrAgent, renderStepRefs(onErrPrompt, rc)); runErr != nil {
				// The recovery agent itself failed: keep the original error.
				return stepErr
			}
			return nil // recovery succeeded: swallow the step failure
		}
	}

	if s.When != nil {
		if s.When.OutputOf == "" {
			return nil, fmt.Errorf("steps[%d] (%s): when.output_of is required", i, s.ID)
		}
		outputOf, needle := s.When.OutputOf, s.When.Contains
		st.Cond = func(_ context.Context, rc *RunCtx) bool {
			val, ok := rc.GetOutput(outputOf)
			if !ok {
				return false
			}
			return strings.Contains(fmt.Sprint(val), needle)
		}
	}
	return st, nil
}

// renderInput substitutes {{input}} in top-level templates.
func renderInput(prompt, input string) string {
	return strings.ReplaceAll(prompt, "{{input}}", input)
}

// renderStepRefs substitutes {{step:<id>}} with earlier step outputs.
func renderStepRefs(prompt string, rc *RunCtx) string {
	if !strings.Contains(prompt, "{{step:") {
		return prompt
	}
	out := prompt
	for key, val := range rc.Outputs() {
		out = strings.ReplaceAll(out, "{{step:"+key+"}}", fmt.Sprint(val))
	}
	return out
}
