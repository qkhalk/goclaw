package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAllowlistEmptyPathDeniesAll(t *testing.T) {
	allow, err := loadAllowlist("")
	if err != nil {
		t.Fatalf("loadAllowlist(\"\"): %v", err)
	}
	if len(allow) != 0 {
		t.Fatalf("empty path must yield an empty (deny-all) allowlist, got %v", allow)
	}
}

func TestLoadAllowlistParsing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.txt")
	content := "# comment line\n\n git \nnode\n/usr/bin/curl\n# another\nnpm\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write allowlist: %v", err)
	}
	allow, err := loadAllowlist(path)
	if err != nil {
		t.Fatalf("loadAllowlist: %v", err)
	}
	// Both the bare name and the full path admit the same binary.
	for _, entry := range []string{"git", "node", "curl", "/usr/bin/curl", "npm"} {
		if !allow[entry] {
			t.Errorf("allowlist missing %q", entry)
		}
	}
	if allow["docker"] {
		t.Error("allowlist must not contain unlisted binaries")
	}
	if allow["rm"] {
		t.Error("allowlist must not contain unlisted binaries")
	}
}

func TestLoadAllowlistMissingFile(t *testing.T) {
	if _, err := loadAllowlist(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("missing allowlist file should error (operator typo), not deny silently")
	}
}

func TestSplitCaps(t *testing.T) {
	got := splitCaps(" exec , FS,browser,,")
	if len(got) != 3 || got[0] != "exec" || got[1] != "fs" || got[2] != "browser" {
		t.Fatalf("splitCaps = %v", got)
	}
	if len(splitCaps("")) != 0 {
		t.Fatal("empty caps string should yield no capabilities")
	}
}

func TestLimitedBufferCapsOutput(t *testing.T) {
	b := newLimitedBuffer(8)
	n, err := b.Write([]byte("0123456789"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 10 {
		t.Fatalf("Write must report the full length to io.Copy, got %d", n)
	}
	if string(b.buf) != "01234567" {
		t.Fatalf("buffer = %q, want head-capped %q", b.buf, "01234567")
	}
}

func TestErrString(t *testing.T) {
	if errString(nil, 0) != "" {
		t.Fatal("nil error should render empty")
	}
	// Non-zero exit swallows the message (exit code carries the signal).
	if errString(os.ErrNotExist, 1) != "" {
		t.Fatal("non-zero exit should render empty error text")
	}
	if errString(os.ErrNotExist, 0) == "" {
		t.Fatal("zero-exit error should surface its message")
	}
}

func TestBackoffBounds(t *testing.T) {
	// Mirror the constants: reconnect growth must stay bounded.
	backoff := reconnectMin
	for i := 0; i < 20; i++ {
		backoff *= 2
		if backoff > reconnectMax {
			backoff = reconnectMax
		}
	}
	if backoff != reconnectMax {
		t.Fatalf("backoff = %s, want capped at %s", backoff, reconnectMax)
	}
	if reconnectMin > registerInterval {
		t.Fatal("first reconnect should not wait longer than the heartbeat interval")
	}
	if _, err := time.ParseDuration(registerInterval.String()); err != nil {
		t.Fatalf("registerInterval: %v", err)
	}
}
