package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/hooks"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Compile-time guard: the approval manager implements the hook dispatcher's
// approval seam so cmd wiring can install it via hooks.SetApprovalGate.
var _ hooks.ApprovalGate = (*ExecApprovalManager)(nil)

// ClassifyTool maps a tool name to its approval class. Unknown tools return ""
// (no class → never gated by tool-class policies). The exec class is absent
// from gate decisions: ExecTool self-gates via ExecApprovalConfig so a
// tool-class policy would double-prompt.
func ClassifyTool(toolName string) ToolClass {
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch {
	case name == "":
		return ""
	case name == "exec" || name == "bash" || name == "sh" || strings.HasPrefix(name, "exec_"):
		return ToolClassExec
	case name == "workstation_exec":
		return ToolClassWorkstationExec
	case name == "browser" || strings.HasPrefix(name, "browser_"):
		return ToolClassBrowser
	case name == "write_file" || name == "edit_file" || name == "filesystem_write" || strings.HasPrefix(name, "write_"):
		return ToolClassWriteFile
	default:
		return ""
	}
}

// modeForClass returns the configured approval mode for a class. The exec
// class always reports off (self-gated inside ExecTool).
func (m *ExecApprovalManager) modeForClass(class ToolClass) ToolApprovalMode {
	if class == "" || class == ToolClassExec {
		return ToolModeOff
	}
	return m.config.ToolPolicies[class]
}

// GateToolCall implements hooks.ApprovalGate. It evaluates the per-tool-class
// policy for a pre-tool-use call AFTER the hook chain allowed it, blocking on
// human approval when the policy demands it. Exec-class calls return
// immediately — ExecTool performs its own CheckCommand/RequestApproval flow,
// and gating here too would prompt twice.
func (m *ExecApprovalManager) GateToolCall(ctx context.Context, q hooks.ApprovalQuery) (bool, string) {
	class := ClassifyTool(q.ToolName)
	mode := m.modeForClass(class)
	switch mode {
	case ToolModeDeny:
		slog.Warn("security.tool_policy_denied",
			"tool", q.ToolName,
			"class", string(class),
			"session", q.SessionKey,
		)
		return false, fmt.Sprintf("tool %q denied by tool-class policy (%s)", q.ToolName, class)
	case ToolModeAsk:
		return m.askForTool(ctx, q, class)
	default:
		return true, ""
	}
}

// ResolveAsk implements hooks.ApprovalGate. It handles a pre_tool_use hook's
// DecisionAsk: instead of degrading to block, it creates an approval request
// through the same engine (tool name + args digest + the hook's reason as
// risk hint) and waits. An explicit human approval unblocks the current wait
// and is recorded as an allow-once grant for (tool, argsDigest) so a
// retried/resumed identical call passes without a second prompt. Deny and
// deny-after-timeout block the call (v1 semantics — no pipeline pause).
func (m *ExecApprovalManager) ResolveAsk(ctx context.Context, q hooks.ApprovalQuery) (bool, string) {
	class := ClassifyTool(q.ToolName)
	if class == "" {
		// Unknown tool asking through a hook: keep the honest tool name as the
		// class label so the queue shows what is actually being approved.
		class = ToolClass(strings.ToLower(strings.TrimSpace(q.ToolName)))
	}
	return m.askForTool(ctx, q, class)
}

