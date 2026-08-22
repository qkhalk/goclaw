package qualitygate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// fakeRunner is a scripted CommandRunner recording the command and dir it
// was asked to run, returning canned output/error.
type fakeRunner struct {
	command string
	dir     string
	output  string
	err     error
	delay   time.Duration
}

func (f *fakeRunner) Run(ctx context.Context, dir, command string) (string, error) {
	f.command = command
	f.dir = dir
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return f.output, f.err
}
func testSubject(t *testing.T) Subject {
	t.Helper()
	return Subject{Workspace: t.TempDir()}
}

// --- spec parsing ---

func TestParseGateSpec(t *testing.T) {
	cases := []struct {
		entry    string
		name     string
		gateType string
		wantErr  bool
	}{
		{"test", "test", "test", false},
		{"tests:test(timeout_ms=5000)", "tests", "test", false},
		{"docs:file(path=README.md)", "docs", "file", false},
		{"gate:build(command=make build,timeout_ms=60)", "gate", "build", false},
		{"flagged(approval(strict))", "", "", true}, // nested head unsupported
		{"", "", "", true},
		{"(no type)", "", "", true},
		{"name:(missing type)", "", "", true},
		{"unclosed(param=1", "", "", true},
	}
	for _, tc := range cases {
		spec, err := ParseGateSpec(tc.entry)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseGateSpec(%q): expected error, got %+v", tc.entry, spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseGateSpec(%q): unexpected error %v", tc.entry, err)
			continue
		}
		if spec.Name != tc.name || spec.Type != tc.gateType {
			t.Errorf("ParseGateSpec(%q) = {%s %s}, want {%s %s}", tc.entry, spec.Name, spec.Type, tc.name, tc.gateType)
		}
	}
}

func TestParseGateSpecParamTypes(t *testing.T) {
	spec, err := ParseGateSpec("g:test(timeout_ms=2500, strict=true, label=smoke)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	switch v := spec.Params["timeout_ms"].(type) {
	case float64:
		if v != 2500 {
			t.Errorf("timeout_ms = %v, want 2500", v)
		}
	default:
		t.Errorf("timeout_ms should parse as number, got %T", spec.Params["timeout_ms"])
	}
	if spec.Params["strict"] != true {
		t.Errorf("strict = %v, want true", spec.Params["strict"])
	}
	if spec.Params["label"] != "smoke" {
		t.Errorf("label = %v, want smoke", spec.Params["label"])
	}
}

// --- fail-closed registry ---

func TestRegistry_UnknownTypeFailsClosed(t *testing.T) {
	r := NewBuiltinRegistry(nil, &fakeRunner{})
	res := r.Evaluate(context.Background(), GateSpec{Name: "mystery", Type: "quantum-verify"}, testSubject(t))
	if res.Passed {
		t.Fatal("unknown gate type must not pass (fail-closed)")
	}
	if res.Detail != "unknown gate type" {
		t.Errorf("Detail = %q, want %q", res.Detail, "unknown gate type")
	}
}

func TestRegistry_EmptyTypeFailsClosed(t *testing.T) {
	r := NewRegistry()
	res := r.Evaluate(context.Background(), GateSpec{Name: "x"}, testSubject(t))
	if res.Passed || res.Detail != "unknown gate type" {
		t.Errorf("empty type: got %+v, want failed with unknown gate type", res)
	}
}

func TestRegistry_Types(t *testing.T) {
	r := NewBuiltinRegistry(nil, nil)
	want := []string{TypeApproval, TypeArtifact, TypeBuild, TypeFile, TypeLint, TypeSchema, TypeTest}
	got := r.Types()
	if len(got) != len(want) {
		t.Fatalf("Types = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Types = %v, want %v", got, want)
		}
	}
}

// --- test/build/lint shell-out gates ---

