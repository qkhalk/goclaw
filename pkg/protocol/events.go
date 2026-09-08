package protocol

// WebSocket event names pushed from server to client.
const (
	EventAgent             = "agent"
	EventChat              = "chat"
	EventHealth            = "health"
	EventCron              = "cron"
	EventHeartbeat         = "heartbeat"
	EventExecApprovalReq   = "exec.approval.requested"
	EventExecApprovalRes   = "exec.approval.resolved"
	EventPresence          = "presence"
	EventTick              = "tick"
	EventShutdown          = "shutdown"
	EventNodePairRequested = "node.pair.requested"
	EventNodePairResolved  = "node.pair.resolved"
	EventDevicePairReq     = "device.pair.requested"
	EventDevicePairRes     = "device.pair.resolved"
	EventVoicewakeChanged  = "voicewake.changed"
	EventConnectChallenge  = "connect.challenge"
	EventTalkMode          = "talk.mode"

	// Agent summoning events (predefined agent setup via LLM).
	EventAgentSummoning = "agent.summoning"

	// Team activity events (real-time team workflow visibility).
	EventTeamTaskCreated     = "team.task.created"
	EventTeamTaskCompleted   = "team.task.completed"
	EventTeamMessageSent     = "team.message.sent"
	EventDelegationStarted   = "delegation.started"
	EventDelegationCompleted = "delegation.completed"

	// Delegation lifecycle events.
	EventDelegationFailed      = "delegation.failed"
	EventDelegationCancelled   = "delegation.cancelled"
	EventDelegationProgress    = "delegation.progress"
	EventDelegationAccumulated = "delegation.accumulated"
	EventDelegationAnnounce    = "delegation.announce"

	// Team task lifecycle events.
	EventTeamTaskClaimed         = "team.task.claimed"
	EventTeamTaskCancelled       = "team.task.cancelled"
	EventTeamTaskFailed          = "team.task.failed"
	EventTeamTaskReviewed        = "team.task.reviewed"
	EventTeamTaskApproved        = "team.task.approved"
	EventTeamTaskRejected        = "team.task.rejected"
	EventTeamTaskProgress        = "team.task.progress"
	EventTeamTaskCommented       = "team.task.commented"
	EventTeamTaskAssigned        = "team.task.assigned"
	EventTeamTaskDispatched      = "team.task.dispatched"
	EventTeamTaskUpdated         = "team.task.updated"
	EventTeamTaskDeleted         = "team.task.deleted"
	EventTeamTaskStale           = "team.task.stale"
	EventTeamTaskAttachmentAdded = "team.task.attachment_added"

	// Emitted when leader starts processing completed team task results (before announce run).
	EventTeamLeaderProcessing = "team.leader.processing"

	// Team CRUD events (admin operations).
	EventTeamCreated       = "team.created"
	EventTeamUpdated       = "team.updated"
	EventTeamDeleted       = "team.deleted"
	EventTeamMemberAdded   = "team.member.added"
	EventTeamMemberRemoved = "team.member.removed"

	// Workspace events (team file changes).
	EventWorkspaceFileChanged = "workspace.file.changed"

	// Agent link events (admin operations).
	EventAgentLinkCreated = "agent_link.created"
	EventAgentLinkUpdated = "agent_link.updated"
	EventAgentLinkDeleted = "agent_link.deleted"

	// Trace lifecycle events (realtime trace/span updates).
	EventTraceUpdated = "trace.updated"
	// Immediate status change event (not flush-buffered; fired on every status write).
	EventTraceStatusChanged = "trace.status"

	// Skill dependency check events (realtime progress during startup/rescan).
	EventSkillDepsChecked  = "skill.deps.checked"
	EventSkillDepsComplete = "skill.deps.complete"

	// Skill dependency install events (triggered by POST /v1/skills/install-deps).
	EventSkillDepsInstalling = "skill.deps.installing"
	EventSkillDepsInstalled  = "skill.deps.installed"

	// Per-item install events (triggered by POST /v1/skills/install-dep).
	EventSkillDepItemInstalling = "skill.dep.item.installing" // payload: {dep: "pip:openpyxl"}
	EventSkillDepItemInstalled  = "skill.dep.item.installed"  // payload: {dep, ok: bool, error?: string}

	// Cache invalidation events (internal, not forwarded to WS clients).
	EventCacheInvalidate = "cache.invalidate"

	// Audit log event (internal, not forwarded to WS clients).
	EventAuditLog = "audit.log"

	// Session lifecycle events.
	EventSessionUpdated = "session.updated"

	// Zalo Personal QR login events (client-scoped, not broadcast).
	EventZaloPersonalQRCode = "zalo.personal.qr.code"
	EventZaloPersonalQRDone = "zalo.personal.qr.done"

	// WhatsApp QR login events (client-scoped, not broadcast).
	EventWhatsAppQRCode = "whatsapp.qr.code"
	EventWhatsAppQRDone = "whatsapp.qr.done"

	// Tenant access revocation — forces affected user's UI to logout.
	EventTenantAccessRevoked = "tenant.access.revoked"

	// Vault enrichment pipeline progress.
	EventVaultEnrichProgress = "vault.enrich.progress"

	// Background worker alerts (non-retryable LLM errors).
	EventBackgroundError = "background.error"

	// Workstation exec streaming events.
	// EventWorkstationExecChunk is emitted for each stdout/stderr chunk during remote exec.
	// Payload: WorkstationExecChunkPayload.
	EventWorkstationExecChunk = "workstation.exec.chunk"
	// EventWorkstationExecDone is emitted when a remote exec command finishes.
	// Payload: WorkstationExecDonePayload.
	EventWorkstationExecDone = "workstation.exec.done"

	// MCP OAuth flow completion — fired after callback token exchange succeeds or fails.
	// Payload: MCPOAuthCompletePayload.
	EventMCPOAuthComplete = "mcp.oauth_complete"

	// Terminal output stream (Paseo plan Phase 4 / §25). Payload: map with
	// terminalId, userId, and data (base64 UTF-8 bytes chunk).
	EventTerminalOutput = "terminal.output"
	// EventTerminalExit is emitted when the shell process exits.
	// Payload: { terminalId, userId, exitCode }.
	EventTerminalExit = "terminal.exit"

	// Multi-agent collaboration events (jury verdicts, negotiation state,
	// formation routing). Payloads: MultiAgentEventPayload variants in
	// pkg/protocol/team_events.go.
	EventMultiAgentVerdict           = "multiagent.verdict"
	EventMultiAgentNegotiationState  = "multiagent.negotiation_state"
	EventMultiAgentFormationSelected = "multiagent.formation_selected"
)