// askForTool consults existing grants and, on miss, creates the approval
// request and blocks for the decision.
func (m *ExecApprovalManager) askForTool(ctx context.Context, q hooks.ApprovalQuery, class ToolClass) (bool, string) {
	// Grant fast-paths (always > session > once). A consumed allow-once grant
	// is exactly the "approve now, retried call passes" record.
	if m.grants.checkAlways(class, q.ToolName) ||
		m.grants.checkSession(class, q.ToolName, q.SessionKey) ||
		m.grants.checkOnce(class, q.ToolName, q.ArgsDigest) {
		slog.Info("tool approval: grant hit, skipping prompt",
			"tool", q.ToolName, "class", string(class))
		return true, ""
	}

	risk := q.Risk
	if risk == "" {
		risk = "human approval requested"
	}
	timeout := m.askTimeout
	decision, err := m.RequestToolApproval(ctx, ToolApprovalRequest{
		Class:      class,
		ToolName:   q.ToolName,
		Preview:    toolCallPreview(q.ToolName, q.ToolInput),
		ArgsDigest: q.ArgsDigest,
		SessionKey: q.SessionKey,
		Risk:       risk,
		AgentID:    store.AgentIDFromContext(ctx).String(),
		Timeout:    timeout,
		Payload: json.RawMessage(`{"tool":` + marshalJSONString(q.ToolName) +
			`,"risk":` + marshalJSONString(risk) +
			`,"sessionKey":` + marshalJSONString(q.SessionKey) + `}`),
	})
	if err != nil {
		return false, fmt.Sprintf("tool approval for %q: %v", q.ToolName, err)
	}
	if decision == ApprovalDeny {
		slog.Warn("security.tool_approval_denied",
			"tool", q.ToolName,
			"class", string(class),
			"session", q.SessionKey,
		)
		return false, fmt.Sprintf("tool %q denied by user", q.ToolName)
	}
	return true, ""
}

// reinstatePending loads PENDING rows from the durable store and reinstates
// them as waiters-without-waiter: approval_requests is the source of truth for
// the queue, so a decision made after a gateway restart resolves the durable
// row (recording any grant) instead of erroring with ErrApprovalNotFound. Rows
// whose resolvability window has passed are transitioned to expired instead.
// Called in the background from SetApprovalStore; failures are logged only.
func (m *ExecApprovalManager) reinstatePending(s store.ApprovalStore) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.ListPendingAll(ctx)
	if err != nil {
		slog.Warn("approval engine: load pending failed (non-blocking)", "err", err)
		return
	}

	now := time.Now()
	reinstated := 0
	for _, row := range rows {
		if row.ExpiredAt != nil && !row.ExpiredAt.After(now) {
			m.expireStaleRow(s, row.ID)
			continue
		}
		pa := &PendingApproval{
			ID:          row.ID.String(),
			Command:     row.Command,
			AgentID:     agentIDString(row.AgentID),
			CreatedAt:   row.CreatedAt,
			TenantID:    row.TenantID,
			ToolClass:   ToolClass(row.ActionType),
			SessionKey:  row.SessionKey,
			ArgsDigest:  row.ArgsDigest,
			Risk:        riskFromPayload(row.Payload),
			ExpiresAt:   row.ExpiredAt,
			durableOnly: true,
		}
		if pa.ToolClass == "" {
			pa.ToolClass = ToolClassExec
		}

		m.mu.Lock()
		_, exists := m.pending[pa.ID]
		if !exists {
			m.pending[pa.ID] = pa
			m.durable[pa.ID] = row.ID
		}
		m.mu.Unlock()
		if !exists {
			reinstated++
		}
	}
	if reinstated > 0 {
		slog.Info("approval engine: reinstated pending requests from store", "count", reinstated)
	}
}

// expireStaleRow best-effort transitions a pending row past its resolvability
// window to the expired status so the durable queue stays clean.
func (m *ExecApprovalManager) expireStaleRow(s store.ApprovalStore, id uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.MarkExpired(ctx, id); err != nil {
		slog.Warn("approval engine: mark stale pending expired failed (non-blocking)", "id", id, "err", err)
	}
}

// agentIDString renders a nullable agent UUID as a plain string.
func agentIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// riskFromPayload extracts the risk hint recorded in a tool-class request
// payload. Legacy exec payloads carry only {"command": ...} and return "".
func riskFromPayload(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var doc struct {
		Risk string `json:"risk"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return ""
	}
	return doc.Risk
}
