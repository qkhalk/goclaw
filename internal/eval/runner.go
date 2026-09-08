package eval

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Driver executes the cases of one suite. Drivers own their identity
// namespace: every suite run gets a unique suffix (RunID) so logical names in
// YAML (alice, bob) map to per-run unique store identities.
type Driver interface {
	Name() string
	// Setup prepares a fresh execution environment for the suite (per-run
	// tenant, temp workspace, ...). The returned cleanup runs after the last
	// case regardless of pass/fail.
	Setup(env *Env, runID string) (func(), error)
	// Run executes one case and returns a failure message, or "" on pass.
	Run(ctx context.Context, env *Env, runID string, c EvalCase) (detail string, err error)
}

var drivers = map[string]Driver{}

// RegisterDriver makes a driver available to suites. Called from init() in
// each driver file.
func RegisterDriver(d Driver) {
	drivers[d.Name()] = d
}

// ListSuites parses and validates all suite files without executing them.
// Used by `goclaw eval list` for a dry-run view of what would run.
func ListSuites(dir string) ([]SuiteReport, error) {
	if dir == "" {
		dir = "evals"
	}
	files, err := discoverSuites(dir)
	if err != nil {
		return nil, err
	}
	var out []SuiteReport
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			return nil, fmt.Errorf("read suite %s: %w", f.path, err)
		}
		suite, err := parseSuite(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.path, err)
		}
		if _, ok := drivers[suite.Driver]; !ok {
			return nil, fmt.Errorf("%s: unknown driver %q", f.path, suite.Driver)
		}
		out = append(out, SuiteReport{Suite: suite.Suite, Driver: suite.Driver, Total: len(suite.Cases)})
	}
	return out, nil
}

// RunOptions selects what to execute.
type RunOptions struct {
	// Dir is the evals root (default "evals").
	Dir string
	// Suites filters by suite name; empty runs all.
	Suites []string
	// OnlyFailures suppresses PASS lines in the report.
	OnlyFailures bool
}

// Run discovers suite files, executes them and returns per-suite reports.
// It returns an error only for harness-level failures (bad YAML, unknown
// driver, env failure) — case failures are captured in the reports.
func Run(env *Env, opts RunOptions) ([]SuiteReport, error) {
	dir := opts.Dir
	if dir == "" {
		dir = "evals"
	}
	files, err := discoverSuites(dir)
	if err != nil {
		return nil, err
	}

	var reports []SuiteReport
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			return nil, fmt.Errorf("read suite %s: %w", f.path, err)
		}
		suite, err := parseSuite(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.path, err)
		}
		if len(opts.Suites) > 0 && !contains(opts.Suites, suite.Suite) {
			continue
		}
		drv, ok := drivers[suite.Driver]
		if !ok {
			return nil, fmt.Errorf("%s: unknown driver %q", f.path, suite.Driver)
		}

		runID := newRunID()
		cleanup, err := drv.Setup(env, runID)
		if err != nil {
			return nil, fmt.Errorf("suite %s setup: %w", suite.Suite, err)
		}

		report := SuiteReport{Suite: suite.Suite, Driver: suite.Driver, Total: len(suite.Cases)}
		suiteStart := time.Now()
		for _, c := range suite.Cases {
			res := CaseResult{Suite: suite.Suite, Name: c.Name, Severity: c.Severity}
			slog.Debug("eval.case_start", "suite", suite.Suite, "case", c.Name)
			start := time.Now()
			// One case must never poison the next: a driver panic surfaces as
			// that case's failure, not a harness crash.
			detail, err := safeRun(drv, env, runID, c)
			res.Duration = time.Since(start)
			res.Detail, res.Err = detail, errString(err)
			res.Passed = err == nil
			if res.Passed {
				report.Passed++
			} else {
				report.Failed++
			}
			report.Results = append(report.Results, res)
		}
		report.Duration = time.Since(suiteStart)
		cleanup()

		reports = append(reports, report)
	}
	return reports, nil
}

// safeRun isolates driver panics into case failures.
func safeRun(d Driver, env *Env, runID string, c EvalCase) (detail string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("driver panic: %v", r)
		}
	}()
	return d.Run(context.Background(), env, runID, c)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// newRunID returns a short unique suffix for per-run identities. UUID-based:
// a time-derived suffix repeats on fast successive runs and would silently
// merge two runs' identities.
func newRunID() string {
	return uuid.NewString()[:8]
}

type suiteFile struct {
	path string
}

// discoverSuites walks dir (expected layout evals/<category>/<name>.yaml) and
// returns files in deterministic order.
func discoverSuites(dir string) ([]suiteFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("evals dir %s not found (run from repo root or pass --dir): %w", dir, err)
	}
	var out []suiteFile
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		files, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || (!strings.HasSuffix(f.Name(), ".yaml") && !strings.HasSuffix(f.Name(), ".yml")) {
				continue
			}
			names = append(names, filepath.Join(sub, f.Name()))
		}
	}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, suiteFile{path: n})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no .yaml suites found under %s", dir)
	}
	return out, nil
}
