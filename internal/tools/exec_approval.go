package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ExecSecurity determines the overall security mode for command execution.
type ExecSecurity string

const (
	// ExecSecurityDeny blocks all commands (no exec tool available).
	ExecSecurityDeny ExecSecurity = "deny"

	// ExecSecurityAllowlist only allows commands matching the allowlist.
	ExecSecurityAllowlist ExecSecurity = "allowlist"

	// ExecSecurityFull allows all commands (ask mode still applies).
	ExecSecurityFull ExecSecurity = "full"
)

// ExecAskMode determines when to prompt for user approval.
type ExecAskMode string

const (
	// ExecAskOff never asks — commands are auto-approved.
	ExecAskOff ExecAskMode = "off"

	// ExecAskOnMiss asks only when a command is not in the allowlist.
	ExecAskOnMiss ExecAskMode = "on-miss"

	// ExecAskAlways asks for every command execution.
	ExecAskAlways ExecAskMode = "always"
)

// ToolClass identifies a class of tools that share an approval policy. The
// class is derived from the tool name (ClassifyTool) and doubles as the
// action_type on persisted approval_requests rows.
type ToolClass string

const (
	// ToolClassExec covers shell/command execution tools ("exec"). This class
	// is self-gated inside ExecTool via ExecApprovalConfig; tool-class policies
	// do not double-gate it.
	ToolClassExec ToolClass = "exec"
	// ToolClassBrowser covers browser automation tools ("browser", "browser_*").
	ToolClassBrowser ToolClass = "browser"
	// ToolClassWorkstationExec covers remote workstation execution tools.
	ToolClassWorkstationExec ToolClass = "workstation_exec"
	// ToolClassWriteFile covers filesystem write/mutate tools.
	ToolClassWriteFile ToolClass = "write_file"
)

// ToolApprovalMode is the per-tool-class approval mode.
type ToolApprovalMode string

const (
	// ToolModeOff disables gating for the class (default — current behavior).
	ToolModeOff ToolApprovalMode = "off"
	// ToolModeAsk routes each call through the approval request flow.
	ToolModeAsk ToolApprovalMode = "ask"
	// ToolModeDeny blocks every call of the class.
	ToolModeDeny ToolApprovalMode = "deny"
)

// DefaultToolApprovalTimeout bounds how long a tool-class approval request
// waits for a human decision before deny-after-timeout kicks in.
const DefaultToolApprovalTimeout = 2 * time.Minute

// ExecApprovalConfig configures command execution approval.
type ExecApprovalConfig struct {
	Security  ExecSecurity `json:"security"`  // "deny", "allowlist", "full" (default "full")
	Ask       ExecAskMode  `json:"ask"`       // "off", "on-miss", "always" (default "off")
	Allowlist []string     `json:"allowlist"` // glob patterns for allowed commands
	// ToolPolicies maps tool classes ("browser", "workstation_exec",
	// "write_file", ...) to approval modes (off|ask|deny). Entries for "exec"
	// are ignored: that class is governed by Security/Ask inside ExecTool.
	ToolPolicies map[ToolClass]ToolApprovalMode `json:"toolPolicies,omitempty"`
}

// DefaultExecApprovalConfig returns the default (permissive) config.
func DefaultExecApprovalConfig() ExecApprovalConfig {
	return ExecApprovalConfig{
		Security: ExecSecurityFull,
		Ask:      ExecAskOff,
	}
}

// safeBins are command names that are always considered safe.
// Only includes read-only, text processing, and dev tools.
// Infrastructure/network tools (docker, kubectl, terraform, ansible,
// curl, wget, ssh, scp, rsync) are excluded — they require approval
// when ask mode is "on-miss".
var safeBins = map[string]bool{
	// Read-only / info tools
	"cat": true, "echo": true, "ls": true, "pwd": true, "head": true,
	"tail": true, "wc": true, "sort": true, "uniq": true, "grep": true,
	"find": true, "which": true, "whoami": true, "date": true,
	"uname": true, "hostname": true,
	"df": true, "du": true, "free": true, "uptime": true, "file": true,
	"stat": true, "dirname": true, "basename": true, "realpath": true,
	// Text processing
	"jq": true, "yq": true, "sed": true, "awk": true, "tr": true,
	"cut": true, "diff": true, "patch": true, "tee": true, "xargs": true,
	// Dev tools (core purpose of a coding agent)
	"git": true, "node": true, "npm": true, "npx": true, "yarn": true,
	"pnpm": true, "bun": true, "deno": true, "python": true, "python3": true,
	"pip": true, "pip3": true, "go": true, "cargo": true, "rustc": true,
	"make": true, "cmake": true, "gcc": true, "g++": true, "clang": true,
	"java": true, "javac": true, "mvn": true, "gradle": true,
}

