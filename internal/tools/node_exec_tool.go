package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/nodes"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// nodeExecMaxArgsBytes / nodeExecMaxArgBytes bound LLM-supplied argv.
const (
	nodeExecMaxArgBytes   = 1024
	nodeExecMaxArgs       = 64
	nodeExecDefaultSecs   = 300
	nodeExecMaxSecs       = 600       // nodes.MaxInvokeTimeout
	nodeExecMaxOutputFeat = 16 * 1024 // stream tail surfaced to the LLM per stream
)

// NodeExecTool executes an allowlisted command on a named, trusted compute
// node via the gateway's in-memory node registry (inheritance plan Phase 2).
// Every gate lives below this tool too (trust + capability + tenant in
// nodes.Invoke; the daemon's own allowlist), so the tool is a router, not the
// security boundary. Registered for the full (PG) edition only.
type NodeExecTool struct {
	nodeStore store.NodeStore
	registry  *nodes.Registry
	audit     bus.EventPublisher
}

func NewNodeExecTool(nodeStore store.NodeStore, registry *nodes.Registry, audit bus.EventPublisher) *NodeExecTool {
	return &NodeExecTool{nodeStore: nodeStore, registry: registry, audit: audit}
}

func (t *NodeExecTool) Name() string { return "node_exec" }

func (t *NodeExecTool) Description() string {
	return "Execute a command on a trusted registered compute node (goclaw-node daemon). " +
		"The node must be trusted and the command must be on that node's allowlist. " +
		"Returns exit code, stdout and stderr tails."
}

func (t *NodeExecTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"node": map[string]any{
				"type":        "string",
				"description": "Target node name or node UUID",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Binary to execute (must be on the node's allowlist)",
			},
			"args": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"timeout_sec": map[string]any{
				"type":    "integer",
				"default": 300,
			},
		},
		"required": []string{"node", "command"},
	}
}

// Execute resolves the target node, routes the invocation through the
// gateway registry, and formats the outcome for the LLM.
func (t *NodeExecTool) Execute(ctx context.Context, args map[string]any) *Result {
	locale := store.LocaleFromContext(ctx)
	agentID := store.AgentIDFromContext(ctx).String()

	command, _ := args["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return ErrorResult(i18n.T(locale, i18n.MsgRequired, "command"))
	}
	if strings.ContainsRune(command, '\x00') {
		return ErrorResult("command contains invalid NUL byte")
	}
	execArgs, err := coerceNodeExecArgs(args["args"])
	if err != nil {
		return ErrorResult("args: " + err.Error())
	}

	nodeRef, _ := args["node"].(string)
	nodeRef = strings.TrimSpace(nodeRef)
	if nodeRef == "" {
		return ErrorResult(i18n.T(locale, i18n.MsgRequired, "node"))
	}

	node, err := t.resolveNode(ctx, nodeRef)
	if err != nil {
		return ErrorResult(err.Error())
	}

	timeoutSec, _ := args["timeout_sec"].(float64)
	if timeoutSec <= 0 {
		timeoutSec = nodeExecDefaultSecs
	}
	if timeoutSec > nodeExecMaxSecs {
		timeoutSec = nodeExecMaxSecs
	}

	res, invokeErr := nodes.Invoke(ctx, t.registry, t.nodeStore, node.ID, nodes.InvokeRequest{
		Command: command,
		Args:    execArgs,
		Timeout: time.Duration(timeoutSec) * time.Second,
	})
	t.auditInvoke(agentID, node, invokeErr)

	switch {
	case errors.Is(invokeErr, nodes.ErrInvokeRejected):
		return ErrorResult(fmt.Sprintf("node rejected the command: %s\nstderr:\n%s", res.Error, tail(res.Stderr)))
	case errors.Is(invokeErr, nodes.ErrInvokeTimeout):
		return ErrorResult("node invocation timed out (the node accepted the job but never reported a result)")
	case invokeErr != nil:
		return ErrorResult(invokeErr.Error())
	}

	out := fmt.Sprintf("exit_code: %d\nnode: %s\nstdout:\n%s\nstderr:\n%s",
		res.ExitCode, node.Name, tail(res.Stdout), tail(res.Stderr))
	if res.ExitCode != 0 {
		return ErrorResult(out)
	}
	return SilentResult(out)
}

// resolveNode accepts a node UUID or an exact name (tenant-scoped). Name
// resolution lists the tenant's nodes — fine at registry scale and avoids a
// bespoke lookup on the store interface.
func (t *NodeExecTool) resolveNode(ctx context.Context, ref string) (*store.Node, error) {
	if id, parseErr := uuid.Parse(ref); parseErr == nil {
		n, err := t.nodeStore.GetByID(ctx, id.String())
		if err != nil || n == nil {
			return nil, fmt.Errorf("node %q not found", ref)
		}
		return n, nil
	}
	rows, err := t.nodeStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("node lookup failed: %w", err)
	}
	for _, n := range rows {
		if n.Name == ref {
			return n, nil
		}
	}
	return nil, fmt.Errorf("node %q not found (use the exact node name or UUID)", ref)
}

// auditInvoke emits one audit event per invocation, denied or executed, so
// node exec is fully auditable alongside workstation exec.
func (t *NodeExecTool) auditInvoke(agentID string, node *store.Node, invokeErr error) {
	if t.audit == nil {
		return
	}
	action := "nodes.invoke"
	if invokeErr != nil {
		action = "nodes.invoke_failed"
	}
	t.audit.Broadcast(bus.Event{
		Name: protocol.EventAuditLog,
		Payload: bus.AuditEventPayload{
			ActorType:  "agent",
			ActorID:    agentID,
			Action:     action,
			EntityType: "node",
			EntityID:   node.ID,
			TenantID:   nodeTenantID(node),
		},
	})
}

func nodeTenantID(n *store.Node) uuid.UUID {
	if n.TenantID == nil || *n.TenantID == "" {
		return store.MasterTenantID
	}
	if id, err := uuid.Parse(*n.TenantID); err == nil {
		return id
	}
	return store.MasterTenantID
}

// coerceNodeExecArgs converts the JSON-decoded args array with per-item and
// total bounds.
func coerceNodeExecArgs(raw any) ([]string, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []any:
		if len(v) > nodeExecMaxArgs {
			return nil, fmt.Errorf("exceeds %d item limit", nodeExecMaxArgs)
		}
		out := make([]string, 0, len(v))
		for _, elem := range v {
			s, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("each arg must be a string")
			}
			if strings.ContainsRune(s, '\x00') {
				return nil, fmt.Errorf("arg contains invalid NUL byte")
			}
			if len(s) > nodeExecMaxArgBytes {
				return nil, fmt.Errorf("arg exceeds %d byte limit", nodeExecMaxArgBytes)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("args must be an array of strings")
	}
}

// tail keeps the last nodeExecMaxOutputFeat bytes of a stream for the LLM.
func tail(s string) string {
	if len(s) <= nodeExecMaxOutputFeat {
		return s
	}
	return s[len(s)-nodeExecMaxOutputFeat:]
}
