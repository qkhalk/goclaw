package qualitygate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// Defaults for the shell-out gates. The test gate's 10m ceiling mirrors the
// CI unit-test timeout; operators override per gate via timeout_ms.
const (
	defaultTestCommand  = "go test ./..."
	defaultBuildCommand = "go build ./..."
	defaultLintCommand  = "go vet ./..."
	defaultTimeout      = 10 * time.Minute
	detailTailLimit     = 2000 // chars of captured output kept in GateResult.Detail
)

// CommandRunner executes a validated command inside dir and returns its
// combined output plus exit status. *exec.Cmd satisfies it; tests substitute
// fakes to avoid real subprocesses.
type CommandRunner interface {
	Run(ctx context.Context, dir, command string) (output string, err error)
}

// execCommandRunner shells out through os/exec with the context deadline
// already applied by runCommand.
type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, dir, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	// Kill the whole process group on timeout: `sh -c` wrappers keep running
	// children (sleep, go test) alive after the shell itself is killed, which
	// would leave the gate blocked on cmd.Run until the child exits.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if p := cmd.Process; p != nil {
			_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
		}
		return nil
	}
	err := cmd.Run()
	return buf.String(), err
}

// commandGate is the shared implementation for test/build/lint: validate →
// deny-check → run under timeout → pass iff exit 0.
type commandGate struct {
	gateType       string
	defaultCommand string
	timeout        time.Duration
	runner         CommandRunner
}

func (g commandGate) Evaluate(ctx context.Context, spec GateSpec, subject Subject) GateResult {
	command := paramString(spec.Params, "command", g.defaultCommand)
	if strings.TrimSpace(command) == "" {
		return result(spec, false, g.gateType+" gate: empty command")
	}
	if reason := denyReason(ctx, command); reason != "" {
		return result(spec, false, fmt.Sprintf("%s gate: command denied by shell policy: %s", g.gateType, reason))
	}
	dir := subject.Workspace
	if d := paramString(spec.Params, "dir", ""); d != "" {
		dir = d
	}
	timeout := g.timeout
	if ms := paramInt64(spec.Params, "timeout_ms", 0); ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	output, err := runCommand(ctx, g.runnerOrDefault(), dir, command, timeout)
	if err != nil {
		return result(spec, false, summarizeOutput(g.gateType+" gate failed", output, err))
	}
	return result(spec, true, summarizeOutput(g.gateType+" gate passed", output, nil))
}

func (g commandGate) runnerOrDefault() CommandRunner {
	if g.runner != nil {
		return g.runner
	}
	return execCommandRunner{}
}

// fileGate checks existence and optional regex content match over Subject
// (or param) paths. Multiple paths must ALL hold for the gate to pass.
type fileGate struct{}

func (fileGate) Evaluate(_ context.Context, spec GateSpec, subject Subject) GateResult {
	paths := paramPathList(spec.Params)
	if len(paths) == 0 && len(subject.Artifacts) > 0 {
		paths = subject.Artifacts
	}
	if len(paths) == 0 {
		return result(spec, false, "file gate: no path/paths param and no subject artifacts")
	}
	base := subject.Workspace
	for _, p := range paths {
		full := resolvePath(base, p)
		data, err := os.ReadFile(full)
		if err != nil {
			return result(spec, false, fmt.Sprintf("file gate: %s unavailable: %v", displayPath(base, full), err))
		}
		if pattern := paramString(spec.Params, "contains_regex", ""); pattern != "" {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return result(spec, false, fmt.Sprintf("file gate: invalid contains_regex: %v", err))
			}
			if !re.Match(data) {
				return result(spec, false, fmt.Sprintf("file gate: %s does not match %q", displayPath(base, full), pattern))
			}
		}
	}
	return result(spec, true, fmt.Sprintf("file gate: %d path(s) verified", len(paths)))
}

// artifactGate requires the Subject's declared artifact paths to exist after
// the run (existence only — no size/content heuristics).
type artifactGate struct{}

