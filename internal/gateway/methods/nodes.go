package methods

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/nodes"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// NodeHeartbeatSeconds is how often a registered daemon should re-send
// nodes.register to stay marked online. Half the registry's online TTL, so a
// single missed beat is tolerated but two are not.
const NodeHeartbeatSeconds = int(nodes.DefaultOnlineTTL / (2 * time.Second))

// maxNodeNameLen bounds user-supplied node names.
const maxNodeNameLen = 255

// NodesMethods implements the nodes.* WS surface (inheritance plan Phase 2):
// the compute-node registry. Distinct from node.* (device leases) and from
// device.pair.* (channel pairing).
//
// Daemon flow: connect as a normal WS client (connect frame first, gateway
// token), then nodes.register with its node key. The gateway validates the
// key's SHA-256 against node_key_hash — the plaintext is revealed exactly
// once when an admin creates it via nodes.list {create:true}.
type NodesMethods struct {
	store    store.NodeStore
	registry *nodes.Registry
	msgBus   bus.EventPublisher
}

// NewNodesMethods wires the nodes.* handlers. The registry is shared with the
// node_exec tool and the invoke helper; nil registry degrades online state
// (every node reads offline) without breaking registration bookkeeping. A nil
// publisher skips audit + node.state broadcasts.
func NewNodesMethods(nodeStore store.NodeStore, registry *nodes.Registry, msgBus bus.EventPublisher) *NodesMethods {
	return &NodesMethods{store: nodeStore, registry: registry, msgBus: msgBus}
}

// Register wires the nodes.* methods into the method router.
func (m *NodesMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodNodesRegister, m.handleRegister)
	router.Register(protocol.MethodNodesList, m.handleList)
	router.Register(protocol.MethodNodesRevoke, m.handleRevoke)
	router.Register(protocol.MethodNodesResult, m.handleResult)
}

// handleRegister authenticates a daemon by node key and creates/refreshes its
// live-connection entry. First registration happens with trust=pending (the
// admin created the key); nothing executes until the node is trusted.
func (m *NodesMethods) handleRegister(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	if m.store == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "node store not wired"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		NodeKey      string   `json:"nodeKey"`
		Name         string   `json:"name"`
		Platform     string   `json:"platform"`
		Capabilities []string `json:"capabilities"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if !nodes.ValidateKeyFormat(params.NodeKey) {
		slog.Warn("security.node_register_bad_key_format", "client", client.ID())
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "nodeKey")))
		return
	}
	if len(params.Platform) > 64 {
		params.Platform = params.Platform[:64]
	}
	caps := filterCapabilities(params.Capabilities)

	node, err := m.store.GetByKeyHash(ctx, nodes.HashKey(params.NodeKey))
	if err != nil || node == nil {
		// Unknown key — do not reveal whether the hash exists.
		slog.Warn("security.node_register_unknown_key", "client", client.ID(), "remote", client.RemoteAddr())
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "invalid node key"))
		return
	}
	if node.Trust == store.NodeTrustRevoked {
		slog.Warn("security.node_register_revoked", "node_id", node.ID, "client", client.ID())
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrFailedPrecondition, "node key revoked; create a new key"))
		return
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		name = node.Name
	}
	if len(name) > maxNodeNameLen {
		name = name[:maxNodeNameLen]
	}
	if err := m.store.UpdateRegistration(ctx, node.ID, name, params.Platform, caps); err != nil {
		slog.Warn("nodes.register_update_failed", "node_id", node.ID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "registration failed")))
		return
	}
	// Track liveness + tenant from the node row (the daemon's connection
	// tenant may differ, e.g. a master-scope gateway token operating a
	// tenant's node).
	nodeTenant := ""
	if node.TenantID != nil {
		nodeTenant = *node.TenantID
	}
	if m.registry != nil {
		m.registry.Set(node.ID, client, nodeTenant)
	}

	emitAudit(m.msgBus, client, "nodes.registered", "node", node.ID)
	m.broadcastNodeState(node.ID, nodeTenant, node.Trust, true)

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"nodeId":        node.ID,
		"name":          name,
		"trust":         node.Trust,
		"capabilities":  caps,
		"heartbeatSecs": NodeHeartbeatSeconds,
	}))
}

// handleList lists the tenant's compute nodes with derived online state.
// Admin actions piggyback here (fail-closed to admin/owner inside the
// handler, because nodes.list itself is viewer-readable):
//   - {create:true, name} → provision a key; the plaintext is returned once.
//   - {nodeId, trust:"trusted"|"pending"} → approve/un-approve a node.
func (m *NodesMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if m.store == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "node store not wired"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		Create bool   `json:"create"`
		Name   string `json:"name"`
		NodeID string `json:"nodeId"`
		Trust  string `json:"trust"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	switch {
	case params.Create:
		m.handleCreateKey(ctx, client, req, params.Name)
		return
	case params.NodeID != "" && params.Trust != "":
		m.handleSetTrust(ctx, client, req, params.NodeID, params.Trust)
		return
	}

	rows, err := m.store.List(ctx)
	if err != nil {
		slog.Warn("nodes.list_failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "list failed")))
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, n := range rows {
		items = append(items, nodeListItem(n, m.registry))
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"nodes": items}))
}

