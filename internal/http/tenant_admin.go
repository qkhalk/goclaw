package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// TenantAdminHandler serves the tenant policy + RBAC custom-role HTTP surface
// (Phase 4). All routes are tenant-admin gated via requireTenantAdmin, matching
// the WS tenant.policies.* / role.* RPC methods.
type TenantAdminHandler struct {
	policyStore store.TenantPolicyStore
	roleStore   store.TenantRoleStore
	tenantStore store.TenantStore
}

// NewTenantAdminHandler creates the tenant admin HTTP handler.
func NewTenantAdminHandler(policyStore store.TenantPolicyStore, roleStore store.TenantRoleStore, tenantStore store.TenantStore) *TenantAdminHandler {
	return &TenantAdminHandler{
		policyStore: policyStore,
		roleStore:   roleStore,
		tenantStore: tenantStore,
	}
}

// RegisterRoutes registers the tenant policy + role admin routes.
func (h *TenantAdminHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/tenants/{id}/policies", h.requireTenantAdmin(h.handlePolicyGet))
	mux.HandleFunc("PUT /v1/tenants/{id}/policies", h.requireTenantAdmin(h.handlePolicyUpdate))
	mux.HandleFunc("DELETE /v1/tenants/{id}/policies", h.requireTenantAdmin(h.handlePolicyDelete))
	mux.HandleFunc("GET /v1/tenants/{id}/roles", h.requireTenantAdmin(h.handleRolesList))
	mux.HandleFunc("POST /v1/tenants/{id}/roles", h.requireTenantAdmin(h.handleRolesCreate))
	mux.HandleFunc("GET /v1/tenants/{id}/roles/{roleID}", h.requireTenantAdmin(h.handleRolesGet))
	mux.HandleFunc("PUT /v1/tenants/{id}/roles/{roleID}", h.requireTenantAdmin(h.handleRolesUpdate))
	mux.HandleFunc("DELETE /v1/tenants/{id}/roles/{roleID}", h.requireTenantAdmin(h.handleRolesDelete))
	mux.HandleFunc("PUT /v1/tenants/{id}/roles/{roleID}/permissions", h.requireTenantAdmin(h.handleRolePermissionsSet))
	mux.HandleFunc("GET /v1/tenants/{id}/roles/{roleID}/permissions", h.requireTenantAdmin(h.handleRolePermissionsList))
	mux.HandleFunc("PUT /v1/tenants/{id}/roles/{roleID}/assign", h.requireTenantAdmin(h.handleRoleAssign))
	mux.HandleFunc("DELETE /v1/tenants/{id}/roles/{roleID}/assign/{userID}", h.requireTenantAdmin(h.handleRoleRevoke))
}

// requireTenantAdmin wraps a handler with tenant-admin auth (owner/dba in the
// path tenant, or system owner).
func (h *TenantAdminHandler) requireTenantAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireTenantAdmin(w, r, h.tenantStore) {
			return
		}
		next(w, r)
	}
}

func (h *TenantAdminHandler) pathTenantID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		locale := store.LocaleFromContext(r.Context())
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "tenant"))
		return uuid.Nil, false
	}
	return id, true
}