// ApprovalDecision is the user's response to an approval request.
type ApprovalDecision string

const (
	ApprovalAllowOnce       ApprovalDecision = "allow-once"
	ApprovalAllowAlways     ApprovalDecision = "allow-always"
	ApprovalAllowForSession ApprovalDecision = "allow-for-session"
	ApprovalDeny            ApprovalDecision = "deny"
)

// PendingApproval is an in-flight approval request. Requests created by the
// legacy exec path carry Command = the shell command; tool-class requests
// carry Command = a human-readable preview of the tool call and the extra
// classification fields below.
type PendingApproval struct {
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	AgentID   string    `json:"agentId"`
	CreatedAt time.Time `json:"createdAt"`
	TenantID  uuid.UUID `json:"-"` // tenant scope; guards cross-tenant resolves
	// ToolClass is the approval class ("exec", "browser", ...). Always set;
	// defaults to exec for legacy requests.
	ToolClass ToolClass `json:"toolClass,omitempty"`
	// ToolName is the concrete tool the request gates ("" for legacy exec).
	ToolName string `json:"toolName,omitempty"`
	// Risk is a human-readable risk hint (e.g. the asking hook's reason).
	Risk string `json:"risk,omitempty"`
	// SessionKey scopes allow-for-session grants to the requesting session.
	SessionKey string `json:"sessionKey,omitempty"`
	// ArgsDigest is the canonical digest of (tool, args) used for allow-once
	// grants so a retried/resumed identical call passes without re-prompting.
	ArgsDigest string `json:"argsDigest,omitempty"`
	// ExpiresAt is when the request stops being resolvable (nil = no bound).
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	resultCh    chan ApprovalDecision
	durableOnly bool // reinstated from the store: no live waiter on the other side
	legacy      bool // created by the legacy exec RequestApproval path
}

// ExecApprovalManager manages pending approval requests and the dynamic allowlist.
// Persistence is best-effort: the manager keeps an in-memory fast path and
// mirrors every transition into the approval store when one is wired. A store
// write never blocks command execution — failures are logged and dropped.
//
// In-memory is the fast path; approval_requests is the source of truth for the
// queue. On SetApprovalStore the manager loads PENDING rows and reinstates
// them as waiters-without-waiter, so a decision made after a gateway restart
// resolves the durable row (and records any grant) instead of erroring.
type ExecApprovalManager struct {
	config        ExecApprovalConfig
	pending       map[string]*PendingApproval
	alwaysAllow   map[string]bool      // exec bins added via legacy allow-always decisions
	grants        *grantStore          // v2 grants: once/session/always with expiry
	durable       map[string]uuid.UUID // in-memory id → persisted row UUID
	mu            sync.Mutex
	nextID        int
	askTimeout    time.Duration       // test hook: overrides the tool-ask wait when > 0
	approvalStore store.ApprovalStore // optional; nil = in-memory only
	msgBus        bus.EventPublisher  // optional; nil = no push notifications
}

// NewExecApprovalManager creates an approval manager with the given config.
func NewExecApprovalManager(cfg ExecApprovalConfig) *ExecApprovalManager {
	return &ExecApprovalManager{
		config:      cfg,
		pending:     make(map[string]*PendingApproval),
		alwaysAllow: make(map[string]bool),
		grants:      newGrantStore(),
		durable:     make(map[string]uuid.UUID),
	}
}

// SetApprovalStore wires an optional durable store. Persist is best-effort:
// when the store is present, every request/resolve/timeout is mirrored in the
// background so the queue survives restarts; store errors never block exec.
// The store becomes the source of truth for the pending queue: existing
// PENDING rows are reinstated as waiters-without-waiter in the background.
func (m *ExecApprovalManager) SetApprovalStore(s store.ApprovalStore) {
	m.mu.Lock()
	m.approvalStore = s
	m.mu.Unlock()
	if s != nil {
		go m.reinstatePending(s)
	}
}

