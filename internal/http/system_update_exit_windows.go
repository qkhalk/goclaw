//go:build windows

package http

import "os"

// defaultExit on Windows: self-update installs are rejected by the platform
// gate long before this runs (linux/amd64 only), so plain exit is fine.
func defaultExit(code int) {
	os.Exit(code)
}