func (artifactGate) Evaluate(_ context.Context, spec GateSpec, subject Subject) GateResult {
	paths := subject.Artifacts
	if extra := paramPathList(spec.Params); len(extra) > 0 {
		paths = append(append([]string(nil), paths...), extra...)
	}
	if len(paths) == 0 {
		return result(spec, false, "artifact gate: no artifacts on subject")
	}
	base := subject.Workspace
	var missing []string
	for _, p := range paths {
		full := resolvePath(base, p)
		info, err := os.Stat(full)
		switch {
		case err != nil:
			missing = append(missing, displayPath(base, full))
		case info.IsDir():
			missing = append(missing, displayPath(base, full)+" (directory)")
		case info.Size() == 0:
			missing = append(missing, displayPath(base, full)+" (empty)")
		}
	}
	if len(missing) > 0 {
		return result(spec, false, "artifact gate: missing artifacts: "+strings.Join(missing, ", "))
	}
	return result(spec, true, fmt.Sprintf("artifact gate: %d artifact(s) present", len(paths)))
}

// schemaGate JSON-validates one Subject artifact against a minimal embedded
// JSON Schema subset: type, properties (+ nested), required, items,
// enum, minimum/maximum. Chosen over adding a schema dependency because the
// repo carries no validator library; unsupported keywords are ignored rather
// than failing so specs stay forward-compatible.
type schemaGate struct{}

func (schemaGate) Evaluate(_ context.Context, spec GateSpec, subject Subject) GateResult {
	target := paramString(spec.Params, "path", "")
	if target == "" && len(subject.Artifacts) > 0 {
		target = subject.Artifacts[0]
	}
	if target == "" {
		return result(spec, false, "schema gate: no path param and no subject artifacts")
	}
	schemaRaw := paramString(spec.Params, "schema", "")
	schemaFile := paramString(spec.Params, "schema_path", "")
	if schemaRaw == "" && schemaFile == "" {
		return result(spec, false, "schema gate: missing schema or schema_path param")
	}
	schemaBytes := []byte(schemaRaw)
	if schemaFile != "" {
		b, err := os.ReadFile(resolvePath(subject.Workspace, schemaFile))
		if err != nil {
			return result(spec, false, fmt.Sprintf("schema gate: cannot read schema: %v", err))
		}
		schemaBytes = b
	}
	var schema schemaNode
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return result(spec, false, fmt.Sprintf("schema gate: invalid schema JSON: %v", err))
	}
	data, err := os.ReadFile(resolvePath(subject.Workspace, target))
	if err != nil {
		return result(spec, false, fmt.Sprintf("schema gate: cannot read %s: %v", target, err))
	}
	var doc any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return result(spec, false, fmt.Sprintf("schema gate: invalid JSON in %s: %v", target, err))
	}
	var errs []string
	validateNode(&schema, doc, "$", &errs)
	if len(errs) > 0 {
		return result(spec, false, "schema gate: validation failed: "+strings.Join(errs, "; "))
	}
	return result(spec, true, fmt.Sprintf("schema gate: %s valid", target))
}

// approvalGate delegates to an injected approval manager when present and
// auto-passes otherwise — a workspace without interactive approvals treats
// the gate as satisfied rather than blocking every run.
type approvalGate struct {
	manager ApprovalRequester
}

// ApprovalRequester is the slice of tools.ExecApprovalManager this gate
// needs (interface injection keeps qualitygate free of a hard dependency on
// the concrete manager and lets tests fake decisions).
type ApprovalRequester interface {
	RequestApproval(ctx context.Context, command, agentID string, timeout time.Duration) (tools.ApprovalDecision, error)
}