// SetEventBus wires an optional event publisher for push notifications.
func (m *ExecApprovalManager) SetEventBus(pub bus.EventPublisher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgBus = pub
}

// CheckCommand evaluates whether a command should be executed, blocked, or needs approval.
// Returns: "allow", "deny", or "ask".
func (m *ExecApprovalManager) CheckCommand(command string) string {
	switch m.config.Security {
	case ExecSecurityDeny:
		return "deny"

	case ExecSecurityAllowlist:
		if m.matchesAllowlist(command) {
			if m.config.Ask == ExecAskAlways {
				return "ask"
			}
			return "allow"
		}
		if m.config.Ask == ExecAskOff {
			return "deny" // not in allowlist, no asking
		}
		return "ask"

	case ExecSecurityFull:
		switch m.config.Ask {
		case ExecAskOff:
			return "allow"
		case ExecAskAlways:
			return "ask"
		case ExecAskOnMiss:
			if m.matchesAllowlist(command) || m.isSafeBin(command) {
				return "allow"
			}
			return "ask"
		}
	}

	return "allow"
}

// RequestApproval creates a pending approval and blocks until resolved or timeout.
// The context carries the tenant scope for persistence and event routing. The
// in-memory fast path stays authoritative for the block/wait; persistence is a
// best-effort background mirror with non-blocking failure handling.
func (m *ExecApprovalManager) RequestApproval(ctx context.Context, command, agentID string, timeout time.Duration) (ApprovalDecision, error) {
	return m.RequestToolApproval(ctx, ToolApprovalRequest{
		Class:      ToolClassExec,
		ToolName:   extractBin(command),
		Preview:    command,
		ArgsDigest: commandDigest(command),
		SessionKey: ToolSessionKeyFromCtx(ctx),
		AgentID:    agentID,
		Timeout:    timeout,
		Legacy:     true,
		Payload:    json.RawMessage(`{"command":` + marshalJSONString(command) + `}`),
	})
}

// ToolApprovalRequest describes a tool-class approval request.
type ToolApprovalRequest struct {
	// Class is the approval class (exec, browser, workstation_exec, ...).
	Class ToolClass
	// ToolName is the concrete tool the request gates ("" for legacy exec).
	ToolName string
	// Preview is the human-readable text shown to the approver (the shell
	// command for exec, a truncated args rendering for other tools).
	Preview string
	// ArgsDigest is the canonical digest of (tool, args) for grant matching.
	ArgsDigest string
	// SessionKey scopes allow-for-session grants.
	SessionKey string
	// Risk is a human-readable risk hint (e.g. the asking hook's reason).
	Risk string
	// AgentID identifies the requesting agent (UUID string when known).
	AgentID string
	// Timeout bounds the wait; <= 0 means DefaultToolApprovalTimeout.
	Timeout time.Duration
	// Payload is opaque JSON persisted with the row (exec: {"command":...}).
	Payload json.RawMessage
	// Legacy marks requests created by the legacy exec path, where an
	// allow-always decision also feeds the per-binary dynamic allowlist.
	Legacy bool
}

