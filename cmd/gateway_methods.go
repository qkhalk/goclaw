package cmd

import (
	"context"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/audio"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/gateway/methods"
	"github.com/nextlevelbuilder/goclaw/internal/memory"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	usagecaps "github.com/nextlevelbuilder/goclaw/internal/usage/caps"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func registerAllMethods(server *gateway.Server, agents *agent.Router, sessStore store.SessionStore, tracingStore store.TracingStore, runTimeline store.RunTimelineStore, runsStore store.RunsStore, cronStore store.CronStore, pairingStore store.PairingStore, cfg *config.Config, cfgPath, workspace, dataDir string, msgBus *bus.MessageBus, execApprovalMgr *tools.ExecApprovalManager, approvalStore store.ApprovalStore, agentStore store.AgentStore, skillStore store.SkillStore, configSecretsStore store.ConfigSecretsStore, teamStore store.TeamStore, agentLinkStore store.AgentLinkStore, contextFileInterceptor *tools.ContextFileInterceptor, logTee *gateway.LogTee, heartbeatStore store.HeartbeatStore, configPermStore store.ConfigPermissionStore, sysConfigStore store.SystemConfigStore, tenantStore store.TenantStore, skillTenantCfgStore store.SkillTenantConfigStore, audioMgr *audio.Manager, usageCapSvc *usagecaps.Service, providerReg *providers.Registry, providerStore store.ProviderStore, teamWorkEmbedder memory.EmbeddingProvider, contractStore store.ContractStore, checkpointSnapshots store.CheckpointSnapshotStore, missionStore store.MissionStore, tenantPolicyStore store.TenantPolicyStore, tenantRoleStore store.TenantRoleStore, nodeLeaseStore store.NodeLeaseStore, workspaceStore store.WorkspaceStore, agentJobStore store.AgentJobStore, taskGraphStore store.TaskGraphStore, memoryFabricStore store.MemoryFabricStore, terminals store.TerminalStore, routingRulesStore store.RoutingRulesStore) (*methods.PairingMethods, *methods.HeartbeatMethods, *methods.ChatMethods, *methods.ConfigPermissionsMethods) {
	router := server.Router()

	// Phase 1: Core methods
	chatMethods := methods.NewChatMethods(agents, sessStore, cfg, server.RateLimiter(), msgBus)
	chatMethods.SetProviderOverrideResolver(providerStore, providerReg)
	chatMethods.SetAudioManager(audioMgr) // Wire TTS auto-apply for WS responses
	chatMethods.SetUsageCapService(usageCapSvc)
	chatMethods.SetTeamWorkClassification(agentStore, teamStore, agentLinkStore, teamWorkEmbedder)
	if tenantPolicyStore != nil {
		chatMethods.SetTenantPolicies(tenantPolicyStore)
	}
	chatMethods.Register(router)
	agentsMethods := methods.NewAgentsMethods(agents, cfg, cfgPath, workspace, agentStore, contextFileInterceptor, msgBus)
	if tenantPolicyStore != nil {
		agentsMethods.SetTenantPolicies(tenantPolicyStore)
	}
	agentsMethods.Register(router)
	methods.NewSessionsMethods(sessStore, msgBus, cfg).Register(router)
	runMethods := methods.NewRunTimelineMethods(runTimeline, cfg)
	runMethods.SetRunsStore(runsStore)
	// Wire the resume entrypoint so runs.resume drives the owning agent's
	// Loop.ResumeRun. Nil when the store/router is absent — the handler then
	// reports unavailable, keeping the surface safe before wiring.
	runMethods.SetResumer(makeRunResumer(agents, runsStore))
	runMethods.Register(router)
	// Intentional suspend/wake (hibernation): runs.pause writes a checkpoint +
	// transitions to paused; runs.wake reuses the same resume closure above.
	// Both closures come nil when the store/router is absent — the handlers
	// then report unavailable, mirroring runs.resume's nil-safety.
	hibMethods := methods.NewHibernateMethods(cfg)
	hibMethods.SetRunsStore(runsStore)
	suspend, wake := makeRunSuspendResumer(agents, runsStore)
	hibMethods.SetSuspendFn(suspend)
	hibMethods.SetResumer(wake)
	hibMethods.Register(router)
	// Node device leases (Paseo plan Phase 1): node.hello/heartbeat/bye.
	// Nil-safe: without a lease store the handlers report unavailable.
	if nodeLeaseStore != nil {
		methods.NewNodeMethods(nodeLeaseStore).Register(router)
	}
	// Workspace registry (Paseo plan Phase 2): workspace.* CRUD over the
	// sandboxed workspace domain. Nil-safe without a store; `workspace` (the
	// gateway's configured workspace root) bounds relative rootPath values.
	if workspaceStore != nil {
		methods.NewWorkspaceMethods(workspaceStore, workspace).Register(router)
	}
	// Workspace file explorer (Paseo plan Phase 3 / §24): lazy directory
	// listing and bounded reads/writes inside a workspace root. Shares the
	// workspace store's nil-safety.
	if workspaceStore != nil {
		methods.NewWorkspaceFilesMethods(workspaceStore).Register(router)
	}
	// Agent jobs + task graph (Paseo plan Phase 2): jobs.list/get/cancel and
	// tasks.tree/create/updateStatus. Nil-safe without their stores.
	if agentJobStore != nil {
		methods.NewJobsMethods(agentJobStore).Register(router)
	}
	if taskGraphStore != nil {
		methods.NewTasksMethods(taskGraphStore).Register(router)
	}
	// Memory fabric (Paseo plan Phase 5): memory.write/get/search/
	// supersede/archive over scoped semantic memory. Nil-safe.
	if memoryFabricStore != nil {
		methods.NewMemoryFabricMethods(memoryFabricStore).Register(router)
	}
	// Web terminal (Paseo plan Phase 4 / §25): PTY-per-tab streaming
	// surface. Nil-safe without stores/event publisher.
	if terminals != nil {
		methods.NewTerminalMethods(terminals, workspaceStore, server.EventPublisher()).Register(router)
	}
	configMethods := methods.NewConfigMethods(cfg, cfgPath, configSecretsStore, msgBus)
	if sysConfigStore != nil {
		configMethods.SetSystemConfigSync(func(ctx context.Context, c *config.Config) {
			// Only sync config for the current tenant (from request context)
			seedConfigForContext(ctx, sysConfigStore, c, false) // onlyMissing=false → upsert
			// Trigger readback via bus event with fresh context (request ctx may be canceled)
			if msgBus != nil {
				freshCtx := store.WithTenantID(context.Background(), store.TenantIDFromContext(ctx))
				msgBus.Broadcast(bus.Event{Name: bus.TopicSystemConfigChanged, Payload: freshCtx})
			}
		})
	}
	configMethods.Register(router)

	// Phase 2: Skills (uses SkillStore interface — PG or File)
	methods.NewSkillsMethods(skillStore, skillTenantCfgStore).Register(router)

	// Phase 2: Cron (store created externally, shared with gateway)
	methods.NewCronMethods(cronStore, msgBus, cfg).Register(router)

	// Phase 2: Heartbeat
	heartbeatMethods := methods.NewHeartbeatMethods(heartbeatStore, msgBus)
	// Wire cache-aware resolver so heartbeat can accept agent_key or UUID
	// without a DB roundtrip on the hot path when the agent is router-cached.
	heartbeatMethods.SetAgentRouter(agents)
	heartbeatMethods.Register(router)

	// Phase 2: Config permissions
	cfgPerms := methods.NewConfigPermissionsMethods(configPermStore, agentStore)
	cfgPerms.SetAgentRouter(agents)
	cfgPerms.Register(router)

	// Phase 2: Pairing (store created externally, shared with channel manager).
	// OnApprove callback is set later by the caller after channel manager is created.
	pairingMethods := methods.NewPairingMethods(pairingStore, msgBus, server.RateLimiter())
	pairingMethods.Register(router)

	// Phase 2: Usage (queries SessionStore for real token data)
	methods.NewUsageMethods(sessStore, tracingStore).Register(router)
	llmMethods := methods.NewLLMMethods(providerReg, cfg.Gateway.BackgroundProvider, cfg.Gateway.BackgroundModel)
	if tenantPolicyStore != nil {
		llmMethods.SetTenantPolicies(tenantPolicyStore)
	}
	llmMethods.Register(router)
	// Wire the same provider registry into the CRUD MCP server (see
	// internal/mcp/crud_server.go, mounted at /api/mcp/ in BuildMux()).
	server.SetLLMProviders(providerReg, cfg.Gateway.BackgroundProvider, cfg.Gateway.BackgroundModel)

	// Phase 2: Exec approval (always registered — returns empty when manager is nil)
	execApprovalMethods := methods.NewExecApprovalMethods(execApprovalMgr, msgBus)
	if approvalStore != nil {
		execApprovalMethods.SetApprovalStore(approvalStore)
	}
	execApprovalMethods.Register(router)

	// Phase 2: Send (outbound message routing)
	methods.NewSendMethods(msgBus).Register(router)

	// Phase 3: Live log tailing
	methods.NewLogsMethods(logTee).Register(router)

	// Multi-agent: dynamic team formation routing + jury/negotiation history.
	methods.NewMultiAgentMethods(contractStore, msgBus).Register(router)

	// Time travel: checkpoint-snapshot history + replay rewind. Nil-safe when the
	// snapshot store is absent — the handlers report unavailable.
	travelMethods := methods.NewTimeTravelMethods(checkpointSnapshots, cfg)
	travelMethods.SetRunsStore(runsStore)
	travelMethods.SetReplay(makeRunReplayer(agents, runsStore, checkpointSnapshots))
	travelMethods.Register(router)

	// Phase 4: tenant policies + RBAC custom roles (tenant-admin gated).
	if tenantPolicyStore != nil && tenantRoleStore != nil && tenantStore != nil && msgBus != nil {
		methods.NewTenantAdminMethods(tenantPolicyStore, tenantRoleStore, tenantStore, msgBus).Register(router)
	}

	// Phase 4 (inheritance plan): routing.rules.list/set/delete —
	// tenant-admin gated DB routing rules. Nil-safe: without a store the
	// surface is not registered.
	if routingRulesStore != nil {
		methods.NewRoutingRulesMethods(routingRulesStore, agentStore).Register(router)
	}

	// Mission Mode: durable mission records + resume. Nil-safe when the store is
	// absent — the handlers report unavailable. The resume closure resolves the
	// mission's run through RunsStore and drives the owning agent's Loop.ResumeRun
	// (the same durable resume path as runs.resume), so a resumed mission picks
	// up from its latest checkpoint instead of starting fresh.
	missionMethods := methods.NewMissionMethods(missionStore)
	missionMethods.SetResumer(makeMissionResumer(agents, missionStore, runsStore))
	missionMethods.Register(router)

	slog.Info("registered all RPC methods",
		"phase1", []string{"chat", "agents", "sessions", "config"},
		"phase2", []string{"skills", "cron", "heartbeat", "pairing", "usage", "llm", "exec_approval", "send"},
		"multiagent", []string{protocol.MethodMultiAgentFormation, protocol.MethodMultiAgentJury, protocol.MethodMultiAgentNegotiate},
	)

	return pairingMethods, heartbeatMethods, chatMethods, cfgPerms
}
