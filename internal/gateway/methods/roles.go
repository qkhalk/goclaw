package methods

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// TenantAdminMethods handles tenant policy + custom-role RPC methods (Phase 4).
//
// All methods are tenant-admin gated: the caller must hold admin/owner inside
// the context tenant (system owners bypass via client.IsOwner). The WS router
// already classifies these methods as admin via MethodRole; this handler adds
// the tenant-scope check so a cross-tenant admin cannot mutate a tenant they
// do not administrate.
type TenantAdminMethods struct {
	policyStore store.TenantPolicyStore
	roleStore   store.TenantRoleStore
	tenantStore store.TenantStore
	msgBus      *bus.MessageBus
}

// NewTenantAdminMethods creates the tenant policy + role handler.
func NewTenantAdminMethods(policyStore store.TenantPolicyStore, roleStore store.TenantRoleStore, tenantStore store.TenantStore, msgBus *bus.MessageBus) *TenantAdminMethods {
	return &TenantAdminMethods{
		policyStore: policyStore,
		roleStore:   roleStore,
		tenantStore: tenantStore,
		msgBus:      msgBus,
	}
}

// Register wires the tenant policy + role RPC methods.
func (m *TenantAdminMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodTenantPoliciesGet, m.handlePolicyGet)
	router.Register(protocol.MethodTenantPoliciesUpdate, m.handlePolicyUpdate)
	router.Register(protocol.MethodRolesList, m.handleRolesList)
	router.Register(protocol.MethodRolesGet, m.handleRolesGet)
	router.Register(protocol.MethodRolesCreate, m.handleRolesCreate)
	router.Register(protocol.MethodRolesUpdate, m.handleRolesUpdate)
	router.Register(protocol.MethodRolesDelete, m.handleRolesDelete)
	router.Register(protocol.MethodRolePermissionsSet, m.handleRolePermissionsSet)
	router.Register(protocol.MethodRolePermissionsList, m.handleRolePermissionsList)
	router.Register(protocol.MethodRoleAssign, m.handleRoleAssign)
	router.Register(protocol.MethodRoleRevoke, m.handleRoleRevoke)
	router.Register(protocol.MethodRoleEffectiveGet, m.handleRoleEffectiveGet)
}

// hasTenantAdminFor reports whether the caller can administrate the tenant
// resolved from context (or a named one). Cross-tenant admins are rejected:
// the caller's own tenant (client.TenantID) must match the target tenant.
func (m *TenantAdminMethods) hasTenantAdminFor(ctx context.Context, client *gateway.Client, tid uuid.UUID) bool {
	if client.IsOwner() {
		return true
	}
	if client.TenantID() != tid || tid == uuid.Nil {
		return false
	}
	return permissions.HasMinRole(client.Role(), permissions.RoleAdmin)
}

func (m *TenantAdminMethods) handlePolicyGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	var params struct {
		TenantID string `json:"tenant_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	tid := store.TenantIDFromContext(ctx)
	if params.TenantID != "" {
		if client.IsOwner() {
			parsed, err := uuid.Parse(params.TenantID)
			if err != nil {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant_id")))
				return
			}
			tid = parsed
		}
	}
	if tid == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenant.policies.get")))
		return
	}
	if !m.hasTenantAdminFor(ctx, client, tid) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenant.policies.get")))
		return
	}

	policy, err := m.policyStore.GetTenantPolicy(ctx, tid)
	if err != nil {
		if err == store.ErrTenantPolicyNotFound {
			client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"policy": nil}))
			return
		}
		slog.Error("tenant.policies.get failed", "error", err, "tenant_id", tid)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "tenant policy")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"policy": policy}))
}

func (m *TenantAdminMethods) handlePolicyUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	var params struct {
		TenantID         string          `json:"tenant_id"`
		AllowedProviders []string        `json:"allowed_providers"`
		AllowedModels    []string        `json:"allowed_models"`
		MaxAgents        *int            `json:"max_agents"`
		MaxSessions      *int            `json:"max_sessions"`
		MaxTeams         *int            `json:"max_teams"`
		Quota            json.RawMessage `json:"quota"`
		Status           string          `json:"status"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.Status != "" && !store.ValidTenantPolicyStatus(params.Status) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidUpdates)))
		return
	}

	tid := store.TenantIDFromContext(ctx)
	if params.TenantID != "" {
		if client.IsOwner() {
			parsed, err := uuid.Parse(params.TenantID)
			if err != nil {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant_id")))
				return
			}
			tid = parsed
		}
	}
	if tid == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenant.policies.update")))
		return
	}
	if !m.hasTenantAdminFor(ctx, client, tid) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "tenant.policies.update")))
		return
	}

	policy := &store.TenantPolicy{
		TenantID:         tid,
		AllowedProviders: params.AllowedProviders,
		AllowedModels:    params.AllowedModels,
		MaxAgents:        params.MaxAgents,
		MaxSessions:      params.MaxSessions,
		MaxTeams:         params.MaxTeams,
		Quota:            params.Quota,
		Status:           params.Status,
	}
	if policy.Status == "" {
		policy.Status = store.TenantPolicyStatusActive
	}
	if err := m.policyStore.UpsertTenantPolicy(ctx, policy); err != nil {
		slog.Error("tenant.policies.update failed", "error", err, "tenant_id", tid)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgPolicyUpdateFailed)))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": "true"}))
}

