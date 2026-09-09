// Command goclaw-node is the GoClaw compute-node daemon (inheritance plan
// Phase 2). It connects to the gateway as a regular WebSocket client
// (connect frame first per protocol), registers itself with its node key
// (nodes.register), and loops on node.invoke events, executing ONLY commands
// admitted by its explicit allowlist (deny by default — nothing runs unless
// the operator listed the binary). Outcomes are posted via nodes.result.
//
// Security model (defense in depth):
//   - the gateway only routes invokes to nodes whose stored trust is
//     `trusted` (plan Rule 5: nodes are untrusted by default);
//   - this daemon independently enforces its own allowlist;
//   - commands run via argv exec (no shell), so injection requires an
//     allowlisted binary itself.
//
// Usage:
//
//	goclaw-node -gateway ws://host:18790/ws -key gnk_... -name build-1 \
//	  -token <gateway-token> -allow allowlist.txt
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

const (
	// registerInterval is how often the daemon re-sends nodes.register as a
	// liveness heartbeat. Must stay well inside the gateway's online TTL.
	registerInterval = 30 * time.Second
	// maxOutputBytes caps stdout/stderr captured per invocation.
	maxOutputBytes = 64 * 1024
	// reconnect bounds (exponential backoff).
	reconnectMin = 1 * time.Second
	reconnectMax = 60 * time.Second
	// maxConcurrentExecs bounds parallel invocations per node.
	maxConcurrentExecs = 4
)

type config struct {
	gatewayURL   string
	token        string
	nodeKey      string
	name         string
	platform     string
	capabilities []string
	allow        map[string]bool
}

func main() {
	var (
		gatewayURL = flag.String("gateway", envOr("GOCLAW_GATEWAY_URL", "ws://localhost:18790/ws"), "Gateway WebSocket URL")
		token      = flag.String("token", os.Getenv("GOCLAW_TOKEN"), "Gateway auth token (connect frame)")
		nodeKey    = flag.String("key", os.Getenv("GOCLAW_NODE_KEY"), "Node key (gnk_...) revealed at key creation")
		name       = flag.String("name", envOr("GOCLAW_NODE_NAME", ""), "Node name shown in the registry")
		caps       = flag.String("capabilities", envOr("GOCLAW_NODE_CAPS", "exec"), "Comma-separated capabilities")
		allowFile  = flag.String("allow", envOr("GOCLAW_NODE_ALLOW", ""), "Allowlist file (one binary name per line; deny by default)")
		verbose    = flag.Bool("v", false, "Verbose logging")
	)
	flag.Parse()
	if *verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	if strings.TrimSpace(*nodeKey) == "" {
		fatal("node key required: pass -key or GOCLAW_NODE_KEY")
	}
	allow, err := loadAllowlist(*allowFile)
	if err != nil {
		fatal("allowlist: %v", err)
	}
	if len(allow) == 0 {
		slog.Warn("empty allowlist: every invocation will be denied (deny by default)")
	}

	cfg := config{
		gatewayURL:   *gatewayURL,
		token:        *token,
		nodeKey:      strings.TrimSpace(*nodeKey),
		name:         strings.TrimSpace(*name),
		platform:     runtimePlatform(),
		capabilities: splitCaps(*caps),
		allow:        allow,
	}
	slog.Info("goclaw-node starting",
		"gateway", cfg.gatewayURL, "name", cfg.name,
		"platform", cfg.platform, "allowlist", len(allow))

	run(context.Background(), cfg)
}