// handleCreateKey provisions a node row + bearer key. Admin/owner only. The
// plaintext key is returned exactly once; only its SHA-256 is persisted.
func (m *NodesMethods) handleCreateKey(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, name string) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		slog.Warn("security.node_key_create_denied", "role", client.Role(), "client", client.ID())
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized,
			i18n.T(locale, i18n.MsgPermissionDenied, "creating node keys requires admin")))
		return
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNodeNameLen {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "name")))
		return
	}
	key, err := nodes.GenerateKey()
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, err.Error())))
		return
	}
	node := &store.Node{
		Name:        name,
		NodeKeyHash: nodes.HashKey(key),
		Trust:       store.NodeTrustPending,
	}
	if err := m.store.Create(ctx, node); err != nil {
		slog.Warn("nodes.key_create_failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "create failed")))
		return
	}
	emitAudit(m.msgBus, client, "nodes.key_created", "node", node.ID)
	slog.Info("nodes.key_created", "node_id", node.ID, "actor", client.UserID())

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"created": true,
		"node":    nodeListItem(node, m.registry),
		"key":     key, // reveal-once
	}))
}

// handleSetTrust transitions pending→trusted (or back to pending). Admin/owner
// only; revoked is terminal (store rejects leaving it).
func (m *NodesMethods) handleSetTrust(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, nodeID, trust string) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		slog.Warn("security.node_trust_denied", "role", client.Role(), "client", client.ID())
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized,
			i18n.T(locale, i18n.MsgPermissionDenied, "changing node trust requires admin")))
		return
	}
	if trust != store.NodeTrustTrusted && trust != store.NodeTrustPending {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "trust=pending|trusted")))
		return
	}
	// Tenant-scoped existence check first: operators must not be able to
	// probe other tenants' node ids through the error.
	if _, err := m.store.GetByID(ctx, nodeID); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "node not found"))
		return
	}
	if err := m.store.SetTrust(ctx, nodeID, trust); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrFailedPrecondition, err.Error()))
		return
	}
	emitAudit(m.msgBus, client, "nodes.trust_changed", "node", nodeID)
	m.broadcastNodeState(nodeID, tenantString(client), trust, m.registry != nil && m.registry.Online(nodeID))
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"nodeId": nodeID, "trust": trust}))
}