func TestCommandGate_PassesOnExitZero(t *testing.T) {
	runner := &fakeRunner{output: "ok"}
	r := NewBuiltinRegistry(nil, runner)
	res := r.Evaluate(context.Background(),
		GateSpec{Name: "test", Type: TypeTest},
		Subject{Workspace: "/ws/demo"})
	if !res.Passed {
		t.Fatalf("expected pass, got %+v", res)
	}
	if runner.command != defaultTestCommand {
		t.Errorf("command = %q, want default %q", runner.command, defaultTestCommand)
	}
	if runner.dir != "/ws/demo" {
		t.Errorf("dir = %q, want workspace cwd-gating", runner.dir)
	}
}

func TestCommandGate_FailsOnNonZeroExit(t *testing.T) {
	runner := &fakeRunner{output: "FAIL: 2 tests broke", err: errors.New("exit status 1")}
	r := NewBuiltinRegistry(nil, runner)
	res := r.Evaluate(context.Background(), GateSpec{Name: "test", Type: TypeTest}, testSubject(t))
	if res.Passed {
		t.Fatal("expected failure on non-zero exit")
	}
	if !strings.Contains(res.Detail, "FAIL: 2 tests broke") {
		t.Errorf("Detail should carry captured output, got %q", res.Detail)
	}
}

func TestCommandGate_ConfigurableCommands(t *testing.T) {
	cases := []struct {
		gateType       string
		defaultCommand string
	}{
		{TypeTest, defaultTestCommand},
		{TypeBuild, defaultBuildCommand},
		{TypeLint, defaultLintCommand},
	}
	for _, tc := range cases {
		runner := &fakeRunner{}
		r := NewBuiltinRegistry(nil, runner)
		res := r.Evaluate(context.Background(),
			GateSpec{Name: tc.gateType, Type: tc.gateType, Params: map[string]any{"command": "make check"}},
			testSubject(t))
		if !res.Passed {
			t.Errorf("%s gate: expected pass, got %+v", tc.gateType, res)
		}
		if runner.command != "make check" {
			t.Errorf("%s gate: command = %q, want configured value", tc.gateType, runner.command)
		}
		// Default sanity: builtin default is the go toolchain command.
		if tc.defaultCommand == "" {
			t.Errorf("%s gate: missing default command constant", tc.gateType)
		}
	}
}

func TestCommandGate_TimeoutKillsRunawayCommand(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("sleep not available")
	}
	r := NewBuiltinRegistry(nil, nil) // real exec runner
	start := time.Now()
	res := r.Evaluate(context.Background(),
		GateSpec{
			Name:   "test",
			Type:   TypeTest,
			Params: map[string]any{"command": "sleep 30", "timeout_ms": float64(300)},
		},
		testSubject(t))
	elapsed := time.Since(start)
	if res.Passed {
		t.Fatal("timed-out gate must fail")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("timeout not applied: ran %v, want ~300ms", elapsed)
	}
	if !strings.Contains(res.Detail, "deadline") && !strings.Contains(res.Detail, "signal") && !strings.Contains(res.Detail, "context") {
		t.Errorf("Detail should indicate timeout, got %q", res.Detail)
	}
}

func TestCommandGate_DenyPatternBlocksBeforeExec(t *testing.T) {
	runner := &fakeRunner{output: "should never run"}
	r := NewBuiltinRegistry(nil, runner)
	res := r.Evaluate(context.Background(),
		GateSpec{
			Name:   "test",
			Type:   TypeTest,
			Params: map[string]any{"command": "go test ./...; rm -rf /"},
		},
		testSubject(t))
	if res.Passed {
		t.Fatal("deny-pattern match must fail the gate")
	}
	if !strings.Contains(res.Detail, "denied by shell policy") {
		t.Errorf("Detail = %q, want deny-policy reason", res.Detail)
	}
	if runner.command != "" {
		t.Errorf("denied command must never execute, but runner saw %q", runner.command)
	}
}

