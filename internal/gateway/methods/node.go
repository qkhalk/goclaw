package methods

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// Node lease timing (Paseo plan Phase 1 / docs/paseo-phase1-node-lease.md):
// clients heartbeat every NodeHeartbeatInterval (15s); the server grants a
// NodeLeaseTTL (60s) per touch. A WS close never invalidates the lease — it
// only marks it reconnecting.
const (
	NodeHeartbeatInterval = 15 * time.Second
	NodeLeaseTTL          = 60 * time.Second
)

// NodeMethods implements the node.* WS surface: device leases that decouple
// connectivity from authentication and agent sessions.
type NodeMethods struct {
	leases store.NodeLeaseStore
}

func NewNodeMethods(leases store.NodeLeaseStore) *NodeMethods {
	return &NodeMethods{leases: leases}
}

// Register wires the node.* methods into the method router.
func (m *NodeMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodNodeHello, m.handleHello)
	router.Register(protocol.MethodNodeHeartbeat, m.handleHeartbeat)
	router.Register(protocol.MethodNodeBye, m.handleBye)
}

// handleHello creates or resumes a lease. Reconnect with a valid resume token
// restores the lease without re-authentication side effects and reports how
// many timeline events the client should replay after its last seen seq.
func (m *NodeMethods) handleHello(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		// The router rejects non-connect frames before connect succeeds; an
		// unset role here is the defensive failure path.
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		NodeID      string `json:"nodeId"`
		ClientID    string `json:"clientId"`
		ResumeToken string `json:"resumeToken"`
		LastSeenSeq int64  `json:"lastSeenSeq"`
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

	resumed := false
	var existing *store.NodeLease
	if m.leases != nil && params.ResumeToken != "" {
		if l, err := m.leases.GetByResumeToken(ctx, params.ResumeToken); err == nil &&
			l.NodeID == params.NodeID && l.UserID == client.UserID() {
			resumed = true
			existing = l
		} else if err != nil {
			slog.Debug("node.hello_resume_lookup_failed", "error", err)
		}
	}

	lease := &store.NodeLease{
		NodeID:      params.NodeID,
		ClientID:    params.ClientID,
		UserID:      client.UserID(),
		TenantID:    tenantString(client),
		ResumeToken: params.ResumeToken,
		ExpiresAt:   time.Now().Add(NodeLeaseTTL),
	}
	if resumed && existing != nil {
		lease.ResumeToken = existing.ResumeToken // keep stable across reconnects
		lease.SessionEpoch = existing.SessionEpoch
	}
	if m.leases == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "node lease store not wired"))
		return
	}
	if err := m.leases.UpsertLease(ctx, lease); err != nil {
		slog.Warn("node.hello_upsert_failed", "node_id", params.NodeID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to persist node lease"))
		return
	}

	resp := map[string]any{
		"resumed": resumed,
		"lease": map[string]any{
			"nodeId":     lease.NodeID,
			"expiresAt":  lease.ExpiresAt.UTC().Format(time.RFC3339),
			"ttlSeconds": int(NodeLeaseTTL.Seconds()),
		},
	}
	if lease.ResumeToken != "" {
		resp["resumeToken"] = lease.ResumeToken
	}
	if params.LastSeenSeq > 0 {
		// Replay accounting: the client replays from its own cursor via
		// runs.events?afterSeq — hello only confirms whether the gap is sane.
		resp["lastSeenSeq"] = params.LastSeenSeq
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, resp))
}

// tenantString renders the client tenant UUID, empty for cross-tenant/master scope.
func tenantString(client *gateway.Client) string {
	tid := client.TenantID()
	if tid == uuid.Nil {
		return ""
	}
	return tid.String()
}

// handleHeartbeat extends the lease TTL. Missing lease → error so the client
// re-runs the hello handshake.
func (m *NodeMethods) handleHeartbeat(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
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
	if m.leases == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "node lease store not wired"))
		return
	}
	expiresAt := time.Now().Add(NodeLeaseTTL)
	if err := m.leases.TouchHeartbeat(ctx, params.NodeID, NodeLeaseTTL); err != nil {
		slog.Warn("node.heartbeat_failed", "node_id", params.NodeID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "node lease expired or unknown"))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"expiresAt":  expiresAt.UTC().Format(time.RFC3339),
		"ttlSeconds": int(NodeLeaseTTL.Seconds()),
	}))
}

// handleBye gracefully releases the lease (intentional logout/tab close with
// unload beacon). The lease is marked offline_grace so a fast reconnect can
// still resume it within the grace window.
func (m *NodeMethods) handleBye(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		NodeID string `json:"nodeId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if m.leases == nil {
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"released": false}))
		return
	}
	if params.NodeID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "nodeId")))
		return
	}
	err := m.leases.MarkStatus(ctx, params.NodeID, store.NodeLeaseStatusOfflineGrace)
	if err != nil {
		// Best-effort: an unknown/expired lease is fine to say goodbye to.
		slog.Debug("node.bye_mark_failed", "node_id", params.NodeID, "error", err)
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"released": true}))
}
