package protocol

// RPC method name constants.
// Organized by priority: CRITICAL (Phase 1) → NEEDED (Phase 2) → NICE TO HAVE (Phase 3+).

// Phase 1 - CRITICAL methods
const (
	// Agent
	MethodAgent            = "agent"
	MethodAgentWait        = "agent.wait"
	MethodAgentIdentityGet = "agent.identity.get"

	// Chat
	MethodChatSend          = "chat.send"
	MethodChatHistory       = "chat.history"
	MethodChatAbort         = "chat.abort"
	MethodChatInject        = "chat.inject"
	MethodChatSessionStatus = "chat.session.status"

	// Agents management
	MethodAgentsList     = "agents.list"
	MethodAgentsCreate   = "agents.create"
	MethodAgentsUpdate   = "agents.update"
	MethodAgentsDelete   = "agents.delete"
	MethodAgentsFileList = "agents.files.list"
	MethodAgentsFileGet  = "agents.files.get"
	MethodAgentsFileSet  = "agents.files.set"

	// Config
	MethodConfigGet           = "config.get"
	MethodConfigApply         = "config.apply"
	MethodConfigPatch         = "config.patch"
	MethodConfigSchema        = "config.schema"
	MethodConfigDefaults      = "config.defaults"
	MethodChatBehaviorPreview = "chat_behavior.preview"

	// Sessions
	MethodSessionsList    = "sessions.list"
	MethodSessionsPreview = "sessions.preview"
	MethodSessionsPatch   = "sessions.patch"
	MethodSessionsDelete  = "sessions.delete"
	MethodSessionsReset   = "sessions.reset"
	MethodSessionsCompact = "sessions.compact"
	// sessions.branch clones a session's history up to a message index into a
	// new session key (fork). Mirrors POST /v1/chat/sessions/{key}/branch;
	// handler lives in internal/gateway/methods/sessions.go.
	MethodSessionsBranch = "sessions.branch"
	MethodRunTimelineGet = "run.timeline.get"

	// Durable run records (agent_runs state machine). Follow the naming
	// pattern of run.timeline.get; handlers live in
	// internal/gateway/methods/run_timeline.go.
	MethodRunsGet             = "runs.get"
	MethodRunsList            = "runs.list"
	MethodRunsEvents          = "runs.events"
	MethodRunsResume          = "runs.resume"
	MethodRunsCheckpointsList = "runs.checkpoints.list"
	MethodRunsReplay          = "runs.replay"

	// Intentional suspend/wake (hibernation): runs.pause writes the run's
	// latest durable checkpoint and transitions the record to paused;
	// runs.wake reuses the existing resume path. Handlers live in
	// internal/gateway/methods/hibernate.go.
	MethodRunsPause = "runs.pause"
	MethodRunsWake  = "runs.wake"

	// Node device leases (Paseo plan Phase 1): decouple connectivity from
	// authentication and agent sessions. WS close never invalidates a lease;
	// handlers live in internal/gateway/methods/node.go.
	MethodNodeHello     = "node.hello"
	MethodNodeHeartbeat = "node.heartbeat"
	MethodNodeBye       = "node.bye"

	// Workspace domain (Paseo plan Phase 2): first-class workspace objects
	// with canonical workspace_id, sandboxed root paths, and optional git
	// binding. Handlers live in internal/gateway/methods/workspace.go.
	MethodWorkspaceCreate = "workspace.create"
	MethodWorkspaceList   = "workspace.list"
	MethodWorkspaceGet    = "workspace.get"
	MethodWorkspaceUpdate = "workspace.update"
	MethodWorkspaceDelete = "workspace.delete"

	// Agent jobs (Paseo plan Phase 2 / §21): execution lifecycle separate
	// from sessions — only restart-surviving state is persisted; hot runtime
	// state stays in agent_runs. Handlers live in internal/gateway/methods/jobs.go.
	MethodJobsList   = "jobs.list"
	MethodJobsGet    = "jobs.get"
	MethodJobsCancel = "jobs.cancel"

	// Task graph (Paseo plan Phase 2 / §22): lightweight parent/child task
	// tree with dependencies per workspace. Handlers live in
	// internal/gateway/methods/tasks.go.
	MethodTasksTree         = "tasks.tree"
	MethodTasksCreate       = "tasks.create"
	MethodTasksUpdateStatus = "tasks.updateStatus"

	// Workspace file explorer (Paseo plan Phase 3 / §24): lazy directory
	// listing and file reads scoped to a workspace root. Handlers live in
	// internal/gateway/methods/workspace_files.go.
	MethodWorkspaceFilesList   = "workspace.files.list"
	MethodWorkspaceFilesRead   = "workspace.files.read"
	MethodWorkspaceFilesWrite  = "workspace.files.write"
	MethodWorkspaceFilesDelete = "workspace.files.delete"
	MethodWorkspaceFilesMkdir  = "workspace.files.mkdir"

	// Memory fabric (Paseo plan Phase 2 / §7.1): semantic memory records
	// with strict scope, provenance, confidence/authority, and a
	// supersede/conflict model. Handlers live in
	// internal/gateway/methods/memory_fabric.go.
	MethodMemoryWrite     = "memory.write"
	MethodMemoryGet       = "memory.get"
	MethodMemorySearch    = "memory.search"
	MethodMemorySupersede = "memory.supersede"
	MethodMemoryArchive   = "memory.archive"

	// Missions (Mission Mode): a durable data model for named objectives with
	// goals, milestones, and acceptance criteria. mission.create/get/list are
	// synchronous store reads/writes; mission.pause transitions to paused;
	// mission.resume re-drives the owning agent's run; mission.delete removes
	// the record. Handlers live in internal/gateway/methods/mission.go.
	MethodMissionCreate = "mission.create"
	MethodMissionGet    = "mission.get"
	MethodMissionList   = "mission.list"
	MethodMissionPause  = "mission.pause"
	MethodMissionResume = "mission.resume"
	MethodMissionDelete = "mission.delete"

	// System
	MethodConnect = "connect"
	MethodHealth  = "health"
	MethodStatus  = "status"
)

