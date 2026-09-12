// Package storage runs rclone rcd as a supervised loopback process and
// exposes list/stat/read/fetch operations over its rc HTTP API.
//
// Security model (rclone docs: rc access "is equivalent to shell access as
// the user running rclone"):
//   - rcd listens on 127.0.0.1 with a random port + random basic-auth
//     credentials generated per supervisor start;
//   - the rc endpoint is NEVER proxied through the gateway HTTP surface;
//   - core/command and config/dump are never called from this code.
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Supervisor owns the rclone rcd child process and its rc client.
type Supervisor struct {
	mu        sync.Mutex
	binPath   string
	configDir string // holds rclone.conf (runtime token truth for rclone)

	cmd      *exec.Cmd
	rc       *RCClient
	user     string
	pass     string
	port     int
	started  time.Time
	shutdown bool
}

// NewSupervisor creates the supervisor; Start is lazy (first use).
func NewSupervisor(binPath, configDir string) *Supervisor {
	if binPath == "" {
		binPath = "rclone"
	}
	return &Supervisor{binPath: binPath, configDir: configDir}
}

// RC returns the rc client, starting rcd if needed.
func (s *Supervisor) RC(ctx context.Context) (*RCClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rc != nil {
		// Liveness: cheap version probe with a short timeout.
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := s.rc.CoreVersion(pctx)
		cancel()
		if err == nil {
			return s.rc, nil
		}
		slog.Warn("cloud storage: rcd unreachable, restarting", "error", err)
		s.stopLocked()
	}
	return s.startLocked(ctx)
}

// startLocked launches `rclone rcd` on a random loopback port with random
// basic-auth credentials. Caller must hold s.mu.
func (s *Supervisor) startLocked(ctx context.Context) (*RCClient, error) {
	if s.shutdown {
		return nil, errors.New("cloud storage: supervisor already shut down")
	}
	if err := os.MkdirAll(s.configDir, 0o700); err != nil {
		return nil, fmt.Errorf("cloud storage: config dir: %w", err)
	}
	// rclone refuses a config file it cannot write (token refresh) — create it.
	confPath := filepath.Join(s.configDir, "rclone.conf")
	f, err := os.OpenFile(confPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cloud storage: rclone.conf: %w", err)
	}
	f.Close()

	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}
	user, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	pass, err := randomToken(24)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(s.binPath, "rcd",
		"--rc-addr", fmt.Sprintf("127.0.0.1:%d", port),
		"--rc-user", user,
		"--rc-pass", pass,
		"--config", confPath,
	)
	// Detach from the gateway's stdio; rcd logs to stderr — capture for debugging.
	cmd.Stderr = nil
	cmd.Stdout = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cloud storage: start rcd: %w (is rclone installed? see docs/30-cloud-accounts.md)", err)
	}

	rc := NewRCClient(fmt.Sprintf("http://127.0.0.1:%d", port), user, pass)
	// Wait for the rc endpoint (rcd needs a moment to bind).
	deadline := time.Now().Add(10 * time.Second)
	for {
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, pingErr := rc.CoreVersion(pctx)
		cancel()
		if pingErr == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, fmt.Errorf("cloud storage: rcd did not become ready: %w", pingErr)
		}
		time.Sleep(200 * time.Millisecond)
	}

	s.cmd = cmd
	s.rc = rc
	s.user, s.pass, s.port = user, pass, port
	s.started = time.Now()
	slog.Info("cloud storage: rcd started", "addr", fmt.Sprintf("127.0.0.1:%d", port))
	return rc, nil
}

// stopLocked kills the child process. Caller must hold s.mu.
func (s *Supervisor) stopLocked() {
	if s.cmd == nil {
		s.rc = nil
		return
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	s.cmd = nil
	s.rc = nil
	slog.Info("cloud storage: rcd stopped")
}

// Shutdown stops the supervisor (gateway close).
func (s *Supervisor) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shutdown = true
	s.stopLocked()
}

// RemoteName is the rclone remote for an account. The full account UUID is
// used (no truncation): the rclone.conf is shared by all tenants, and a
// truncated-name collision would make account B resolve to account A's Drive.
func RemoteName(accountID string) string {
	return "goclaw-" + strings.ToLower(accountID)
}

func freeLoopbackPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("cloud storage: pick port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
