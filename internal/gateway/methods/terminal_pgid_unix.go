//go:build !windows

package methods

import (
	"os/exec"
	"syscall"
)

// setProcessGroup makes the child its own process-group leader so
// signalGroup can reach the whole tree (shell + forked children) with one
// kill(2). Mirrors internal/cronexec and internal/tools.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup sends SIGKILL to the process group rooted at cmd.Process.Pid.
// Because Setpgid was set, pgid == pid, so kill(-pid) reaches every forked
// child.
func signalGroup(cmd *exec.Cmd, _ bool) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
