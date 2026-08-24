package store

import "database/sql"

// Stores is the top-level container for all storage backends.
type Stores struct {
	DB                    *sql.DB // underlying connection
	Sessions              SessionStore
	Memory                MemoryStore
	Cron                  CronStore
	Pairing               PairingStore
	Skills                SkillStore
	Agents                AgentStore
	Providers             ProviderStore
	Tracing               TracingStore
	RunTimeline           RunTimelineStore
	Runs                  RunsStore
	MCP                   MCPServerStore
	MCPOAuthTokens        MCPOAuthTokenStore
	ChannelInstances      ChannelInstanceStore
	ConfigSecrets         ConfigSecretsStore
	AgentLinks            AgentLinkStore
	Teams                 TeamStore
	BuiltinTools          BuiltinToolStore
	PendingMessages       PendingMessageStore
	ChannelMemory         ChannelMemoryExtractionStore
	KnowledgeGraph        KnowledgeGraphStore
	Contacts              ContactStore
	Activity              ActivityStore
	Snapshots             SnapshotStore
	UsageEvents           UsageEventStore
	BrowserCookies        BrowserCookieStore
	SecureCLI             SecureCLIStore
	SecureCLIGrants       SecureCLIAgentGrantStore
	APIKeys               APIKeyStore
	Heartbeats            HeartbeatStore
	ConfigPermissions     ConfigPermissionStore
	Tenants               TenantStore
	BuiltinToolTenantCfgs BuiltinToolTenantConfigStore
	SkillTenantCfgs       SkillTenantConfigStore
	SkillEvolution        SkillEvolutionStore
	SystemConfigs         SystemConfigStore
	SubagentTasks         SubagentTaskStore
	SubagentTaskRecovery  SubagentTaskRecoveryStore
	Vault                 VaultStore
	Episodic              EpisodicStore
	EvolutionMetrics      EvolutionMetricsStore
	EvolutionSuggestions  EvolutionSuggestionStore
	BitrixPortals         BitrixPortalStore
	// Hooks is hooks.HookStore — typed as any to avoid import cycle
	// (hooks package imports store for context helpers).
	// Callers: type-assert to hooks.HookStore before use.
	Hooks any

	// Contracts persists durable multi-agent collaboration records.
	Contracts ContractStore
	// Artifacts persists agent-produced artifacts with a version graph.
	Artifacts ArtifactStore
	// CheckpointSnapshots persists append-only checkpoint history so a paused
	// run can be replayed from any earlier snapshot.
	CheckpointSnapshots CheckpointSnapshotStore
	// Missions persists durable mission records (Mission Mode).
	Missions MissionStore
	// Approval persists command-execution approval requests so the queue
	// survives restarts and operators can audit resolved decisions.
	Approval ApprovalStore

	Webhooks     WebhookStore
	WebhookCalls WebhookCallStore

	// Workstations — Standard edition only (gated at router registration).
	Workstations           WorkstationStore
	WorkstationLinks       AgentWorkstationLinkStore
	WorkstationPermissions WorkstationPermissionStore
	WorkstationActivity    WorkstationActivityStore

	// UsageCaps persists budget-control policies, counters, and pricing.
	// Implemented for both PostgreSQL (pg) and SQLite (sqlitestore), so the
	// desktop/Lite edition enforces budgets too.
	UsageCaps UsageCapStore

	// TenantPolicies persists one per-tenant policy row (resource caps,
	// provider/model allowlists, suspension status).
	TenantPolicies TenantPolicyStore
	// TenantRoles persists per-tenant custom roles + role_permissions for
	// fine-grained RBAC (Phase 4). Builtin roles stay legacy tier anchors.
	TenantRoles TenantRoleStore
	// PublisherKeys manages ed25519 trust anchors for signed skill packages
	// (Phase 3 W2). A published skill's manifest signature must verify against
	// an active key here or the skill is rejected on install/import.
	PublisherKeys PublisherKeystore

	// NodeLeases persists device connectivity leases (Paseo plan Phase 1),
	// deliberately separate from auth and agent sessions: a WebSocket close
	// marks the lease reconnecting, never a logout.
	NodeLeases NodeLeaseStore

	// Workspaces persists first-class workspace objects (Paseo plan Phase 2):
	// named sandboxed root directories with optional git binding, scoped to
	// owner + tenant. Runtime path resolution stays in internal/workspace.
	Workspaces WorkspaceStore
}