// --- file gate ---

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFileGate_ExistsAndRegexMatch(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "README.md", "# Demo\n\nstatus: ready\n")

	r := NewBuiltinRegistry(nil, nil)
	subject := Subject{Workspace: ws}
	if res := r.Evaluate(context.Background(),
		GateSpec{Name: "docs", Type: TypeFile, Params: map[string]any{"path": "README.md"}}, subject); !res.Passed {
		t.Errorf("existing path should pass, got %+v", res)
	}
	if res := r.Evaluate(context.Background(),
		GateSpec{Name: "docs", Type: TypeFile, Params: map[string]any{"path": "README.md", "contains_regex": `status:\s*ready`}}, subject); !res.Passed {
		t.Errorf("regex hit should pass, got %+v", res)
	}
	if res := r.Evaluate(context.Background(),
		GateSpec{Name: "docs", Type: TypeFile, Params: map[string]any{"path": "README.md", "contains_regex": `status:\s*broken`}}, subject); res.Passed {
		t.Error("regex miss should fail")
	}
	if res := r.Evaluate(context.Background(),
		GateSpec{Name: "docs", Type: TypeFile, Params: map[string]any{"path": "missing.txt"}}, subject); res.Passed {
		t.Error("missing path should fail")
	}
}

func TestFileGate_MultiplePathsAllMustPass(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "a.txt", "alpha")
	r := NewBuiltinRegistry(nil, nil)
	res := r.Evaluate(context.Background(),
		GateSpec{Name: "files", Type: TypeFile, Params: map[string]any{"paths": "a.txt,b.txt"}},
		Subject{Workspace: ws})
	if res.Passed {
		t.Error("one missing path of several must fail the whole gate")
	}
	if !strings.Contains(res.Detail, "b.txt") {
		t.Errorf("Detail should name the missing path, got %q", res.Detail)
	}
}

func TestFileGate_FallsBackToSubjectArtifacts(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "out.json", "{}")
	r := NewBuiltinRegistry(nil, nil)
	res := r.Evaluate(context.Background(),
		GateSpec{Name: "outputs", Type: TypeFile},
		Subject{Workspace: ws, Artifacts: []string{"out.json"}})
	if !res.Passed {
		t.Errorf("subject artifact fallback should pass, got %+v", res)
	}
}

// --- artifact gate ---

func TestArtifactGate_RequiresExistingNonEmptyFiles(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "report.md", "# Report\n")
	writeFile(t, ws, "empty.bin", "")

	r := NewBuiltinRegistry(nil, nil)
	pass := r.Evaluate(context.Background(), GateSpec{Name: "artifacts", Type: TypeArtifact},
		Subject{Workspace: ws, Artifacts: []string{"report.md"}})
	if !pass.Passed {
		t.Errorf("present artifact should pass, got %+v", pass)
	}

	fail := r.Evaluate(context.Background(), GateSpec{Name: "artifacts", Type: TypeArtifact},
		Subject{Workspace: ws, Artifacts: []string{"gone.md"}})
	if fail.Passed || !strings.Contains(fail.Detail, "gone.md") {
		t.Errorf("missing artifact should fail naming it, got %+v", fail)
	}

	empty := r.Evaluate(context.Background(), GateSpec{Name: "artifacts", Type: TypeArtifact},
		Subject{Workspace: ws, Artifacts: []string{"empty.bin"}})
	if empty.Passed || !strings.Contains(empty.Detail, "empty") {
		t.Errorf("zero-byte artifact should fail as empty, got %+v", empty)
	}

	none := r.Evaluate(context.Background(), GateSpec{Name: "artifacts", Type: TypeArtifact}, Subject{Workspace: ws})
	if none.Passed {
		t.Error("no artifacts declared should fail")
	}
}

// --- schema gate ---

