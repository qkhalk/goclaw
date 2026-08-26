package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ToolsInvokeHandler handles POST /v1/tools/invoke (direct tool invocation).
type ToolsInvokeHandler struct {
	registry     *tools.Registry
	agentStore   store.AgentStore  // nil if not configured
	maxBodyBytes int64             // request body cap; 0 => DefaultInvokeMaxBodyBytes
	rateLimiter  func(string) bool // nil = no limit
}

// SetRateLimiter injects the gateway limiter (per IP / bearer token) so the
// invoke endpoint throttles like chat completions.
func (h *ToolsInvokeHandler) SetRateLimiter(fn func(string) bool) { h.rateLimiter = fn }

// DefaultInvokeMaxBodyBytes caps the /v1/tools/invoke request body. Matches
// the 1 MiB convention of sibling JSON endpoints; override via
// tools.invoke_max_body_bytes when a tool legitimately consumes larger docs.
const DefaultInvokeMaxBodyBytes int64 = 1 << 20

// SetMaxBodyBytes overrides the request body cap (bytes). Non-positive values
// are ignored so a misconfigured gateway cannot disable the DoS guard.
func (h *ToolsInvokeHandler) SetMaxBodyBytes(n int64) {
	if n > 0 {
		h.maxBodyBytes = n
	}
}

// NewToolsInvokeHandler creates a handler for the tools invoke endpoint.
func NewToolsInvokeHandler(registry *tools.Registry, agentStore store.AgentStore) *ToolsInvokeHandler {
	return &ToolsInvokeHandler{
		registry:   registry,
		agentStore: agentStore,
	}
}

type toolsInvokeRequest struct {
	Tool       string         `json:"tool"`
	Action     string         `json:"action,omitempty"`
	Args       map[string]any `json:"args"`
	SessionKey string         `json:"sessionKey,omitempty"`
	AgentID    string         `json:"agentId,omitempty"`
	DryRun     bool           `json:"dryRun,omitempty"`
	Channel    string         `json:"channel,omitempty"`  // tool context: channel name
	ChatID     string         `json:"chatId,omitempty"`   // tool context: chat ID
	PeerKind   string         `json:"peerKind,omitempty"` // tool context: "direct" or "group"
}

func (h *ToolsInvokeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": i18n.T(locale, i18n.MsgMethodNotAllowed)})
		return
	}

	auth := resolveAuth(r)
	if !auth.Authenticated {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": i18n.T(locale, i18n.MsgUnauthorized)})
		return
	}
	if !permissions.HasMinRole(auth.Role, permissions.RoleOperator) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": i18n.T(locale, i18n.MsgPermissionDenied, r.URL.Path)})
		return
	}

	// Rate limit check (per IP or bearer token), same policy as chat completions.
	if h.rateLimiter != nil {
		key := r.RemoteAddr
		if token := extractBearerToken(r); token != "" {
			key = "token:" + token
		}
		if !h.rateLimiter(key) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": i18n.T(locale, i18n.MsgRateLimitExceeded)})
			return
		}
	}

	// Cap request body BEFORE decode: this endpoint reaches the tool registry
	// (including exec-class tools) — an unbounded decode is a DoS vector.
	cap := h.maxBodyBytes
	if cap <= 0 {
		cap = DefaultInvokeMaxBodyBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, cap)

	var req toolsInvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// MaxBytesReader trips produce "request body too large" — surface as
		// 413 rather than the generic invalid-request 400.
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, err.Error()))
		return
	}

	if req.Tool == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgRequired, "tool")})
		return
	}

	slog.Info("tools invoke request", "tool", req.Tool, "dry_run", req.DryRun)

	if req.DryRun {
		// Just check if tool exists and return its schema
		tool, ok := h.registry.Get(req.Tool)
		if !ok {
			writeToolError(w, http.StatusNotFound, "NOT_FOUND", i18n.T(locale, i18n.MsgNotFound, "tool", req.Tool))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"tool":        req.Tool,
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
			"dryRun":      true,
		})
		return
	}

	// Inject agentID into context for interceptors (bootstrap, memory).
	// Note: userID, tenantID, role, locale already injected by enrichContext above.
	ctx := r.Context()

	agentIDStr := req.AgentID
	if agentIDStr == "" {
		agentIDStr = extractAgentID(r, "")
	}
	if agentIDStr != "" && h.agentStore != nil {
		ag, err := h.agentStore.GetByKey(ctx, agentIDStr)
		if err == nil {
			ctx = store.WithAgentID(ctx, ag.ID)
		}
	}

	// Inject tool context keys (channel, chatID, peerKind) for message routing.
	if req.Channel != "" {
		ctx = tools.WithToolChannel(ctx, req.Channel)
	}
	if req.ChatID != "" {
		ctx = tools.WithToolChatID(ctx, req.ChatID)
	}
	if req.PeerKind != "" {
		ctx = tools.WithToolPeerKind(ctx, req.PeerKind)
	}

	// Execute the tool
	args := req.Args
	if args == nil {
		args = make(map[string]any)
	}

	// If action is specified, add it to args
	if req.Action != "" {
		args["action"] = req.Action
	}

	result := h.registry.ExecuteWithContext(ctx, req.Tool, args, "http", "api", "direct", "", nil)

	if result.IsError {
		writeToolError(w, http.StatusBadRequest, "TOOL_ERROR", result.ForLLM)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"result": map[string]any{
			"output":   result.ForLLM,
			"forUser":  result.ForUser,
			"metadata": map[string]any{},
		},
	})
}

func writeToolError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
