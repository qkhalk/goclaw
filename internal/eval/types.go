// Package eval implements the GoClaw agent evaluation harness: deterministic,
// store-level behavioral suites (memory isolation, run resume, tool security)
// declared in YAML under evals/ and executed by `goclaw eval run`.
//
// Evals differ from tests in what they guarantee: a test pins an
// implementation detail, an eval pins a user-visible behavior ("another user's
// memory must never surface for me") and reports a per-category score so
// regressions are visible at a glance (`make eval`).
package eval

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EvalSuite is one YAML file under evals/<category>/. Every case in a file
// runs against the same driver with a shared per-run identity suffix so
// repeated executions never collide with earlier runs.
type EvalSuite struct {
	Suite  string     `yaml:"suite"`
	Driver string     `yaml:"driver"`
	Cases  []EvalCase `yaml:"cases"`
}

// EvalCase is one behavioral check. Fields beyond Name/Description/Severity
// are driver-specific; see driver_memory.go, driver_resume.go and
// driver_security.go for the shapes each driver accepts.
type EvalCase struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Severity    string `yaml:"severity"` // P0 (blocking) | P1

	// memory driver
	Seed     *MemorySeed     `yaml:"seed,omitempty"`
	Act      *MemoryAct      `yaml:"act,omitempty"`
	Expect   *MemoryExpect   `yaml:"expect,omitempty"`
	Positive *PositiveControl `yaml:"positive,omitempty"`

	// resume driver
	Scenario string `yaml:"scenario,omitempty"`

	// security driver
	Command      string `yaml:"command,omitempty"`
	Path         string `yaml:"path,omitempty"`
	ExpectDenied bool   `yaml:"expect_denied"`
}

// MemorySeed declares memories to write before the act step. Identities
// (user/agent/session) are logical names unique'd per run by the driver.
type MemorySeed struct {
	Memories []MemorySeedItem `yaml:"memories"`
}

type MemorySeedItem struct {
	User      string  `yaml:"user"`
	Agent     string  `yaml:"agent"`
	Session   string  `yaml:"session"`
	Scope     string  `yaml:"scope"`
	Kind      string  `yaml:"kind"`
	Content   string  `yaml:"content"`
	Source    string  `yaml:"source"`              // session|manual|consolidation|import
	Confidence float64 `yaml:"confidence,omitempty"`
}

// MemoryAct is the retrieval being evaluated.
type MemoryAct struct {
	Search MemoryQuerySpec `yaml:"search"`
}

// MemoryQuerySpec mirrors store.MemoryQuery with logical identity names.
type MemoryQuerySpec struct {
	User    string   `yaml:"user"`
	Agent   string   `yaml:"agent"`
	Session string   `yaml:"session"`
	Scopes  []string `yaml:"scopes"`
	Kinds   []string `yaml:"kinds"`
	Limit   int      `yaml:"limit"`
	// CrossTenant runs the search from a second tenant's context. Only
	// meaningful on the act step: it proves the tenant hard gate excludes
	// rows seeded under the primary tenant.
	CrossTenant bool `yaml:"cross_tenant"`
}

// MemoryExpect asserts on the concatenated content of the act's results.
// Contains/NotContains are substring checks; MinResults/MaxResults bound the
// result count so "nothing returned" cannot silently pass a leak check that
// should have had a positive baseline (pair cases with `positive`).
type MemoryExpect struct {
	Contains    []string `yaml:"contains"`
	NotContains []string `yaml:"not_contains"`
	MinResults  int      `yaml:"min_results"`
	MaxResults  int      `yaml:"max_results"`
}

// PositiveControl runs the same retrieval for the owning identity and must
// return the seeded content. A failing positive control fails the case: it
// proves the negative assertion passed only because nothing was stored.
type PositiveControl struct {
	Search   MemoryQuerySpec `yaml:"search"`
	Contains []string        `yaml:"contains"`
}

// CaseResult is the outcome of one EvalCase.
type CaseResult struct {
	Suite     string
	Name      string
	Severity  string
	Passed    bool
	Err       string
	Detail    string
	Duration  time.Duration
}

// SuiteReport aggregates CaseResults per suite for the score table.
type SuiteReport struct {
	Suite    string
	Driver   string
	Total    int
	Passed   int
	Failed   int
	Duration time.Duration
	Results  []CaseResult
}

// Score returns the pass percentage for the suite.
func (r *SuiteReport) Score() int {
	if r.Total == 0 {
		return 0
	}
	return int(float64(r.Passed) / float64(r.Total) * 100)
}

// parseSuite decodes one YAML suite file and applies cross-driver validation.
func parseSuite(data []byte) (*EvalSuite, error) {
	var s EvalSuite
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if s.Suite == "" {
		return nil, fmt.Errorf("suite name is required")
	}
	if s.Driver == "" {
		return nil, fmt.Errorf("driver is required")
	}
	if len(s.Cases) == 0 {
		return nil, fmt.Errorf("at least one case is required")
	}
	seen := map[string]bool{}
	for i := range s.Cases {
		c := &s.Cases[i]
		if c.Name == "" {
			return nil, fmt.Errorf("case %d: name is required", i+1)
		}
		if seen[c.Name] {
			return nil, fmt.Errorf("case %d: duplicate name %q", i+1, c.Name)
		}
		seen[c.Name] = true
		if c.Severity == "" {
			c.Severity = "P1"
		}
		if !strings.EqualFold(c.Severity, "P0") && !strings.EqualFold(c.Severity, "P1") {
			return nil, fmt.Errorf("case %q: severity must be P0 or P1", c.Name)
		}
	}
	return &s, nil
}