func TestSchemaGate_ValidatesJSONDocument(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "result.json", `{"name":"demo","count":3,"tags":["a","b"]}`)

	schemaParams := func(schema string) map[string]any { return map[string]any{"path": "result.json", "schema": schema} }
	objectSchema := `{"type":"object","required":["name","count"],"properties":{"name":{"type":"string"},"count":{"type":"integer"},"tags":{"type":"array","items":{"type":"string"}}}}`

	r := NewBuiltinRegistry(nil, nil)
	subject := Subject{Workspace: ws}
	if res := r.Evaluate(context.Background(), GateSpec{Name: "shape", Type: TypeSchema, Params: schemaParams(objectSchema)}, subject); !res.Passed {
		t.Errorf("valid doc should pass, got %+v", res)
	}

	missingRequired := `{"type":"object","required":["summary"],"properties":{"summary":{"type":"string"}}}`
	if res := r.Evaluate(context.Background(), GateSpec{Name: "shape", Type: TypeSchema, Params: schemaParams(missingRequired)}, subject); res.Passed {
		t.Error("missing required property must fail")
	}

	wrongType := `{"type":"object","properties":{"count":{"type":"boolean"}}}`
	if res := r.Evaluate(context.Background(), GateSpec{Name: "shape", Type: TypeSchema, Params: schemaParams(wrongType)}, subject); res.Passed {
		t.Error("wrong-typed property must fail")
	}

	badDoc := `{"name":`
	writeFile(t, ws, "broken.json", badDoc)
	if res := r.Evaluate(context.Background(),
		GateSpec{Name: "shape", Type: TypeSchema, Params: map[string]any{"path": "broken.json", "schema": objectSchema}}, subject); res.Passed {
		t.Error("malformed JSON document must fail")
	}
}

// --- approval gate ---

type fakeApprovals struct {
	decision tools.ApprovalDecision
	err      error
	command  string
	agentID  string
}

func (f *fakeApprovals) RequestApproval(_ context.Context, command, agentID string, _ time.Duration) (tools.ApprovalDecision, error) {
	f.command = command
	f.agentID = agentID
	return f.decision, f.err
}

func TestApprovalGate_AutoPassesWithoutManager(t *testing.T) {
	r := NewBuiltinRegistry(nil, nil)
	res := r.Evaluate(context.Background(), GateSpec{Name: "signoff", Type: TypeApproval}, testSubject(t))
	if !res.Passed {
		t.Fatalf("approval gate without manager must auto-pass, got %+v", res)
	}
	if !strings.Contains(res.Detail, "auto-passed") {
		t.Errorf("Detail = %q, want auto-pass explanation", res.Detail)
	}
}

func TestApprovalGate_DelegatesToManager(t *testing.T) {
	cases := []struct {
		decision tools.ApprovalDecision
		wantPass bool
	}{
		{tools.ApprovalAllowOnce, true},
		{tools.ApprovalAllowAlways, true},
		{tools.ApprovalDeny, false},
	}
	for _, tc := range cases {
		mgr := &fakeApprovals{decision: tc.decision}
		r := NewBuiltinRegistry(mgr, nil)
		res := r.Evaluate(context.Background(),
			GateSpec{Name: "deploy-signoff", Type: TypeApproval, Params: map[string]any{"command": "make deploy", "agent_id": "ag-1"}},
			testSubject(t))
		if res.Passed != tc.wantPass {
			t.Errorf("decision %q: passed = %v, want %v (%s)", tc.decision, res.Passed, tc.wantPass, res.Detail)
		}
		if mgr.command != "make deploy" || mgr.agentID != "ag-1" {
			t.Errorf("manager received command=%q agent=%q, want make deploy/ag-1", mgr.command, mgr.agentID)
		}
	}

	mgr := &fakeApprovals{err: errors.New("channel down")}
	r := NewBuiltinRegistry(mgr, nil)
	if res := r.Evaluate(context.Background(), GateSpec{Name: "s", Type: TypeApproval}, testSubject(t)); res.Passed {
		t.Error("approval transport error must fail the gate")
	}
}