// run drives the connect→register→serve cycle with exponential backoff.
func run(ctx context.Context, cfg config) {
	backoff := reconnectMin
	for {
		if err := session(ctx, cfg); err != nil {
			slog.Warn("session ended", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > reconnectMax {
			backoff = reconnectMax
		}
	}
}

// session performs one connect→register→serve cycle.
func session(ctx context.Context, cfg config) error {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	conn, _, err := websocket.DefaultDialer.DialContext(dialCtx, cfg.gatewayURL, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	ws := &wsConn{conn: conn}
	if err := ws.rpc(protocol.MethodConnect, map[string]string{
		"token":   cfg.token,
		"user_id": "goclaw-node",
	}); err != nil {
		return fmt.Errorf("connect rejected: %w", err)
	}
	var reg registerResponse
	if err := ws.rpcInto(&reg, protocol.MethodNodesRegister, map[string]any{
		"nodeKey":      cfg.nodeKey,
		"name":         cfg.name,
		"platform":     cfg.platform,
		"capabilities": cfg.capabilities,
	}); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	slog.Info("registered", "nodeId", reg.NodeID, "trust", reg.Trust)

	return serve(ctx, cfg, ws, conn)
}

// serve reads frames until the connection dies, dispatching node.invoke
// events to bounded executor goroutines. Liveness = periodic re-registration.
func serve(ctx context.Context, cfg config, ws *wsConn, conn *websocket.Conn) error {
	conn.SetReadLimit(512 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPingHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return ws.pong()
	})

	ticker := time.NewTicker(registerInterval)
	defer ticker.Stop()
	sem := make(chan struct{}, maxConcurrentExecs)

	for {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var header struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &header) != nil {
			continue
		}
		switch header.Type {
		case protocol.FrameTypeEvent:
			var ev protocol.EventFrame
			if json.Unmarshal(data, &ev) != nil || ev.Event != protocol.EventNodeInvoke {
				continue
			}
			payload, mErr := json.Marshal(ev.Payload)
			if mErr != nil {
				continue
			}
			select {
			case sem <- struct{}{}:
				go func() {
					defer func() { <-sem }()
					handleInvoke(ctx, cfg, ws, payload)
				}()
			default:
				slog.Warn("invoke queue full, dropping invocation")
			}
		case protocol.FrameTypeResponse:
			// Handshake responses were consumed synchronously; late ones
			// (heartbeat register replies) are logged when they fail.
			var res protocol.ResponseFrame
			if json.Unmarshal(data, &res) == nil && !res.OK && res.Error != nil {
				slog.Warn("rpc error", "id", res.ID, "code", res.Error.Code, "message", res.Error.Message)
			}
		}
	}
}

// handleInvoke executes one allowlisted command and posts nodes.result.
func handleInvoke(ctx context.Context, cfg config, ws *wsConn, payload json.RawMessage) {
	var inv invokePayload
	if err := json.Unmarshal(payload, &inv); err != nil || inv.InvokeID == "" || inv.Command == "" {
		slog.Warn("malformed node.invoke payload")
		return
	}
	if !cfg.allow[strings.TrimSpace(inv.Command)] {
		slog.Warn("security.node_exec_denied", "command", inv.Command, "invokeId", inv.InvokeID)
		ws.post(resultPayload{
			NodeID:   inv.NodeID,
			InvokeID: inv.InvokeID,
			Error:    "command not on this node's allowlist",
		})
		return
	}

	timeout := time.Duration(inv.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	if timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	// argv exec of an operator-allowlisted binary — the feature itself.
	cmd := exec.CommandContext(execCtx, inv.Command, inv.Args...) //nolint:gosec
	stdout, stderr, exitCode, execErr := runCapture(cmd, execCtx)
	ws.post(resultPayload{
		NodeID:     inv.NodeID,
		InvokeID:   inv.InvokeID,
		ExitCode:   exitCode,
		Stdout:     string(stdout),
		Stderr:     string(stderr),
		DurationMs: time.Since(start).Milliseconds(),
		Error:      errString(execErr, exitCode),
	})
	slog.Info("invoke done", "command", inv.Command, "exitCode", exitCode,
		"durationMs", time.Since(start).Milliseconds())
}

// runCapture runs cmd with bounded output capture and normalizes failures
// into (streams, exitCode, err). Context cancellation (timeout) surfaces as
// a non-nil err regardless of the raw exit status.
func runCapture(cmd *exec.Cmd, execCtx context.Context) (stdout, stderr []byte, exitCode int, err error) {
	out := newLimitedBuffer(maxOutputBytes)
	errBuf := newLimitedBuffer(maxOutputBytes)
	cmd.Stdout = out
	cmd.Stderr = errBuf
	runErr := cmd.Run()
	if ctxErr := execCtx.Err(); ctxErr != nil {
		return out.buf, errBuf.buf, 1, fmt.Errorf("timed out or cancelled: %w", ctxErr)
	}
	exitCode = cmd.ProcessState.ExitCode()
	if runErr != nil && exitCode == 0 {
		exitCode = 1
		err = runErr
	}
	return out.buf, errBuf.buf, exitCode, err
}

// --- wsConn: serialized frame writes + handshake RPCs ---

type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsConn) writeJSON(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(v)
}

func (w *wsConn) pong() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(websocket.PongMessage, nil)
}

