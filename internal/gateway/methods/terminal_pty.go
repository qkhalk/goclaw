package methods

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/creack/pty"
)

// Terminal PTY manager (Paseo plan Phase 4 / §25). One in-memory session per
// terminal tab; raw output is streamed to clients over WS events and kept
// only in a bounded ring buffer (last ringCapacity bytes) for replay on
// reconnect. Nothing here touches disk.
const (
	// ringCapacity bounds the replay buffer per terminal (64 KiB).
	ringCapacity = 64 << 10
	// readerChunkSize is the read granularity of the PTY pump goroutine;
	// small enough that chunks reach the client promptly, large enough to
	// avoid syscall storms under bulk output.
	readerChunkSize = 4 << 10

	defaultTermCols = 80
	defaultTermRows = 24
	minTermCols     = 20
	maxTermCols     = 500
	minTermRows     = 5
	maxTermRows     = 200

	// maxSessionsPerUser caps concurrent running PTYs per user so a runaway
	// UI cannot exhaust host processes/file descriptors.
	maxSessionsPerUser = 8
)

// ptySession is one live terminal. mu guards the master fd and window size;
// closed flips exactly once when the shell exits or the session is closed.
type ptySession struct {
	id          string
	workspaceID string
	userID      string

	ptmx *os.File
	cmd  *exec.Cmd

	ring []byte // bounded at ringCapacity, guarded by mu

	cols uint16
	rows uint16

	closed atomic.Bool
	mu     sync.Mutex
}

// ptyManager owns every live PTY. Sessions remove themselves from the map
// after broadcasting exit; DB rows persist as 'exited' metadata.
type ptyManager struct {
	mu       sync.RWMutex
	sessions map[string]*ptySession
}

func newPtyManager() *ptyManager {
	return &ptyManager{sessions: make(map[string]*ptySession)}
}

func (m *ptyManager) get(id string) (*ptySession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessions[id]
	return sess, ok
}

// countRunning returns how many live PTYs the user currently owns.
func (m *ptyManager) countRunning(userID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, sess := range m.sessions {
		if sess.userID == userID && !sess.closed.Load() {
			n++
		}
	}
	return n
}

func (m *ptyManager) put(sess *ptySession) {
	m.mu.Lock()
	m.sessions[sess.id] = sess
	m.mu.Unlock()
}

func (m *ptyManager) remove(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// start spawns a shell in a new PTY rooted inside the workspace and begins
// pumping its output through broadcast as "terminal.output" events. On shell
// exit it emits "terminal.exit" and reaps the session.
func (m *ptyManager) start(id, userID, workspaceID, root, cwd, shell string, cols, rows int, broadcast func(event string, payload map[string]any)) (*ptySession, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("terminal requires a unix host")
	}
	if cols <= 0 {
		cols = defaultTermCols
	}
	if rows <= 0 {
		rows = defaultTermRows
	}
	cols = clampInt(cols, minTermCols, maxTermCols)
	rows = clampInt(rows, minTermRows, maxTermRows)

	resolved, err := resolveShell(shell)
	if err != nil {
		return nil, fmt.Errorf("resolve shell: %w", err)
	}

	dir := root
	if cwd != "" {
		dir = filepath.Join(root, cwd)
	}

	cmd := exec.Command(resolved)
	cmd.Dir = dir
	env := os.Environ()
	env = append(env, "TERM=xterm-256color")
	cmd.Env = env
	setProcessGroup(cmd)

	sess := &ptySession{
		id:          id,
		workspaceID: workspaceID,
		userID:      userID,
		cmd:         cmd,
		ring:        make([]byte, 0, ringCapacity),
		cols:        uint16(cols),
		rows:        uint16(rows),
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}
	sess.ptmx = ptmx

	m.put(sess)
	go m.pump(sess, broadcast)
	return sess, nil
}

// pump reads the PTY master until EOF/error, appending each chunk to the
// replay ring and streaming it base64-encoded to subscribers. After the
// final read it stamps the exit code (when extractable), broadcasts exit,
// and removes itself from the manager; the store row stays behind as
// 'exited' metadata.
func (m *ptyManager) pump(sess *ptySession, broadcast func(event string, payload map[string]any)) {
	defer m.remove(sess.id)
	buf := make([]byte, readerChunkSize)
	for {
		n, err := sess.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			sess.appendRing(chunk)
			broadcast("terminal.output", map[string]any{
				"terminalId": sess.id,
				"userId":     sess.userID,
				"data":       base64.StdEncoding.EncodeToString(chunk),
			})
		}
		if err != nil {
			break
		}
	}
	exitCode := -1
	if sess.cmd.ProcessState != nil {
		exitCode = sess.cmd.ProcessState.ExitCode()
	}
	sess.closed.Store(true)
	_ = sess.ptmx.Close()
	broadcast("terminal.exit", map[string]any{
		"terminalId": sess.id,
		"userId":     sess.userID,
		"exitCode":   exitCode,
	})
}

// appendRing appends chunk to the fixed-capacity ring, dropping the oldest
// bytes once full. Chunks are byte-exact replays; partial escape sequences
// across chunk boundaries are tolerated by xterm.js on decode.
func (s *ptySession) appendRing(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(chunk) >= ringCapacity {
		// Keep only the tail of an oversized chunk.
		s.ring = append(s.ring[:0], chunk[len(chunk)-ringCapacity:]...)
		return
	}
	free := ringCapacity - len(s.ring)
	if len(chunk) <= free {
		s.ring = append(s.ring, chunk...)
		return
	}
	drop := len(chunk) - free
	if drop >= len(s.ring) {
		s.ring = s.ring[:0]
	} else {
		s.ring = s.ring[drop:]
	}
	s.ring = append(s.ring, chunk...)
}

// snapshot returns the ring contents in order plus the current size.
func (s *ptySession) snapshot() (data []byte, cols, rows uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, len(s.ring))
	copy(out, s.ring)
	return out, s.cols, s.rows
}

// write sends user keystrokes to the shell.
func (s *ptySession) write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil || s.closed.Load() {
		return fmt.Errorf("terminal closed")
	}
	_, err := s.ptmx.Write(data)
	return err
}

// resize resizes the PTY window.
func (s *ptySession) resize(cols, rows uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil || s.closed.Load() {
		return fmt.Errorf("terminal closed")
	}
	s.cols, s.rows = cols, rows
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// close kills the whole process group (shell + children). Safe to call
// twice; the pump goroutine still owns exit broadcasting for a natural
// death, while an explicit close marks the session dead immediately.
func (s *ptySession) close() {
	if !s.closed.Swap(true) {
		signalGroup(s.cmd, true)
	}
	if s.ptmx != nil {
		_ = s.ptmx.Close()
	}
}

// resolveShell picks the shell binary: explicit param if its base name
// resolves in PATH, else $SHELL, else /bin/bash, then sh as last resort.
func resolveShell(requested string) (string, error) {
	if requested != "" {
		if path, err := exec.LookPath(requested); err == nil {
			return path, nil
		}
	}
	if envShell := os.Getenv("SHELL"); envShell != "" {
		if path, err := exec.LookPath(envShell); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath("/bin/bash"); err == nil {
		return path, nil
	}
	return exec.LookPath("sh")
}

// clampInt constrains v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