// RequestToolApproval creates a pending tool-class approval and blocks until
// resolved or timed out (deny-after-timeout). It is the single entry point
// shared by the legacy exec path and the tool-class/hook-ask gate.
func (m *ExecApprovalManager) RequestToolApproval(ctx context.Context, req ToolApprovalRequest) (ApprovalDecision, error) {
	if req.Class == "" {
		req.Class = ToolClassExec
	}
	if req.Timeout <= 0 {
		req.Timeout = DefaultToolApprovalTimeout
	}

	m.mu.Lock()
	m.nextID++
	id := fmt.Sprintf("%s-%d", req.Class, m.nextID)
	expiresAt := time.Now().Add(req.Timeout)
	pa := &PendingApproval{
		ID:         id,
		Command:    req.Preview,
		AgentID:    req.AgentID,
		CreatedAt:  time.Now(),
		TenantID:   store.TenantIDFromContext(ctx),
		ToolClass:  req.Class,
		ToolName:   req.ToolName,
		Risk:       req.Risk,
		SessionKey: req.SessionKey,
		ArgsDigest: req.ArgsDigest,
		ExpiresAt:  &expiresAt,
		resultCh:   make(chan ApprovalDecision, 1),
		legacy:     req.Legacy,
	}
	m.pending[id] = pa
	st := m.approvalStore
	pub := m.msgBus
	m.mu.Unlock()

	slog.Info("exec approval requested",
		"id", id,
		"class", string(req.Class),
		"command", truncateCmd(req.Preview, 100),
	)

	// Persist best-effort in a background goroutine: a DB hiccup must never
	// block command execution or leave the in-memory state half-updated.
	if st != nil {
		go func() {
			durableID, err := persistApprovalRequest(ctx, st, req)
			if err != nil {
				slog.Warn("exec approval: persist request failed (non-blocking)", "id", id, "err", err)
				return
			}
			m.mu.Lock()
			m.durable[id] = durableID
			m.mu.Unlock()
		}()
	}

	// Broadcast push notification to the tenant's clients.
	m.broadcastRequest(ctx, pub, pa)

	// Wait for resolution or timeout. The authoritative resolve/broadcast
	// happens in Resolve (the WS handler) — this goroutine only continues the
	// exec once a decision has been granted.
	select {
	case decision := <-pa.resultCh:
		m.mu.Lock()
		delete(m.pending, id)
		delete(m.durable, id)
		m.mu.Unlock()
		return decision, nil

	case <-time.After(req.Timeout):
		m.mu.Lock()
		delete(m.pending, id)
		durID := m.durable[id]
		delete(m.durable, id)
		m.mu.Unlock()
		go m.markExpiredBestEffort(ctx, st, id, durID, pub)
		return ApprovalDeny, ErrApprovalTimedOut
	}
}

// broadcastRequest pushes an exec.approval.requested event for pa.
func (m *ExecApprovalManager) broadcastRequest(ctx context.Context, pub bus.EventPublisher, pa *PendingApproval) {
	if pub == nil {
		return
	}
	tid := store.TenantIDFromContext(ctx)
	uid := store.UserIDFromContext(ctx)
	payload := map[string]any{
		"id":        pa.ID,
		"command":   truncateCmd(pa.Command, 100),
		"agentId":   pa.AgentID,
		"createdAt": pa.CreatedAt.UnixMilli(),
		"userId":    uid,
		"toolClass": string(pa.ToolClass),
	}
	if pa.Risk != "" {
		payload["risk"] = pa.Risk
	}
	if pa.SessionKey != "" {
		payload["sessionKey"] = pa.SessionKey
	}
	if pa.ExpiresAt != nil {
		payload["expiresAt"] = pa.ExpiresAt.UnixMilli()
	}
	pub.Broadcast(bus.Event{
		Name:     protocol.EventExecApprovalReq,
		TenantID: tid,
		Payload:  payload,
	})
}

// persistApprovalRequest writes a pending row. The tenant comes from the
// context; rows never leak across tenants. Returns the persisted row UUID so
// the caller can route future resolve/expire operations at it.
func persistApprovalRequest(ctx context.Context, st store.ApprovalStore, req ToolApprovalRequest) (uuid.UUID, error) {
	tid := store.TenantIDFromContext(ctx)
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultToolApprovalTimeout
	}
	durable := &store.ApprovalRequest{
		TenantID:       tid,
		ActionType:     string(req.Class),
		Payload:        req.Payload,
		Command:        truncateCmd(req.Preview, maxCommandLen),
		Status:         store.ApprovalStatusPending,
		SessionKey:     req.SessionKey,
		ArgsDigest:     req.ArgsDigest,
		TimeoutSeconds: int(timeout.Seconds()),
	}
	if durable.TimeoutSeconds <= 0 {
		durable.TimeoutSeconds = int(DefaultToolApprovalTimeout.Seconds())
	}
	// Record the deadline so operators auditing the queue see exactly how long a
	// request stays resolvable (mirrors the in-memory timeout in RequestToolApproval).
	expiresAt := time.Now().Add(time.Duration(durable.TimeoutSeconds) * time.Second)
	durable.ExpiredAt = &expiresAt
	if parsed, err := uuid.Parse(req.AgentID); err == nil {
		durable.AgentID = &parsed
	}
	durable.RequesterType = "agent"
	if err := st.CreateRequest(ctx, durable); err != nil {
		return uuid.Nil, err
	}
	return durable.ID, nil
}

