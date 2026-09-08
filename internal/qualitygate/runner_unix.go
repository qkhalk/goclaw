//go:build !windows

package qualitygate

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
)

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
