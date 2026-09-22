//go:build windows

package installer

import "os/exec"

// setProcessGroup is a no-op on Windows: installs target the Linux gateway
// host; dev machines don't need process-group kills for correctness.
func setProcessGroup(*exec.Cmd) {}