// markExpiredBestEffort transitions the persisted row to expired and notifies
// clients that the request is closed.
func (m *ExecApprovalManager) markExpiredBestEffort(ctx context.Context, st store.ApprovalStore, id string, durableID uuid.UUID, pub bus.EventPublisher) {
	if st != nil && durableID != uuid.Nil {
		if err := st.MarkExpired(ctx, durableID); err != nil {
			slog.Warn("exec approval: mark expired failed (non-blocking)", "id", id, "err", err)
		}
	}
	if pub != nil {
		tid := store.TenantIDFromContext(ctx)
		pub.Broadcast(bus.Event{
			Name:     protocol.EventExecApprovalRes,
			TenantID: tid,
			Payload: map[string]any{
				"id":       id,
				"decision": "timeout",
				"status":   store.ApprovalStatusExpired,
				"userId":   store.UserIDFromContext(ctx),
				"tenantId": tid.String(),
			},
		})
	}
}

// ErrApprovalTimedOut is returned when an approval waits too long for a decision.
var ErrApprovalTimedOut = fmt.Errorf("approval timed out")

// ErrApprovalNotFound is returned when no in-flight approval matches the id.
var ErrApprovalNotFound = fmt.Errorf("approval not found or already resolved")

// Resolve resolves a pending approval request.
//
// ctx carries the tenant scope (WS handler context) so cross-tenant resolves
// are refused. The decidedBy actor is recorded on the persisted row. The
// in-memory decision is delivered to the blocked exec; when a durable row
// exists it is transitioned best-effort and clients get a push notification.
func (m *ExecApprovalManager) Resolve(ctx context.Context, id string, decision ApprovalDecision, decidedBy *uuid.UUID) error {
	return m.ResolveScope(ctx, id, decision, decidedBy, nil)
}

// ResolveScope resolves a pending approval request, recording the granted
// scope (allow-once / allow-always / allow-for-session) and its optional
// expiry both in-memory and on the durable row.
//
// Resolution is claimed atomically under the manager lock so a decision can
// only be delivered once — for live waiters via resultCh, for reinstated
// waiters-without-waiter by transitioning the durable row directly.
func (m *ExecApprovalManager) ResolveScope(ctx context.Context, id string, decision ApprovalDecision, decidedBy *uuid.UUID, grantExpiresAt *time.Time) error {
	m.mu.Lock()
	pa, ok := m.pending[id]
	if !ok {
		m.mu.Unlock()
		return ErrApprovalNotFound
	}
	// Tenant guard: the caller must share the requesting tenant.
	tid := store.TenantIDFromContext(ctx)
	if tid != uuid.Nil && pa.TenantID != uuid.Nil && tid != pa.TenantID {
		m.mu.Unlock()
		return fmt.Errorf("approval %q belongs to another tenant", id)
	}
	// Claim the resolution: remove the entry so a second resolve fails closed
	// and the timeout path cannot double-deliver.
	delete(m.pending, id)
	durID := m.durable[id]
	delete(m.durable, id)
	st := m.approvalStore
	pub := m.msgBus
	m.recordGrantLocked(pa, decision, grantExpiresAt)
	m.mu.Unlock()

	// Deliver the decision to the blocked exec (live waiters only).
	if !pa.durableOnly {
		pa.resultCh <- decision
	}

	// Persist best-effort (log + drop on error; never block the WS reply).
	if st != nil && durID != uuid.Nil {
		go func() {
			grant := grantForDecision(pa, decision, grantExpiresAt)
			if err := st.ResolveWithScope(ctx, durID, string(decision), decidedBy, grant); err != nil {
				slog.Warn("exec approval: persist resolve failed (non-blocking)", "id", id, "err", err)
			}
		}()
	}
	if pub != nil {
		tid := store.TenantIDFromContext(ctx)
		status := store.ApprovalStatusDenied
		if decision != ApprovalDeny {
			status = store.ApprovalStatusApproved
		}
		pub.Broadcast(bus.Event{
			Name:     protocol.EventExecApprovalRes,
			TenantID: tid,
			Payload: map[string]any{
				"id":       id,
				"decision": string(decision),
				"status":   status,
				"userId":   store.UserIDFromContext(ctx),
			},
		})
	}
	return nil
}

