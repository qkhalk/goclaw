package cloud

import (
	"errors"
	"fmt"
	"strings"
)

// Remote paths are always forward-slash separated (rclone spec syntax),
// regardless of the gateway's host OS — never build them with filepath.Join.

// CleanRemotePath validates and normalizes one remote path (a file OR a
// directory). Rejects empty input, "."/".."/empty segments (traversal), and
// backslash separators (normalized to slashes first so Windows clients get a
// chance). Returns the path without leading/trailing slashes: "/a/b" → "a/b".
// The HTTP layer logs slog.Warn("security.*") on error; this helper stays
// silent so the storage service can call it as defense in depth.
func CleanRemotePath(p string) (string, error) {
	p = strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" {
		return "", errors.New("path is required")
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("invalid path %q", p)
		}
	}
	return p, nil
}

// CleanRemoteDir is CleanRemotePath but accepts the drive root: "" and "/"
// both normalize to "" (no error). Upload target folders use this.
func CleanRemoteDir(p string) (string, error) {
	p = strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" {
		return "", nil
	}
	return CleanRemotePath(p)
}

// RemoteJoin appends name to dir with remote (slash) separators.
// dir "" (root) yields just name.
func RemoteJoin(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}