func (g approvalGate) Evaluate(ctx context.Context, spec GateSpec, subject Subject) GateResult {
	if g.manager == nil {
		return result(spec, true, "approval gate auto-passed: no approval manager wired")
	}
	command := paramString(spec.Params, "command", "")
	agentID := paramString(spec.Params, "agent_id", "")
	decision, err := g.manager.RequestApproval(ctx, command, agentID, defaultTimeout)
	if err != nil {
		return result(spec, false, fmt.Sprintf("approval gate error: %v", err))
	}
	switch decision {
	case tools.ApprovalAllowOnce, tools.ApprovalAllowAlways:
		return result(spec, true, "approval gate approved by user")
	case tools.ApprovalDeny:
		return result(spec, false, "approval gate denied by user")
	default:
		return result(spec, false, fmt.Sprintf("approval gate unknown decision %q", decision))
	}
}

// Registry maps gate type → evaluator with Register/Evaluate. Zero value is
// usable but has no evaluators; use NewBuiltinRegistry for the standard set.
type Registry struct {
	evaluators map[string]Evaluator
}

// NewRegistry returns an empty registry. Evaluators are registered with
// Register; unregistered types fail closed at Evaluate time.
func NewRegistry() *Registry {
	return &Registry{evaluators: make(map[string]Evaluator)}
}

// Register wires an evaluator for gateType, replacing any previous one.
// Empty type or nil evaluator is ignored.
func (r *Registry) Register(gateType string, e Evaluator) {
	if r.evaluators == nil {
		r.evaluators = make(map[string]Evaluator)
	}
	if gateType == "" || e == nil {
		return
	}
	r.evaluators[gateType] = e
}