// Phase 2 - NEEDED methods
const (
	MethodSkillsList   = "skills.list"
	MethodSkillsGet    = "skills.get"
	MethodSkillsUpdate = "skills.update"

	// Skill review/curation (Phase 3 W1) — admin-classified.
	MethodSkillsApprove = "skills.approve"
	MethodSkillsReject  = "skills.reject"

	// Tenant policies (Phase 4 W1) — tenant-admin sorted.
	MethodTenantPoliciesGet    = "tenant.policies.get"
	MethodTenantPoliciesUpdate = "tenant.policies.update"

	// RBAC custom roles (Phase 4 W2) — tenant-admin sorted.
	MethodRolesList           = "role.list"
	MethodRolesGet            = "role.get"
	MethodRolesCreate         = "role.create"
	MethodRolesUpdate         = "role.update"
	MethodRolesDelete         = "role.delete"
	MethodRolePermissionsSet  = "role.permissions.set"
	MethodRolePermissionsList = "role.permissions.list"
	MethodRoleAssign          = "role.assign"
	MethodRoleRevoke          = "role.revoke"
	MethodRoleEffectiveGet    = "role.effective.get"

	MethodCronList   = "cron.list"
	MethodCronCreate = "cron.create"
	MethodCronUpdate = "cron.update"
	MethodCronDelete = "cron.delete"
	MethodCronToggle = "cron.toggle"
	MethodCronStatus = "cron.status"
	MethodCronRun    = "cron.run"
	MethodCronRuns   = "cron.runs"

	MethodChannelsList   = "channels.list"
	MethodChannelsStatus = "channels.status"
	MethodChannelsToggle = "channels.toggle"

	MethodPairingRequest = "device.pair.request"
	MethodPairingApprove = "device.pair.approve"
	MethodPairingDeny    = "device.pair.deny"
	MethodPairingList    = "device.pair.list"
	MethodPairingRevoke  = "device.pair.revoke"

	MethodBrowserPairingStatus = "browser.pairing.status"

	MethodApprovalsList    = "exec.approval.list"
	MethodApprovalsApprove = "exec.approval.approve"
	MethodApprovalsDeny    = "exec.approval.deny"
	MethodApprovalsHistory = "exec.approval.history"

	MethodUsageGet     = "usage.get"
	MethodUsageSummary = "usage.summary"

	MethodQuotaUsage = "quota.usage"

	MethodLLMComplete = "llm.complete"

	MethodSend = "send"
)

