package tools

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// FileAction classifies a file/cloud tool operation for per-agent policy checks.
type FileAction string

const (
	FileActionRead   FileAction = "read"   // list/read/search existing files, cloud browse/fetch
	FileActionWrite  FileAction = "write"  // modify existing file content (edit, overwrite)
	FileActionCreate FileAction = "create" // create new files or folders
)

// AgentFilePolicyLookup resolves an agent key to its AgentData row.
// Implemented by store.AgentStore (pg/sqlitestore).
type AgentFilePolicyLookup interface {
	GetByKey(ctx context.Context, agentKey string) (*store.AgentData, error)
}

// FilePolicyAware is implemented by tools that need the precise per-agent
// file capability split at execute time (currently write_file, whose
// write-vs-create decision depends on target existence).
type FilePolicyAware interface {
	SetFilePolicyGuard(g *FilePolicyGuard)
}

// filePolicyCacheTTL bounds how long a resolved agent policy stays cached.
// Short TTL keeps admin toggles near-immediate without a per-tool-call DB hit.
const filePolicyCacheTTL = 30 * time.Second

type filePolicyCacheEntry struct {
	policy  config.FilePolicy
	known   bool // false = agent not found / lookup failed → fail-open
	expires time.Time
}

// FilePolicyGuard enforces the per-agent file/cloud capability toggles
// (agents.other_config.file_policy: read/write/create, default true) at tool
// execution time. The guard is wired once at gateway startup and shared by the
// registry (read tools + cloud tools + edit) and the write_file tool (which
// splits write vs create by target existence). A nil guard, an empty agent
// key, an unknown agent, or a lookup error all fail open — global/per-agent
// tool allow/deny policy continues to apply independently.
type FilePolicyGuard struct {
	lookup AgentFilePolicyLookup
	ttl    time.Duration

	mu    sync.Mutex
	cache map[string]filePolicyCacheEntry
}

// NewFilePolicyGuard builds a guard backed by the given agent lookup.
func NewFilePolicyGuard(lookup AgentFilePolicyLookup) *FilePolicyGuard {
	return &FilePolicyGuard{
		lookup: lookup,
		ttl:    filePolicyCacheTTL,
		cache:  make(map[string]filePolicyCacheEntry),
	}
}

// policyFor returns the agent's file policy; known=false when the calling
// agent has no resolvable policy row (fail-open). The cache is keyed by
// tenant + agent key: GetByKey is tenant-scoped, so the same agent_key can
// exist in different tenants with different policies.
func (g *FilePolicyGuard) policyFor(ctx context.Context, agentKey string) (config.FilePolicy, bool) {
	now := time.Now()
	cacheKey := store.TenantIDFromContext(ctx).String() + "\x00" + agentKey
	g.mu.Lock()
	entry, ok := g.cache[cacheKey]
	if ok && now.Before(entry.expires) {
		g.mu.Unlock()
		return entry.policy, entry.known
	}
	g.mu.Unlock()

	// Lookup outside the lock — may hit the DB.
	ag, err := g.lookup.GetByKey(ctx, agentKey)
	policy := config.FilePolicy{}
	known := err == nil && ag != nil
	if known {
		policy = ag.ParseFilePolicy()
	} else if err != nil {
		slog.Warn("security.file_policy.lookup_failed", "agent", agentKey, "error", err)
	}

	g.mu.Lock()
	g.cache[cacheKey] = filePolicyCacheEntry{policy: policy, known: known, expires: now.Add(g.ttl)}
	g.mu.Unlock()
	return policy, known
}

// Allow reports whether the calling agent may perform the given action.
func (g *FilePolicyGuard) Allow(ctx context.Context, action FileAction) bool {
	if g == nil || g.lookup == nil {
		return true
	}
	agentKey := ToolAgentKeyFromCtx(ctx)
	if agentKey == "" {
		return true
	}
	policy, known := g.policyFor(ctx, agentKey)
	if !known {
		return true
	}
	switch action {
	case FileActionRead:
		return policy.CanRead()
	case FileActionWrite:
		return policy.CanWrite()
	case FileActionCreate:
		return policy.CanCreate()
	}
	return true
}

// Check returns a localized deny error when the calling agent's policy
// blocks the given action, nil otherwise.
func (g *FilePolicyGuard) Check(ctx context.Context, action FileAction) error {
	if g.Allow(ctx, action) {
		return nil
	}
	slog.Warn("security.file_policy.denied",
		"agent", ToolAgentKeyFromCtx(ctx),
		"action", string(action))
	locale := store.LocaleFromContext(ctx)
	var key string
	switch action {
	case FileActionRead:
		key = i18n.MsgFileReadDenied
	case FileActionWrite:
		key = i18n.MsgFileWriteDenied
	case FileActionCreate:
		key = i18n.MsgFileCreateDenied
	default:
		key = i18n.MsgFileReadDenied
	}
	return fmt.Errorf("%s", i18n.T(locale, key))
}

// CheckAny denies only when the calling agent's policy blocks ALL listed
// actions. Used by write_file, whose write-vs-create split depends on whether
// the target file already exists: the tool is callable when either capability
// is enabled and the tool itself denies the specific action.
func (g *FilePolicyGuard) CheckAny(ctx context.Context, actions ...FileAction) error {
	for _, action := range actions {
		if g.Allow(ctx, action) {
			return nil
		}
	}
	if len(actions) == 0 {
		return nil
	}
	slog.Warn("security.file_policy.denied",
		"agent", ToolAgentKeyFromCtx(ctx),
		"action", "write|create")
	locale := store.LocaleFromContext(ctx)
	return fmt.Errorf("%s", i18n.T(locale, i18n.MsgFileWriteDenied))
}

// filePolicyAction maps a canonical tool name to the file capability it
// requires. write_file is intentionally absent — it needs the write/create
// split done by the tool itself (target existence), gated coarsely via
// CheckAny(write, create) in the registry.
//
// Cloud drive mutation tools are classified alongside their filesystem
// counterparts: overwrite/delete/rename/share mutate existing drive content
// (write), mkdir/copy create new drive entries (create). Shell/exec tools
// remain outside this policy by design (documented gap).
func filePolicyAction(name string) (FileAction, bool) {
	switch name {
	case "read_file", "list_files",
		"read_image", "read_audio", "read_video", "read_document",
		"cloud_ls", "cloud_read", "cloud_fetch", "cloud_about":
		return FileActionRead, true
	case "edit",
		"cloud_write", "cloud_delete", "cloud_move", "cloud_share":
		return FileActionWrite, true
	case "create_audio", "create_image", "create_video", "tts",
		"cloud_mkdir", "cloud_copy":
		// Media creators mkdir + os.WriteFile into the workspace — that is
		// unambiguously file creation. cloud_mkdir/cloud_copy likewise add
		// new entries on the drive.
		return FileActionCreate, true
	}
	return "", false
}

// checkFilePolicy is the registry-side enforcement hook. Returns an error
// result when the calling agent's file policy denies the tool.
func (r *Registry) checkFilePolicy(ctx context.Context, tool Tool) *Result {
	if r.fileGuard == nil {
		return nil
	}
	if name := tool.Name(); name == "write_file" {
		if err := r.fileGuard.CheckAny(ctx, FileActionWrite, FileActionCreate); err != nil {
			return ErrorResult(err.Error())
		}
		return nil
	}
	if action, ok := filePolicyAction(tool.Name()); ok {
		if err := r.fileGuard.Check(ctx, action); err != nil {
			return ErrorResult(err.Error())
		}
	}
	return nil
}
