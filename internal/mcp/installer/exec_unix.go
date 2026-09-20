//go:build !windows

package installer

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group so a step timeout
// kills the WHOLE tree (npm/pip/git spawn grandchildren) instead of only the
// direct child — orphaned install processes on a 512MB host are how OOMs
// happen.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID signals the whole process group.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
}
