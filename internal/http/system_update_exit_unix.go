//go:build !windows

package http

import "syscall"

// defaultExit terminates the process via SIGTERM so the gateway's graceful
// shutdown path (lifecycle drain, provider close) runs before exit; systemd
// then restarts the unit into the replaced binary.
func defaultExit(int) {
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}