// Agent heartbeat
const (
	MethodHeartbeatGet          = "heartbeat.get"
	MethodHeartbeatSet          = "heartbeat.set"
	MethodHeartbeatToggle       = "heartbeat.toggle"
	MethodHeartbeatTest         = "heartbeat.test"
	MethodHeartbeatLogs         = "heartbeat.logs"
	MethodHeartbeatChecklistGet = "heartbeat.checklist.get"
	MethodHeartbeatChecklistSet = "heartbeat.checklist.set"
	MethodHeartbeatTargets      = "heartbeat.targets"
)

// Config permissions
const (
	MethodConfigPermissionsList   = "config.permissions.list"
	MethodConfigPermissionsCheck  = "config.permissions.check"
	MethodConfigPermissionsGrant  = "config.permissions.grant"
	MethodConfigPermissionsRevoke = "config.permissions.revoke"
)

// Channel instances management
const (
	MethodChannelInstancesList   = "channels.instances.list"
	MethodChannelInstancesGet    = "channels.instances.get"
	MethodChannelInstancesCreate = "channels.instances.create"
	MethodChannelInstancesUpdate = "channels.instances.update"
	MethodChannelInstancesDelete = "channels.instances.delete"
)

// Agent links (inter-agent delegation)
const (
	MethodAgentsLinksList   = "agents.links.list"
	MethodAgentsLinksCreate = "agents.links.create"
	MethodAgentsLinksUpdate = "agents.links.update"
	MethodAgentsLinksDelete = "agents.links.delete"
)

// Agent teams
const (
	MethodTeamsList                = "teams.list"
	MethodTeamsCreate              = "teams.create"
	MethodTeamsGet                 = "teams.get"
	MethodTeamsDelete              = "teams.delete"
	MethodTeamsTaskList            = "teams.tasks.list"
	MethodTeamsTaskGet             = "teams.tasks.get"
	MethodTeamsTaskGetLight        = "teams.tasks.get-light"
	MethodTeamsTaskApprove         = "teams.tasks.approve"
	MethodTeamsTaskReject          = "teams.tasks.reject"
	MethodTeamsTaskComment         = "teams.tasks.comment"
	MethodTeamsTaskComments        = "teams.tasks.comments"
	MethodTeamsTaskEvents          = "teams.tasks.events"
	MethodTeamsTaskCreate          = "teams.tasks.create"
	MethodTeamsTaskDelete          = "teams.tasks.delete"
	MethodTeamsTaskDeleteBulk      = "teams.tasks.delete-bulk"
	MethodTeamsTaskAssign          = "teams.tasks.assign"
	MethodTeamsTaskActiveBySession = "teams.tasks.active-by-session"
	MethodTeamsMembersAdd          = "teams.members.add"
	MethodTeamsMembersRemove       = "teams.members.remove"
	MethodTeamsUpdate              = "teams.update"
	MethodTeamsKnownUsers          = "teams.known_users"
	MethodTeamsScopes              = "teams.scopes"
)

// Team workspace
const (
	MethodTeamsWorkspaceList   = "teams.workspace.list"
	MethodTeamsWorkspaceRead   = "teams.workspace.read"
	MethodTeamsWorkspaceDelete = "teams.workspace.delete"
)

// Team events
const (
	MethodTeamsEventsList = "teams.events.list"
)

// Tenants (multi-tenant management)
const (
	MethodTenantsList        = "tenants.list"
	MethodTenantsGet         = "tenants.get"
	MethodTenantsCreate      = "tenants.create"
	MethodTenantsUpdate      = "tenants.update"
	MethodTenantsUsersList   = "tenants.users.list"
	MethodTenantsUsersAdd    = "tenants.users.add"
	MethodTenantsUsersRemove = "tenants.users.remove"
	MethodTenantsMine        = "tenants.mine"
)