// Types returns the registered gate type names sorted alphabetically.
func (r *Registry) Types() []string {
	out := make([]string, 0, len(r.evaluators))
	for t := range r.evaluators {
		out = append(out, t)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Evaluate runs the evaluator registered for spec.Type. FAIL-CLOSED: an
// unknown or unregistered type yields Passed=false with detail
// "unknown gate type" — never an implicit pass.
func (r *Registry) Evaluate(ctx context.Context, spec GateSpec, subject Subject) GateResult {
	if spec.Type == "" {
		return result(spec, false, "unknown gate type")
	}
	e, ok := r.evaluators[spec.Type]
	if !ok {
		return result(spec, false, "unknown gate type")
	}
	return e.Evaluate(ctx, spec, subject)
}

// NewBuiltinRegistry registers the standard gate types:
//
//	test/build/lint — configurable shell-out commands (defaults go test/build/vet ./...),
//	    cwd-gated to Subject.Workspace, default timeout 10m (timeout_ms overrides),
//	    every command validated against the shared shell deny groups;
//	file     — existence + optional contains_regex match over given paths;
//	artifact — required output paths exist post-run and are non-empty files;
//	schema   — minimal JSON-schema validation of a subject artifact;
//	approval — delegates to the injected approval manager, auto-passes without one.
func NewBuiltinRegistry(approvals ApprovalRequester, runner CommandRunner) *Registry {
	r := NewRegistry()
	r.Register(TypeTest, commandGate{gateType: TypeTest, defaultCommand: defaultTestCommand, timeout: defaultTimeout, runner: runner})
	r.Register(TypeBuild, commandGate{gateType: TypeBuild, defaultCommand: defaultBuildCommand, timeout: defaultTimeout, runner: runner})
	r.Register(TypeLint, commandGate{gateType: TypeLint, defaultCommand: defaultLintCommand, timeout: defaultTimeout, runner: runner})
	r.Register(TypeFile, fileGate{})
	r.Register(TypeArtifact, artifactGate{})
	r.Register(TypeSchema, schemaGate{})
	r.Register(TypeApproval, approvalGate{manager: approvals})
	return r
}

// --- shared helpers ---

// denyReason validates command against the effective shell deny groups,
// honoring per-agent group overrides carried on ctx exactly like the exec
// tool (store.ShellDenyGroupsFromContext). Returns "" when allowed.
func denyReason(ctx context.Context, command string) string {
	patterns := tools.ResolveDenyPatterns(store.ShellDenyGroupsFromContext(ctx))
	for _, re := range patterns {
		if re.MatchString(command) {
			return re.String()
		}
	}
	return ""
}

// runCommand executes command in dir under timeout via runner, applying the
// deadline on the context so CommandContext kills runaway children.
func runCommand(ctx context.Context, runner CommandRunner, dir, command string, timeout time.Duration) (string, error) {
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	return runner.Run(runCtx, dir, command)
}

// resolvePath joins p onto base unless p is absolute.
func resolvePath(base, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(base, p)
}

// displayPath renders full relative to base when possible for compact details.
func displayPath(base, full string) string {
	if rel, err := filepath.Rel(base, full); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return full
}

// summarizeOutput builds a bounded detail string from captured command output.
func summarizeOutput(prefix, output string, err error) string {
	var b strings.Builder
	b.WriteString(prefix)
	if err != nil {
		fmt.Fprintf(&b, ": %v", err)
	}
	if out := strings.TrimSpace(output); out != "" {
		b.WriteString("\n")
		if len(out) > detailTailLimit {
			out = "…truncated…" + out[len(out)-detailTailLimit:]
		}
		b.WriteString(out)
	}
	return b.String()
}

// --- minimal JSON Schema subset ---

// schemaNode models the supported subset: type, properties (recursive),
// required, items, enum, minimum/maximum.
type schemaNode struct {
	Type       string                 `json:"type"`
	Properties map[string]*schemaNode `json:"properties"`
	Required   []string               `json:"required"`
	Items      *schemaNode            `json:"items"`
	Enum       []any                  `json:"enum"`
	Minimum    *float64               `json:"minimum"`
	Maximum    *float64               `json:"maximum"`
}

func validateNode(s *schemaNode, doc any, path string, errs *[]string) {
	if s == nil {
		return
	}
	if len(s.Enum) > 0 {
		match := false
		for _, allowed := range s.Enum {
			if reflect.DeepEqual(allowed, doc) {
				match = true
				break
			}
		}
		if !match {
			*errs = append(*errs, fmt.Sprintf("%s: value not in enum", path))
		}
	}
	switch s.Type {
	case "":
	case "object":
		obj, ok := doc.(map[string]any)
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s: expected object", path))
			return
		}
		for _, req := range s.Required {
			if _, present := obj[req]; !present {
				*errs = append(*errs, fmt.Sprintf("%s: missing required property %q", path, req))
			}
		}
		for name, sub := range s.Properties {
			if val, present := obj[name]; present {
				validateNode(sub, val, path+"."+name, errs)
			}
		}
	case "array":
		arr, ok := doc.([]any)
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s: expected array", path))
			return
		}
		for i, item := range arr {
			validateNode(s.Items, item, fmt.Sprintf("%s[%d]", path, i), errs)
		}
	case "string":
		if _, ok := doc.(string); !ok {
			*errs = append(*errs, fmt.Sprintf("%s: expected string", path))
		}
	case "number":
		n, ok := doc.(json.Number)
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s: expected number", path))
		} else if f, err := n.Float64(); err != nil {
			*errs = append(*errs, fmt.Sprintf("%s: invalid number", path))
		} else if s.Minimum != nil && f < *s.Minimum {
			*errs = append(*errs, fmt.Sprintf("%s: %v below minimum %v", path, f, *s.Minimum))
		} else if s.Maximum != nil && f > *s.Maximum {
			*errs = append(*errs, fmt.Sprintf("%s: %v above maximum %v", path, f, *s.Maximum))
		}
	case "integer":
		n, ok := doc.(json.Number)
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s: expected integer", path))
		} else if f, err := n.Float64(); err != nil || f != float64(int64(f)) {
			*errs = append(*errs, fmt.Sprintf("%s: expected integer", path))
		}
	case "boolean":
		if _, ok := doc.(bool); !ok {
			*errs = append(*errs, fmt.Sprintf("%s: expected boolean", path))
		}
	case "null":
		if doc != nil {
			*errs = append(*errs, fmt.Sprintf("%s: expected null", path))
		}
	default:
		*errs = append(*errs, fmt.Sprintf("%s: unsupported schema type %q", path, s.Type))
	}
}