func (h *TenantAdminHandler) handlePolicyGet(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	policy, err := h.policyStore.GetTenantPolicy(r.Context(), tid)
	if err != nil {
		if err == store.ErrTenantPolicyNotFound {
			writeJSON(w, http.StatusOK, map[string]any{"policy": nil})
			return
		}
		slog.Error("tenant.policies.get", "error", err, "tenant_id", tid)
		locale := store.LocaleFromContext(r.Context())
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "tenant policy"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": policy})
}

func (h *TenantAdminHandler) handlePolicyUpdate(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	var params struct {
		AllowedProviders []string          `json:"allowed_providers"`
		AllowedModels    []string          `json:"allowed_models"`
		MaxAgents        *int              `json:"max_agents"`
		MaxSessions      *int              `json:"max_sessions"`
		MaxTeams         *int              `json:"max_teams"`
		Quota            json.RawMessage   `json:"quota"`
		Status           string            `json:"status"`
	}
	if !bindJSON(w, r, locale, &params) {
		return
	}
	if params.Status != "" && !store.ValidTenantPolicyStatus(params.Status) {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidUpdates))
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
	if err := h.policyStore.UpsertTenantPolicy(r.Context(), policy); err != nil {
		slog.Error("tenant.policies.update", "error", err, "tenant_id", tid)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgPolicyUpdateFailed))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *TenantAdminHandler) handlePolicyDelete(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	if err := h.policyStore.DeleteTenantPolicy(r.Context(), tid); err != nil {
		slog.Error("tenant.policies.delete", "error", err, "tenant_id", tid)
		locale := store.LocaleFromContext(r.Context())
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "tenant policy", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *TenantAdminHandler) handleRolesList(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	ctx := store.WithTenantID(r.Context(), tid)
	roles, err := h.roleStore.ListRoles(ctx)
	if err != nil {
		slog.Error("role.list", "error", err, "tenant_id", tid)
		locale := store.LocaleFromContext(r.Context())
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "roles"))
		return
	}
	if roles == nil {
		roles = []store.TenantRole{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (h *TenantAdminHandler) handleRolesGet(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	roleID, err := uuid.Parse(r.PathValue("roleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role"))
		return
	}
	ctx := store.WithTenantID(r.Context(), tid)
	role, err := h.roleStore.GetRole(ctx, roleID)
	if err != nil {
		if err == store.ErrRoleNotFound {
			writeError(w, http.StatusNotFound, protocol.ErrNotFound, i18n.T(locale, i18n.MsgRoleNotFound))
			return
		}
		slog.Error("role.get", "error", err, "tenant_id", tid)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "role"))
		return
	}
	perms, err := h.roleStore.ListRolePermissions(ctx, roleID)
	if err != nil {
		slog.Error("role.get permissions", "error", err)
		perms = []store.RolePermission{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"role": role, "permissions": perms})
}

func (h *TenantAdminHandler) handleRolesCreate(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	var params struct {
		Name        string `json:"name"`
		Description *string `json:"description"`
		Permissions []struct {
			Permission string `json:"permission"`
			Effect     string `json:"effect"`
		} `json:"permissions"`
	}
	if !bindJSON(w, r, locale, &params) {
		return
	}
	if params.Name == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleNameRequired))
		return
	}
	if store.IsBuiltinTenantRole(params.Name) {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected))
		return
	}
	for _, p := range params.Permissions {
		if _, ok := permissions.MinRoleFor(permissions.CatalogResource(p.Permission), permissions.CatalogAction(p.Permission)); !ok {
			writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleInvalidPermission, p.Permission))
			return
		}
		if !store.ValidPermissionEffect(p.Effect) {
			writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleInvalidPermission, p.Effect))
			return
		}
	}

	ctx := store.WithTenantID(r.Context(), tid)
	role := &store.TenantRole{
		ID:          store.GenNewID(),
		Name:        params.Name,
		Description: params.Description,
	}
	if err := h.roleStore.CreateRole(ctx, role); err != nil {
		slog.Error("role.create", "error", err, "tenant_id", tid)
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleNameConflict))
		return
	}
	if len(params.Permissions) > 0 {
		perms := make([]store.RolePermission, 0, len(params.Permissions))
		for _, p := range params.Permissions {
			perms = append(perms, store.RolePermission{RoleID: role.ID, Permission: p.Permission, Effect: p.Effect})
		}
		if err := h.roleStore.SetRolePermissions(ctx, role.ID, perms); err != nil {
			slog.Error("role.create permissions", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"role": role})
}