// Agent event subtypes (in payload.type)
const (
	AgentEventRunStarted   = "run.started"
	AgentEventRunCompleted = "run.completed"
	AgentEventRunFailed    = "run.failed"
	AgentEventRunCancelled = "run.cancelled"
	// AgentEventRunPaused/AgentEventRunWoken are emitted by the intentional
	// hibernation path (runs.pause / runs.wake). They are distinct agent-stream
	// subtypes of the "agent" WS event; their typed payloads live in
	// pkg/protocol/run_events.go.
	AgentEventRunPaused = "run.paused"
	AgentEventRunWoken  = "run.woken"

	AgentEventRunRetrying  = "run.retrying"
	AgentEventToolCall     = "tool.call"
	AgentEventToolResult   = "tool.result"
	AgentEventToolStarted  = "tool.started"
	AgentEventToolProgress = "tool.progress"
	AgentEventToolLog      = "tool.log"
	AgentEventToolComplete = "tool.completed"
	AgentEventBlockReply   = "block.reply"
	AgentEventActivity     = "activity" // agent phase transitions: thinking, tool_exec, compacting

	// Completion-verifier verdict events (terminal gate in internal/agent
	// loop_run.go). Emitted as agent-stream subtypes next to the run.* events:
	// verification.passed when the verifier accepts the finished run;
	// verification.failed with a localized reason payload when it rejects it
	// (hard mode, or recover mode after its one continuation still fails).
	AgentEventVerificationPassed = "verification.passed"
	AgentEventVerificationFailed = "verification.failed"

	// AgentEventCheckpointCreated is emitted by the run-record updater each
	// time a durable pipeline checkpoint persists to agent_runs (checkpoint
	// cadence, not the pause write — a pause carries its own run.paused
	// event). Payload: iteration, status. Persisted to the run timeline so
	// replay clients can render checkpoint markers.
	AgentEventCheckpointCreated = "checkpoint.created"

	// AgentEventLLMStarted / AgentEventLLMCompleted bracket one think-stage
	// LLM call (including internal guard retries: the pair closes with the
	// total duration). Payloads: started — provider, model, iteration;
	// completed — duration_ms, input_tokens, output_tokens, is_error.
	AgentEventLLMStarted   = "llm.started"
	AgentEventLLMCompleted = "llm.completed"
)

// block.reply payload source values.
const (
	BlockReplySourceLLMProgress      = "llm_progress"
	BlockReplySourceToolAnnouncement = "tool_announcement"
)

// Chat event subtypes (in payload.type)
const (
	ChatEventChunk    = "chunk"
	ChatEventMessage  = "message"
	ChatEventThinking = "thinking"
)
