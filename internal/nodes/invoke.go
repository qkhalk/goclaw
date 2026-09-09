package nodes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// Typed invoke failures. Callers (agent tool, RPC handlers) surface these
// directly instead of string-matching; "offline" and "timeout" are distinct
// and neither ever hangs — every path returns through a bounded wait.
var (
	ErrNodeNotFound    = errors.New("node not found")
	ErrNodeUntrusted   = errors.New("node is not trusted")
	ErrNodeOffline     = errors.New("node is offline")
	ErrNodeNoExecCap   = errors.New("node does not advertise the exec capability")
	ErrInvokeTimeout   = errors.New("node invoke timed out")
	ErrInvokeRejected  = errors.New("node rejected the invocation")
	ErrInvokeMalformed = errors.New("node returned a malformed result")
)

// DefaultInvokeTimeout bounds a node invocation when the caller passes none.
const DefaultInvokeTimeout = 120 * time.Second

// MaxInvokeTimeout caps caller-supplied timeouts.
const MaxInvokeTimeout = 10 * time.Minute

// MaxResultBytes caps stdout/stderr carried by a single invoke result.
const MaxResultBytes = 64 * 1024

// NodeLookup is the store surface Invoke needs (satisfied by store.NodeStore).
type NodeLookup interface {
	GetByID(ctx context.Context, id string) (*store.Node, error)
}

// InvokeRequest is one remote-exec request routed to a node daemon.
type InvokeRequest struct {
	Command   string        // binary to execute (argv[0], no shell)
	Args      []string      // argv[1:]
	Timeout   time.Duration // per-invocation bound (default DefaultInvokeTimeout)
	StartedAt time.Time     // optional: set by the gateway for bookkeeping
}

// InvokeResult is the daemon-posted outcome of one invocation.
type InvokeResult struct {
	InvokeID   string `json:"invokeId"`
	NodeID     string `json:"nodeId,omitempty"`
	ExitCode   int    `json:"exitCode,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Error      string `json:"error,omitempty"` // daemon-side failure (allowlist deny, spawn error)
	DurationMs int64  `json:"durationMs,omitempty"`
}

// OK reports whether the invocation completed without a daemon-side error.
func (r *InvokeResult) OK() bool {
	return r != nil && r.Error == "" && r.ExitCode == 0
}

// Invoke routes a command to the named node's live connection and waits for
// the correlated nodes.result. Gates in order: node exists → tenant matches
// the registration → trust == trusted → exec capability advertised → live
// connection within the online TTL. Any gate failure returns a typed error
// immediately; a registered-but-silent daemon returns ErrInvokeTimeout.
func Invoke(ctx context.Context, reg *Registry, lookup NodeLookup, nodeID string, req InvokeRequest) (*InvokeResult, error) {
	if reg == nil {
		return nil, ErrNodeOffline
	}
	if lookup == nil {
		return nil, ErrNodeNotFound
	}
	node, err := lookup.GetByID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("node lookup: %w", err)
	}
	if node == nil {
		return nil, ErrNodeNotFound
	}
	// Tenant gate: a node row belongs to its creating tenant (nil = master).
	// Cross-tenant probes read as "not found", never as "untrusted", so the
	// error cannot be used to enumerate another tenant's nodes.
	nodeTenant := store.MasterTenantID
	if node.TenantID != nil && *node.TenantID != "" {
		if parsed, parseErr := uuid.Parse(*node.TenantID); parseErr == nil {
			nodeTenant = parsed
		}
	}
	if tid := store.TenantIDFromContext(ctx); tid != uuid.Nil && tid != nodeTenant {
		return nil, ErrNodeNotFound
	}
	if node.Trust != store.NodeTrustTrusted {
		return nil, ErrNodeUntrusted
	}
	if !node.HasCapability(store.NodeCapabilityExec) {
		return nil, ErrNodeNoExecCap
	}
	sender, online := reg.Get(nodeID)
	if !online {
		return nil, ErrNodeOffline
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultInvokeTimeout
	}
	if timeout > MaxInvokeTimeout {
		timeout = MaxInvokeTimeout
	}

	invokeID := uuid.NewString()
	waiter := reg.registerWaiter(invokeID)
	defer func() {
		// If we exit via timeout/cancel, drop the waiter so a late daemon
		// result cannot be delivered to a future, unrelated invocation that
		// reused the id space.
		reg.resolveWaiter(invokeID)
	}()

	sender.SendEvent(*protocol.NewEvent(protocol.EventNodeInvoke, map[string]any{
		"nodeId":    nodeID,
		"invokeId":  invokeID,
		"command":   req.Command,
		"args":      req.Args,
		"timeoutMs": timeout.Milliseconds(),
	}))
	slog.Info("nodes.invoke_sent",
		"node_id", nodeID,
		"invoke_id", invokeID,
		"command", req.Command,
		"timeout_ms", timeout.Milliseconds(),
	)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res := <-waiter:
		if res.Error != "" {
			return res, ErrInvokeRejected
		}
		return res, nil
	case <-timer.C:
		return nil, ErrInvokeTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TruncateResult caps daemon-provided streams to MaxResultBytes so one
// chatty invocation cannot blow up a frame or the agent context.
func TruncateResult(res *InvokeResult) *InvokeResult {
	if res == nil {
		return nil
	}
	res.Stdout = truncate(res.Stdout)
	res.Stderr = truncate(res.Stderr)
	return res
}

func truncate(s string) string {
	if len(s) <= MaxResultBytes {
		return s
	}
	return s[len(s)-MaxResultBytes:]
}