// recordGrantLocked records the in-memory grant implied by decision. Callers
// must hold m.mu.
func (m *ExecApprovalManager) recordGrantLocked(pa *PendingApproval, decision ApprovalDecision, expiresAt *time.Time) {
	switch decision {
	case ApprovalAllowAlways:
		if pa.ToolClass == ToolClassExec && pa.legacy {
			// Legacy dynamic allowlist: future commands sharing the binary
			// skip the ask via matchesAllowlist. Only genuine legacy exec
			// requests (Command = the raw shell command) update it — a hook
			// ask on the exec tool must not widen the allowlist to every
			// binary by extracting "exec" from the args preview.
			if bin := extractBin(pa.Command); bin != "" && bin != string(ToolClassExec) {
				m.alwaysAllow[bin] = true
				slog.Info("exec approval: added to always-allow", "bin", bin)
			}
		}
		m.grants.addAlways(pa.ToolClass, pa.ToolName, expiresAt)
	case ApprovalAllowForSession:
		m.grants.addSession(pa.ToolClass, pa.ToolName, pa.SessionKey, expiresAt)
	case ApprovalAllowOnce:
		m.grants.addOnce(pa.ToolClass, pa.ToolName, pa.ArgsDigest, expiresAt)
	case ApprovalDeny:
		// No grant.
	}
}

// grantForDecision maps a decision to the durable grant record. pa supplies
// the session key for allow-for-session rows.
func grantForDecision(pa *PendingApproval, decision ApprovalDecision, expiresAt *time.Time) store.ApprovalGrant {
	switch decision {
	case ApprovalAllowOnce:
		return store.ApprovalGrant{AllowOnce: true, ExpiresAt: expiresAt}
	case ApprovalAllowAlways:
		return store.ApprovalGrant{AllowAlways: true, ExpiresAt: expiresAt}
	case ApprovalAllowForSession:
		return store.ApprovalGrant{AllowForSession: true, SessionKey: pa.SessionKey, ExpiresAt: expiresAt}
	default:
		return store.ApprovalGrant{}
	}
}

// ListPending returns all pending approval requests, oldest first. The queue
// includes both live requests and reinstated waiters-without-waiter recovered
// from the durable store after a restart.
func (m *ExecApprovalManager) ListPending() []*PendingApproval {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*PendingApproval, 0, len(m.pending))
	for _, pa := range m.pending {
		result = append(result, pa)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// matchesAllowlist checks if a command matches any allowlist pattern or dynamic always-allow.
func (m *ExecApprovalManager) matchesAllowlist(command string) bool {
	bin := extractBin(command)

	// Check dynamic always-allow
	m.mu.Lock()
	if m.alwaysAllow[bin] {
		m.mu.Unlock()
		return true
	}
	m.mu.Unlock()

	// Check static allowlist patterns
	for _, pattern := range m.config.Allowlist {
		if matched, _ := filepath.Match(pattern, bin); matched {
			return true
		}
		// Also match against full command
		if matched, _ := filepath.Match(pattern, command); matched {
			return true
		}
	}

	return false
}

// isSafeBin checks if the command's base binary is in the safe list.
func (m *ExecApprovalManager) isSafeBin(command string) bool {
	return safeBins[extractBin(command)]
}

// extractBin returns the first word of a command (the binary name).
func extractBin(command string) string {
	command = strings.TrimSpace(command)
	// Skip env var assignments like FOO=bar cmd
	for strings.Contains(command, "=") {
		parts := strings.SplitN(command, " ", 2)
		if !strings.Contains(parts[0], "=") {
			break
		}
		if len(parts) < 2 {
			return ""
		}
		command = strings.TrimSpace(parts[1])
	}

	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

func truncateCmd(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// marshalJSONString returns the JSON string literal for s (with quotes and
// escaping), safe to embed in a JSON payload document without double-encoding.
func marshalJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