// API key management
const (
	MethodAPIKeysList   = "api_keys.list"
	MethodAPIKeysCreate = "api_keys.create"
	MethodAPIKeysRevoke = "api_keys.revoke"
)

// Voices (ElevenLabs voice picker)
const (
	MethodVoicesList    = "voices.list"
	MethodVoicesRefresh = "voices.refresh"
)

// Phase 3+ - NICE TO HAVE methods
const (
	MethodLogsTail = "logs.tail"

	MethodTTSStatus      = "tts.status"
	MethodTTSEnable      = "tts.enable"
	MethodTTSDisable     = "tts.disable"
	MethodTTSConvert     = "tts.convert"
	MethodTTSSetProvider = "tts.setProvider"
	MethodTTSProviders   = "tts.providers"

	MethodBrowserAct        = "browser.act"
	MethodBrowserSnapshot   = "browser.snapshot"
	MethodBrowserScreenshot = "browser.screenshot"

	// Zalo Personal
	MethodZaloPersonalQRStart  = "zalo.personal.qr.start"
	MethodZaloPersonalContacts = "zalo.personal.contacts"

	// WhatsApp
	MethodWhatsAppQRStart = "whatsapp.qr.start"
)

// Workstations (Standard edition only — gated at router)
const (
	MethodWorkstationsList        = "workstations.list"
	MethodWorkstationsGet         = "workstations.get"
	MethodWorkstationsCreate      = "workstations.create"
	MethodWorkstationsUpdate      = "workstations.update"
	MethodWorkstationsDelete      = "workstations.delete"
	MethodWorkstationsTest        = "workstations.testConnection"
	MethodWorkstationsLinkAgent   = "workstations.linkAgent"
	MethodWorkstationsUnlinkAgent = "workstations.unlinkAgent"

	// Workstation permission allowlist CRUD (Phase 6)
	MethodWorkstationsPermList   = "workstations.permissions.list"
	MethodWorkstationsPermAdd    = "workstations.permissions.add"
	MethodWorkstationsPermRemove = "workstations.permissions.remove"
	MethodWorkstationsPermToggle = "workstations.permissions.toggle"

	// Workstation activity audit log (Phase 7)
	MethodWorkstationsListActivity = "workstations.activity.list"
)

// Agent hooks (Phase 3)
const (
	MethodHooksList    = "hooks.list"
	MethodHooksCreate  = "hooks.create"
	MethodHooksUpdate  = "hooks.update"
	MethodHooksDelete  = "hooks.delete"
	MethodHooksToggle  = "hooks.toggle"
	MethodHooksTest    = "hooks.test"
	MethodHooksHistory = "hooks.history"
)

// Bitrix24 portal management (self-service onboarding for the bitrix24 channel).
// See plans/260513-1648-bitrix24-portal-self-service-ux/phase-02-backend-rpc-portals.md.
const (
	MethodBitrixPortalsList          = "bitrix.portals.list"
	MethodBitrixPortalsCreate        = "bitrix.portals.create"
	MethodBitrixPortalsGetInstallURL = "bitrix.portals.get_install_url"
	MethodBitrixPortalsDelete        = "bitrix.portals.delete"
)

// Multi-agent (dynamic team formation, jury verdict history, negotiation
// history). Execution of jury/negotiate rounds happens through the
// jury/negotiate tools in the agent loop; these RPC methods expose formation
// routing and read-only persistence windows.
const (
	MethodMultiAgentFormation = "multiagent.formation"
	MethodMultiAgentJury      = "multiagent.jury"
	MethodMultiAgentNegotiate = "multiagent.negotiate"
)

// Web terminal (Paseo plan Phase 4 / §25): one PTY per terminal tab,
// streamed over WS events with a bounded in-memory ring buffer for replay.
// Raw output is never persisted; handlers live in
// internal/gateway/methods/terminal.go.
const (
	MethodTerminalCreate = "terminal.create"
	MethodTerminalList   = "terminal.list"
	MethodTerminalAttach = "terminal.attach"
	MethodTerminalInput  = "terminal.input"
	MethodTerminalResize = "terminal.resize"
	MethodTerminalClose  = "terminal.close"
)