// post is fire-and-forget nodes.result; a lost result surfaces as an invoke
// timeout gateway-side and the next heartbeat re-establishes liveness.
func (w *wsConn) post(p resultPayload) {
	req := protocol.RequestFrame{
		Type:   protocol.FrameTypeRequest,
		ID:     frameID(),
		Method: protocol.MethodNodesResult,
		Params: mustMarshal(p),
	}
	if err := w.writeJSON(req); err != nil {
		slog.Warn("post result failed", "invokeId", p.InvokeID, "error", err)
	}
}

// rpc sends one request and synchronously awaits the response with the OK
// payload dropped. Used only for connect (pre-serve).
func (w *wsConn) rpc(method string, params any) error {
	return w.rpcInto(nil, method, params)
}

// rpcInto sends one request and synchronously decodes the OK response into
// out (nil skips decoding). Only safe before serve's read loop starts.
func (w *wsConn) rpcInto(out any, method string, params any) error {
	id := frameID()
	if err := w.writeJSON(protocol.RequestFrame{
		Type: protocol.FrameTypeRequest, ID: id, Method: method,
		Params: mustMarshal(params),
	}); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		_ = w.conn.SetReadDeadline(deadline)
		var res protocol.ResponseFrame
		if err := w.conn.ReadJSON(&res); err != nil {
			return err
		}
		if res.ID != id {
			continue
		}
		_ = w.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		if !res.OK {
			if res.Error != nil {
				return fmt.Errorf("%s: %s", res.Error.Code, res.Error.Message)
			}
			return fmt.Errorf("request failed")
		}
		if out == nil {
			return nil
		}
		data, err := json.Marshal(res.Payload)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, out)
	}
}

// --- wire types / helpers ---

type invokePayload struct {
	NodeID    string   `json:"nodeId"`
	InvokeID  string   `json:"invokeId"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	TimeoutMs int64    `json:"timeoutMs"`
}

type resultPayload struct {
	NodeID     string `json:"nodeId"`
	InvokeID   string `json:"invokeId"`
	ExitCode   int    `json:"exitCode"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

type registerResponse struct {
	NodeID string `json:"nodeId"`
	Trust  string `json:"trust"`
}

// loadAllowlist reads one binary name per line (# comments allowed). An empty
// path yields an empty list — deny by default.
func loadAllowlist(path string) (map[string]bool, error) {
	allow := make(map[string]bool)
	if strings.TrimSpace(path) == "" {
		return allow, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // operator-provided flag by design
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		entry := strings.TrimSpace(line)
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		allow[entry] = true
		allow[filepath.Base(entry)] = true
	}
	return allow, nil
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func splitCaps(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(strings.ToLower(part)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// runtimePlatform renders "os/arch" like the Go target triple.
func runtimePlatform() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "goclaw-node: "+format+"\n", args...)
	os.Exit(1)
}

func frameID() string {
	return fmt.Sprintf("node-%d", time.Now().UnixNano())
}

func errString(err error, exitCode int) string {
	if err == nil {
		return ""
	}
	if exitCode != 0 {
		return "" // non-zero exit already reported via exitCode + stderr
	}
	return err.Error()
}

// limitedBuffer caps captured output (keeps the head, drops the tail).
type limitedBuffer struct {
	limit int
	buf   []byte
}

func newLimitedBuffer(limit int) *limitedBuffer { return &limitedBuffer{limit: limit} }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - len(b.buf); room > 0 {
		if len(p) > room {
			b.buf = append(b.buf, p[:room]...)
		} else {
			b.buf = append(b.buf, p...)
		}
	}
	return len(p), nil // claim full write so cmd.Run never retries/aborts
}