// handleRevoke marks the node revoked (terminal) and force-disconnects its
// live connection — mirroring the pairing revoke flow. Admin/owner only.
func (m *NodesMethods) handleRevoke(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		slog.Warn("security.node_revoke_denied", "role", client.Role(), "client", client.ID())
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized,
			i18n.T(locale, i18n.MsgPermissionDenied, "revoking nodes requires admin")))
		return
	}
	var params struct {
		NodeID string `json:"nodeId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.NodeID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "nodeId")))
		return
	}
	if m.store == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "node store not wired"))
		return
	}
	// Tenant-scoped existence check (same probe protection as SetTrust).
	nodeTenant := tenantString(client)
	if node, err := m.store.GetByID(ctx, params.NodeID); err == nil && node != nil && node.TenantID != nil {
		nodeTenant = *node.TenantID
	} else if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "node not found"))
		return
	}
	if err := m.store.Revoke(ctx, params.NodeID); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrFailedPrecondition, err.Error()))
		return
	}
	// Force-disconnect the live daemon (its key is dead) — same pattern as
	// pairing revoke's DisconnectByPairing, via the node registry.
	if m.registry != nil {
		m.registry.Disconnect(params.NodeID)
	}
	emitAudit(m.msgBus, client, "nodes.revoked", "node", params.NodeID)
	m.broadcastNodeState(params.NodeID, nodeTenant, store.NodeTrustRevoked, false)
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"revoked": true, "nodeId": params.NodeID}))
}

// handleResult receives a daemon's invoke outcome and routes it to the
// in-memory waiter matching the correlation id. Only the currently registered
// connection for that node may post results.
func (m *NodesMethods) handleResult(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if m.registry == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "node registry not wired"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		NodeID     string `json:"nodeId"`
		InvokeID   string `json:"invokeId"`
		ExitCode   int    `json:"exitCode"`
		Stdout     string `json:"stdout"`
		Stderr     string `json:"stderr"`
		Error      string `json:"error"`
		DurationMs int64  `json:"durationMs"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.InvokeID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "invokeId")))
		return
	}
	// Sender check: the result must come from the node's registered
	// connection — an admin console cannot inject outcomes for a node.
	sender, online := m.registry.Get(params.NodeID)
	if !online || sender != client {
		slog.Warn("security.node_result_unregistered_sender",
			"node_id", params.NodeID, "client", client.ID())
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not the registered connection for this node"))
		return
	}

	res := nodes.TruncateResult(&nodes.InvokeResult{
		InvokeID:   params.InvokeID,
		NodeID:     params.NodeID,
		ExitCode:   params.ExitCode,
		Stdout:     params.Stdout,
		Stderr:     params.Stderr,
		Error:      params.Error,
		DurationMs: params.DurationMs,
	})
	delivered := m.registry.DeliverResult(res)
	if !delivered {
		slog.Debug("nodes.result_unmatched", "node_id", params.NodeID, "invoke_id", params.InvokeID)
	}
	// Results double as liveness signals.
	m.registry.Touch(params.NodeID)
	if m.store != nil {
		_ = m.store.TouchSeen(ctx, params.NodeID) // best-effort durable echo
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"delivered": delivered}))
}

// nodeListItem renders one node for nodes.list / create responses (camelCase
// wire format, never exposes the key hash).
func nodeListItem(n *store.Node, reg *nodes.Registry) map[string]any {
	online := false
	if reg != nil {
		online = reg.Online(n.ID)
	}
	item := map[string]any{
		"id":           n.ID,
		"name":         n.Name,
		"platform":     n.Platform,
		"capabilities": n.Capabilities,
		"trust":        n.Trust,
		"online":       online,
		"createdAt":    n.CreatedAt.UTC().Format(time.RFC3339),
	}
	if n.TenantID != nil {
		item["tenantId"] = *n.TenantID
	}
	if n.LastSeenAt != nil {
		item["lastSeenAt"] = n.LastSeenAt.UTC().Format(time.RFC3339)
	}
	if n.RevokedAt != nil {
		item["revokedAt"] = n.RevokedAt.UTC().Format(time.RFC3339)
	}
	return item
}

// filterCapabilities keeps only the known capability names, deduplicated,
// preserving order.
func filterCapabilities(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, c := range in {
		c = strings.TrimSpace(strings.ToLower(c))
		if c == "" || seen[c] || !store.ValidNodeCapability(c) {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// broadcastNodeState emits a node.state event (tenant-scoped) so admin UIs
// can refresh live. Best-effort: nil bus is a no-op.
func (m *NodesMethods) broadcastNodeState(nodeID, tenantID, trust string, online bool) {
	if m.msgBus == nil {
		return
	}
	tid := uuid.Nil
	if tenantID != "" {
		if parsed, err := uuid.Parse(tenantID); err == nil {
			tid = parsed
		}
	}
	m.msgBus.Broadcast(bus.Event{
		Name:     protocol.EventNodeState,
		Payload:  map[string]any{"nodeId": nodeID, "trust": trust, "online": online},
		TenantID: tid,
	})
}
