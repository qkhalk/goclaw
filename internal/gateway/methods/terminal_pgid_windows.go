//go:build windows

package methods

import (
	"os/exec"
)

// setProcessGroup is a no-op on Windows; terminal.create reports unavailable
// there before ever reaching this path.
func setProcessGroup(_ *exec.Cmd) {}

// signalGroup is a no-op on Windows (no POSIX process groups).
func signalGroup(_ *exec.Cmd, _ bool) {}
