package http

import (
	"log/slog"
	"net/http"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

type channelCapabilityDTO struct {
	Type               string   `json:"type"`
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	DisplayName        string   `json:"display_name,omitempty"`
	Enabled            bool     `json:"enabled"`
	Source             string   `json:"source"`
	ToolAllow          []string `json:"tool_allow,omitempty"`
	ToolDeny           []string `json:"tool_deny,omitempty"`
	CredentialSource   string   `json:"credential_source"`
	HasCredential      bool     `json:"has_credential"`
	ContextGrant       bool     `json:"context_grant_configured"`
	ContextCredentials bool     `json:"context_credentials_configured"`
}

func (h *ChannelInstancesHandler) handleListContextCapabilities(w http.ResponseWriter, r *http.Request) {
	inst, ok := h.resolveInstance(w, r)
	if !ok {
		return
	}
	scopeType := r.PathValue("scopeType")
	scopeKey := r.PathValue("scopeKey")
	if scopeType == "" || scopeKey == "" {
		locale := store.LocaleFromContext(r.Context())
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "scopeType and scopeKey"))
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = store.CredentialUserIDFromContext(r.Context())
	}

	mcpRows, err := h.listChannelMCPRows(r, inst, userID)
	if err != nil {
		slog.Error("channel_instances.capabilities_mcp", "error", err)
		locale := store.LocaleFromContext(r.Context())
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "MCP capabilities"))
		return
	}
	cliRows, err := h.listChannelCLIRows(r, inst, userID)
	if err != nil {
		slog.Error("channel_instances.capabilities_cli", "error", err)
		locale := store.LocaleFromContext(r.Context())
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "CLI capabilities"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"scope_type":   scopeType,
		"scope_key":    scopeKey,
		"capabilities": append(mcpRows, cliRows...),
		"mcp":          mcpRows,
		"secure_cli":   cliRows,
	})
}

func (h *ChannelInstancesHandler) listChannelMCPRows(r *http.Request, inst *store.ChannelInstanceData, userID string) ([]channelCapabilityDTO, error) {
	if h.mcpStore == nil {
		return []channelCapabilityDTO{}, nil
	}
	accessible, err := h.mcpStore.ListAccessible(r.Context(), inst.AgentID, userID)
	if err != nil {
		return nil, err
	}
	rows := make([]channelCapabilityDTO, 0, len(accessible))
	for _, info := range accessible {
		credentialSource := "global"
		hasCredential := info.Server.APIKey != "" || len(info.Server.Headers) > 0 || len(info.Server.Env) > 0
		if userID != "" {
			if creds, err := h.mcpStore.GetUserCredentials(r.Context(), info.Server.ID, userID); err == nil && creds != nil {
				if creds.APIKey != "" || len(creds.Headers) > 0 || len(creds.Env) > 0 {
					credentialSource = "user"
					hasCredential = true
				}
			}
		}
		rows = append(rows, channelCapabilityDTO{
			Type:               "mcp_server",
			ID:                 info.Server.ID.String(),
			Name:               info.Server.Name,
			DisplayName:        info.Server.DisplayName,
			Enabled:            info.Server.Enabled,
			Source:             "agent",
			ToolAllow:          info.ToolAllow,
			ToolDeny:           info.ToolDeny,
			CredentialSource:   credentialSource,
			HasCredential:      hasCredential,
			ContextGrant:       false,
			ContextCredentials: false,
		})
	}
	return rows, nil
}

func (h *ChannelInstancesHandler) listChannelCLIRows(r *http.Request, inst *store.ChannelInstanceData, userID string) ([]channelCapabilityDTO, error) {
	if h.secureCLIStore == nil {
		return []channelCapabilityDTO{}, nil
	}
	binaries, err := h.secureCLIStore.ListForAgent(r.Context(), inst.AgentID)
	if err != nil {
		return nil, err
	}
	rows := make([]channelCapabilityDTO, 0, len(binaries))
	for _, b := range binaries {
		source := "agent"
		if b.IsGlobal {
			source = "global"
		}
		credentialSource := source
		hasCredential := len(b.EncryptedEnv) > 0
		if userID != "" {
			if creds, err := h.secureCLIStore.GetUserCredentials(r.Context(), b.ID, userID); err == nil && creds != nil && len(creds.EncryptedEnv) > 0 {
				credentialSource = "user"
				hasCredential = true
			}
		}
		rows = append(rows, channelCapabilityDTO{
			Type:               "secure_cli",
			ID:                 b.ID.String(),
			Name:               b.BinaryName,
			DisplayName:        b.Description,
			Enabled:            b.Enabled,
			Source:             source,
			CredentialSource:   credentialSource,
			HasCredential:      hasCredential,
			ContextGrant:       false,
			ContextCredentials: false,
		})
	}
	return rows, nil
}