func (m *TenantAdminMethods) handleRolesList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.list")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.list")))
		return
	}
	roles, err := m.roleStore.ListRoles(ctx)
	if err != nil {
		slog.Error("role.list failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "roles")))
		return
	}
	if roles == nil {
		roles = []store.TenantRole{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"roles": roles}))
}

func (m *TenantAdminMethods) handleRolesGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.get")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.get")))
		return
	}
	var params struct {
		RoleID string `json:"role_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	roleID, err := uuid.Parse(params.RoleID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role_id")))
		return
	}
	role, err := m.roleStore.GetRole(ctx, roleID)
	if err != nil {
		if err == store.ErrRoleNotFound {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgRoleNotFound)))
			return
		}
		slog.Error("role.get failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "role")))
		return
	}
	perms, err := m.roleStore.ListRolePermissions(ctx, roleID)
	if err != nil {
		slog.Error("role.get: permissions failed", "error", err)
		perms = []store.RolePermission{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"role": role, "permissions": perms}))
}

func (m *TenantAdminMethods) handleRolesCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.create")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.create")))
		return
	}
	var params struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Permissions []struct {
			Permission string `json:"permission"`
			Effect     string `json:"effect"`
		} `json:"permissions"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.Name == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleNameRequired)))
		return
	}
	if store.IsBuiltinTenantRole(params.Name) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected)))
		return
	}
	for _, p := range params.Permissions {
		if _, ok := permissions.MinRoleFor(permissions.CatalogResource(p.Permission), permissions.CatalogAction(p.Permission)); !ok {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleInvalidPermission, p.Permission)))
			return
		}
		if !store.ValidPermissionEffect(p.Effect) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleInvalidPermission, p.Effect)))
			return
		}
	}

	role := &store.TenantRole{
		ID:          store.GenNewID(),
		Name:        params.Name,
		Description: params.Description,
	}
	if err := m.roleStore.CreateRole(ctx, role); err != nil {
		slog.Error("role.create failed", "error", err)
		if err == store.ErrBuiltinRoleProtected {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected)))
			return
		}
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleNameConflict)))
		return
	}
	if len(params.Permissions) > 0 {
		perms := make([]store.RolePermission, 0, len(params.Permissions))
		for _, p := range params.Permissions {
			perms = append(perms, store.RolePermission{RoleID: role.ID, Permission: p.Permission, Effect: p.Effect})
		}
		if err := m.roleStore.SetRolePermissions(ctx, role.ID, perms); err != nil {
			slog.Error("role.create: permissions failed", "error", err)
		}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"role": role}))
}

