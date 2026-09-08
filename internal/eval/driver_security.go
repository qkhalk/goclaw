package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// SecurityDriver evaluates the tool-layer guards that must hold before any
// command or file access executes: shell deny patterns reject dangerous
// commands WITHOUT running them (the deny scan happens pre-execution in
// ExecTool.Execute), and the restricted filesystem tool refuses paths that
// escape the workspace (traversal). Allow cases double as guards against
// over-blocking: a deny regex that eats "echo" breaks agents just as surely
// as one that misses "rm -rf /".
type SecurityDriver struct {
	workspace string
	cleanup   func()
}

func init() { RegisterDriver(&SecurityDriver{}) }

func (d *SecurityDriver) Name() string { return "security" }

func (d *SecurityDriver) Setup(env *Env, runID string) (func(), error) {
	dir, err := os.MkdirTemp("", "goclaw-eval-"+runID+"-*")
	if err != nil {
		return nil, fmt.Errorf("temp workspace: %w", err)
	}
	// The in-workspace read control needs at least one readable file.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("eval workspace file\n"), 0o644); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("seed workspace file: %w", err)
	}
	d.workspace = dir
	d.cleanup = func() { os.RemoveAll(dir) }
	return d.cleanup, nil
}

func (d *SecurityDriver) Run(ctx context.Context, env *Env, runID string, c EvalCase) (string, error) {
	// restrict=true mirrors production: the workspace is the boundary.
	switch {
	case c.Command != "":
		return d.checkCommand(ctx, c)
	case c.Path != "":
		return d.checkPath(ctx, c)
	default:
		return "", fmt.Errorf("security case requires command or path")
	}
}

// policyDenyMarker is the substring ExecTool returns when the pre-execution
// deny scan rejects a command. Deny cases must fail WITH this marker: any
// other error (execution timeout, command not found, sandbox failure) means
// the dangerous command actually RAN — a policy hole, not a pass. The first
// eval run learned this the hard way: a missing chmod pattern let
// "chmod -R 777 /" execute for the full 60s timeout and counted as "denied".
const policyDenyMarker = "denied by safety policy"

// evalExecTimeoutSeconds caps execution of allow-cases (and any command that
// slips past a missing deny pattern) so one eval case cannot hold the runner
// for the 60s default — let alone mutate the host meaningfully.
const evalExecTimeoutSeconds = 5

func (d *SecurityDriver) checkCommand(ctx context.Context, c EvalCase) (string, error) {
	tool := tools.NewExecTool(d.workspace, true)
	args := map[string]any{"command": c.Command, "timeout_seconds": evalExecTimeoutSeconds}
	res := tool.Execute(ctx, args)
	denied := res != nil && res.IsError
	detail := fmt.Sprintf("is_error=%v output=%q", denied, res.ForLLM)
	if c.ExpectDenied {
		if !denied {
			return detail, fmt.Errorf("expected policy denial, tool allowed it: %s", res.ForLLM)
		}
		if !strings.Contains(res.ForLLM, policyDenyMarker) {
			return detail, fmt.Errorf("command errored but was NOT denied by policy (policy hole or runtime failure): %s", res.ForLLM)
		}
		return detail, nil
	}
	if denied {
		return detail, fmt.Errorf("expected success, tool denied it: %s", res.ForLLM)
	}
	return detail, nil
}

func (d *SecurityDriver) checkPath(ctx context.Context, c EvalCase) (string, error) {
	tool := tools.NewReadFileTool(d.workspace, true)
	res := tool.Execute(ctx, map[string]any{"path": c.Path})
	denied := res != nil && res.IsError
	detail := fmt.Sprintf("is_error=%v output=%q", denied, res.ForLLM)
	if denied != c.ExpectDenied {
		if c.ExpectDenied {
			return detail, fmt.Errorf("expected denial, tool allowed it: %s", res.ForLLM)
		}
		return detail, fmt.Errorf("expected success, tool denied it: %s", res.ForLLM)
	}
	return detail, nil
}