func (h *TenantAdminHandler) handleRolesUpdate(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	roleID, err := uuid.Parse(r.PathValue("roleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role"))
		return
	}
	var params struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if !bindJSON(w, r, locale, &params) {
		return
	}
	updates := make(map[string]any)
	if params.Name != "" {
		if store.IsBuiltinTenantRole(params.Name) {
			writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected))
			return
		}
		updates["name"] = params.Name
	}
	if params.Description != nil {
		updates["description"] = *params.Description
	}
	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidUpdates))
		return
	}
	ctx := store.WithTenantID(r.Context(), tid)
	if err := h.roleStore.UpdateRole(ctx, roleID, updates); err != nil {
		slog.Error("role.update", "error", err, "tenant_id", tid)
		if err == store.ErrRoleNotFound {
			writeError(w, http.StatusNotFound, protocol.ErrNotFound, i18n.T(locale, i18n.MsgRoleNotFound))
			return
		}
		if err == store.ErrBuiltinRoleProtected {
			writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected))
			return
		}
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "role", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *TenantAdminHandler) handleRolesDelete(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	roleID, err := uuid.Parse(r.PathValue("roleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role"))
		return
	}
	ctx := store.WithTenantID(r.Context(), tid)
	if err := h.roleStore.DeleteRole(ctx, roleID); err != nil {
		slog.Error("role.delete", "error", err, "tenant_id", tid)
		if err == store.ErrRoleNotFound {
			writeError(w, http.StatusNotFound, protocol.ErrNotFound, i18n.T(locale, i18n.MsgRoleNotFound))
			return
		}
		if err == store.ErrBuiltinRoleProtected {
			writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleBuiltinProtected))
			return
		}
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "role", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *TenantAdminHandler) handleRolePermissionsSet(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	roleID, err := uuid.Parse(r.PathValue("roleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role"))
		return
	}
	var params struct {
		Permissions []struct {
			Permission string `json:"permission"`
			Effect     string `json:"effect"`
		} `json:"permissions"`
	}
	if !bindJSON(w, r, locale, &params) {
		return
	}
	perms := make([]store.RolePermission, 0, len(params.Permissions))
	for _, p := range params.Permissions {
		if _, ok := permissions.MinRoleFor(permissions.CatalogResource(p.Permission), permissions.CatalogAction(p.Permission)); !ok {
			writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleInvalidPermission, p.Permission))
			return
		}
		if !store.ValidPermissionEffect(p.Effect) {
			writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRoleInvalidPermission, p.Effect))
			return
		}
		perms = append(perms, store.RolePermission{RoleID: roleID, Permission: p.Permission, Effect: p.Effect})
	}
	ctx := store.WithTenantID(r.Context(), tid)
	if err := h.roleStore.SetRolePermissions(ctx, roleID, perms); err != nil {
		slog.Error("role.permissions.set", "error", err, "tenant_id", tid)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "role permissions", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *TenantAdminHandler) handleRolePermissionsList(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	roleID, err := uuid.Parse(r.PathValue("roleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role"))
		return
	}
	ctx := store.WithTenantID(r.Context(), tid)
	perms, err := h.roleStore.ListRolePermissions(ctx, roleID)
	if err != nil {
		slog.Error("role.permissions.list", "error", err, "tenant_id", tid)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "role permissions"))
		return
	}
	if perms == nil {
		perms = []store.RolePermission{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": perms})
}

func (h *TenantAdminHandler) handleRoleAssign(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	roleID, err := uuid.Parse(r.PathValue("roleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role"))
		return
	}
	var params struct {
		UserID string `json:"user_id"`
	}
	if !bindJSON(w, r, locale, &params) {
		return
	}
	if params.UserID == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "user_id"))
		return
	}
	if err := h.roleStore.AssignRoleToMember(r.Context(), tid, params.UserID, roleID); err != nil {
		slog.Error("role.assign", "error", err, "tenant_id", tid)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "role assignment", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *TenantAdminHandler) handleRoleRevoke(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.pathTenantID(w, r)
	if !ok {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	roleID, err := uuid.Parse(r.PathValue("roleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "role"))
		return
	}
	userID := r.PathValue("userID")
	if userID == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "user_id"))
		return
	}
	if err := h.roleStore.RevokeRoleFromMember(r.Context(), tid, userID, roleID); err != nil {
		slog.Error("role.revoke", "error", err, "tenant_id", tid)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "role assignment", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}