func (m *TenantAdminMethods) handleRolesUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.update")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.update")))
		return
	}
	var params struct {
		RoleID      string  `json:"role_id"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	roleID, err := uuid.Parse(params.RoleID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role_id")))
		return
	}
	updates := make(map[string]any)
	if params.Name != "" {
		if store.IsBuiltinTenantRole(params.Name) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected)))
			return
		}
		updates["name"] = params.Name
	}
	if params.Description != nil {
		updates["description"] = *params.Description
	}
	if len(updates) == 0 {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidUpdates)))
		return
	}
	if err := m.roleStore.UpdateRole(ctx, roleID, updates); err != nil {
		slog.Error("role.update failed", "error", err)
		if err == store.ErrRoleNotFound {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgRoleNotFound)))
			return
		}
		if err == store.ErrBuiltinRoleProtected {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected)))
			return
		}
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "role", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": "true"}))
}

func (m *TenantAdminMethods) handleRolesDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.delete")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.delete")))
		return
	}
	var params struct {
		RoleID string `json:"role_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	roleID, err := uuid.Parse(params.RoleID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role_id")))
		return
	}
	if err := m.roleStore.DeleteRole(ctx, roleID); err != nil {
		slog.Error("role.delete failed", "error", err)
		if err == store.ErrRoleNotFound {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgRoleNotFound)))
			return
		}
		if err == store.ErrBuiltinRoleProtected {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected)))
			return
		}
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "role", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": "true"}))
}

func (m *TenantAdminMethods) handleRolePermissionsSet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.permissions.set")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.permissions.set")))
		return
	}
	var params struct {
		RoleID      string `json:"role_id"`
		Permissions []struct {
			Permission string `json:"permission"`
			Effect     string `json:"effect"`
		} `json:"permissions"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	roleID, err := uuid.Parse(params.RoleID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role_id")))
		return
	}
	perms := make([]store.RolePermission, 0, len(params.Permissions))
	for _, p := range params.Permissions {
		if _, ok := permissions.MinRoleFor(permissions.CatalogResource(p.Permission), permissions.CatalogAction(p.Permission)); !ok {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleInvalidPermission, p.Permission)))
			return
		}
		if !store.ValidPermissionEffect(p.Effect) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleInvalidPermission, p.Effect)))
			return
		}
		perms = append(perms, store.RolePermission{RoleID: roleID, Permission: p.Permission, Effect: p.Effect})
	}
	if err := m.roleStore.SetRolePermissions(ctx, roleID, perms); err != nil {
		slog.Error("role.permissions.set failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "role permissions", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": "true"}))
}

func (m *TenantAdminMethods) handleRolePermissionsList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.permissions.list")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.permissions.list")))
		return
	}
	var params struct {
		RoleID string `json:"role_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	roleID, err := uuid.Parse(params.RoleID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role_id")))
		return
	}
	perms, err := m.roleStore.ListRolePermissions(ctx, roleID)
	if err != nil {
		slog.Error("role.permissions.list failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "role permissions")))
		return
	}
	if perms == nil {
		perms = []store.RolePermission{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"permissions": perms}))
}

func (m *TenantAdminMethods) handleRoleAssign(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.assign")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.assign")))
		return
	}
	var params struct {
		RoleID string `json:"role_id"`
		UserID string `json:"user_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	roleID, err := uuid.Parse(params.RoleID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role_id")))
		return
	}
	if params.UserID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "user_id")))
		return
	}
	tid := store.TenantIDFromContext(ctx)
	if err := m.roleStore.AssignRoleToMember(ctx, tid, params.UserID, roleID); err != nil {
		slog.Error("role.assign failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "role assignment", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": "true"}))
}

func (m *TenantAdminMethods) handleRoleRevoke(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.revoke")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.revoke")))
		return
	}
	var params struct {
		RoleID string `json:"role_id"`
		UserID string `json:"user_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	roleID, err := uuid.Parse(params.RoleID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role_id")))
		return
	}
	if params.UserID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "user_id")))
		return
	}
	tid := store.TenantIDFromContext(ctx)
	if err := m.roleStore.RevokeRoleFromMember(ctx, tid, params.UserID, roleID); err != nil {
		slog.Error("role.revoke failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "role assignment", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": "true"}))
}

func (m *TenantAdminMethods) handleRoleEffectiveGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.effective.get")))
		return
	}
	if !client.IsOwner() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "role.effective.get")))
		return
	}
	var params struct {
		UserID string `json:"user_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.UserID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "user_id")))
		return
	}
	tid := store.TenantIDFromContext(ctx)
	role, err := m.tenantStore.GetUserRole(ctx, tid, params.UserID)
	if err != nil {
		slog.Error("role.effective.get: role lookup failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "role")))
		return
	}
	roleIDs, err := m.roleStore.ListMemberRoles(ctx, tid, params.UserID)
	if err != nil {
		slog.Error("role.effective.get: assignments failed", "error", err)
		roleIDs = []uuid.UUID{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"role":            role,
		"custom_role_ids": roleIDs,
	}))
}