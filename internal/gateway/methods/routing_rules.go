package methods

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// RoutingRulesMethods handles the routing.rules.* RPC surface (inheritance
// plan Phase 4): tenant-scoped inbound routing rules CRUD.
//
// All methods are tenant-admin gated: the caller must hold admin/owner inside
// the context tenant (system owners bypass via client.IsOwner). The router
// classifies routing.rules.list as a read and routing.rules.set/delete as
// admin via MethodRole; this handler adds the tenant-scope check so a
// cross-tenant admin cannot read or mutate rules of a tenant they do not
// administrate.
type RoutingRulesMethods struct {
	rules  store.RoutingRulesStore
	agents store.AgentStore // optional: validates targetAgentId exists
}

// NewRoutingRulesMethods creates the routing rules handler. agents may be
// nil (validation then accepts any well-formed UUID).
func NewRoutingRulesMethods(rules store.RoutingRulesStore, agents store.AgentStore) *RoutingRulesMethods {
	return &RoutingRulesMethods{rules: rules, agents: agents}
}

// Register wires the routing.rules.* RPC methods.
func (m *RoutingRulesMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodRoutingRulesList, m.handleList)
	router.Register(protocol.MethodRoutingRulesSet, m.handleSet)
	router.Register(protocol.MethodRoutingRulesDelete, m.handleDelete)
}

// hasTenantAdminFor reports whether the caller can administrate the tenant
// resolved from context. Cross-tenant admins are rejected: the caller's own
// tenant (client.TenantID) must match the target tenant.
func (m *RoutingRulesMethods) hasTenantAdminFor(ctx context.Context, client *gateway.Client) bool {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		return false // rules are always tenant-scoped, never global
	}
	if client.IsOwner() {
		return true
	}
	return client.TenantID() == tid && permissions.HasMinRole(client.Role(), permissions.RoleAdmin)
}

func (m *RoutingRulesMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !m.hasTenantAdminFor(ctx, client) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "routing.rules.list")))
		return
	}
	tid := store.TenantIDFromContext(ctx)
	rules, err := m.rules.ListRules(ctx, tid.String())
	if err != nil {
		slog.Error("routing.rules.list failed", "error", err, "tenant_id", tid)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "routing rules")))
		return
	}
	if rules == nil {
		rules = []*store.RoutingRule{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"rules": rules}))
}

func (m *RoutingRulesMethods) handleSet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !m.hasTenantAdminFor(ctx, client) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "routing.rules.set")))
		return
	}
	var params struct {
		ID            string                   `json:"id"`
		Priority      *int                     `json:"priority"`
		Match         store.RoutingRuleMatch   `json:"match"`
		TargetAgentID string                   `json:"targetAgentId"`
		Enabled       *bool                    `json:"enabled"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.TargetAgentID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "targetAgentId")))
		return
	}
	if _, err := uuid.Parse(params.TargetAgentID); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "targetAgentId")))
		return
	}
	// Optional referential check so admins get immediate feedback instead of
	// a rule that silently never routes.
	if m.agents != nil {
		agentUUID, err := uuid.Parse(params.TargetAgentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "targetAgentId")))
			return
		}
		if ag, err := m.agents.GetByID(ctx, agentUUID); err != nil || ag == nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "targetAgentId")))
			return
		}
	}

	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}
	priority := 100
	if params.Priority != nil {
		priority = *params.Priority
	}
	if params.ID != "" {
		if _, err := uuid.Parse(params.ID); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "id")))
			return
		}
	}

	rule := &store.RoutingRule{
		ID:            params.ID,
		TenantID:      store.TenantIDFromContext(ctx).String(),
		Priority:      priority,
		Match:         params.Match,
		TargetAgentID: params.TargetAgentID,
		Enabled:       enabled,
	}
	if err := m.rules.UpsertRule(ctx, rule); err != nil {
		if err == store.ErrRoutingRuleNotFound {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "routing rule", params.ID)))
			return
		}
		slog.Error("routing.rules.set failed", "error", err, "tenant_id", rule.TenantID)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToSave, "routing rule", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"rule": rule}))
}

func (m *RoutingRulesMethods) handleDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !m.hasTenantAdminFor(ctx, client) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "routing.rules.delete")))
		return
	}
	var params struct {
		ID string `json:"id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	ruleID, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "id")))
		return
	}
	tid := store.TenantIDFromContext(ctx)
	if err := m.rules.DeleteRule(ctx, tid.String(), ruleID.String()); err != nil {
		if err == store.ErrRoutingRuleNotFound {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "routing rule", ruleID.String())))
			return
		}
		slog.Error("routing.rules.delete failed", "error", err, "tenant_id", tid)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "routing rule", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}
