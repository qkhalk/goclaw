//go:build windows

package qualitygate

import (
	"bytes"
	"context"
	"os/exec"
)

// execCommandRunner shells out through os/exec with the context deadline
// already applied by runCommand. Windows has no process groups reachable via
// syscall.Kill, so timeout cancellation falls back to killing the shell
// process itself (exec.Cmd's default Cancel). The `sh -c` invocation still
// requires a POSIX shell on PATH — quality-gate agents on Windows need Git
// Bash or WSL; a missing shell surfaces as the gate's captured error output.
type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, dir, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
