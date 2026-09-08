package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Memory scope values: the visibility ring a record lives in (plan §7.1).
// global rows are readable by everyone; narrower scopes gate on the
// user/agent/workspace/session identity of the caller.
const (
	MemoryScopeGlobal    = "global"
	MemoryScopeUser      = "user"
	MemoryScopeAgent     = "agent"
	MemoryScopeWorkspace = "workspace"
	MemoryScopeProject   = "project"
	MemoryScopeSession   = "session"
	MemoryScopeThread    = "thread"
)

// Memory kind values: what kind of semantic content the record carries.
const (
	MemoryKindFact                = "fact"
	MemoryKindPreference          = "preference"
	MemoryKindDecision            = "decision"
	MemoryKindInstruction         = "instruction"
	MemoryKindConstraint          = "constraint"
	MemoryKindProjectContext      = "project_context"
	MemoryKindTaskState           = "task_state"
	MemoryKindConversationSummary = "conversation_summary"
	MemoryKindObservation         = "observation"
)

// Memory status values: lifecycle of a record. Superseded/archived rows stay
// for lineage/audit but drop out of retrieval.
const (
	MemoryStatusActive     = "active"
	MemoryStatusSuperseded = "superseded"
	MemoryStatusArchived   = "archived"
)

// ValidMemoryScope reports whether s is a known memory scope.
func ValidMemoryScope(s string) bool {
	switch s {
	case MemoryScopeGlobal, MemoryScopeUser, MemoryScopeAgent,
		MemoryScopeWorkspace, MemoryScopeProject, MemoryScopeSession,
		MemoryScopeThread:
		return true
	}
	return false
}

// ValidMemoryKind reports whether s is a known memory kind.
func ValidMemoryKind(s string) bool {
	switch s {
	case MemoryKindFact, MemoryKindPreference, MemoryKindDecision,
		MemoryKindInstruction, MemoryKindConstraint, MemoryKindProjectContext,
		MemoryKindTaskState, MemoryKindConversationSummary, MemoryKindObservation:
		return true
	}
	return false
}

// ValidMemoryStatus reports whether s is a known memory status.
func ValidMemoryStatus(s string) bool {
	switch s {
	case MemoryStatusActive, MemoryStatusSuperseded, MemoryStatusArchived:
		return true
	}
	return false
}

// HashMemoryContent returns the sha256 hex digest of the normalized
// (whitespace-trimmed) content — the dedup key stored in memories.content_hash.
func HashMemoryContent(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}

// Memory is one atomic semantic memory record in the fabric (Paseo plan Phase
// 5). It is deliberately distinct from the file-based memory_documents/chunks
// and episodic_summaries stores: each row is a scoped fact with provenance and
// lifecycle metadata so retrieval can rank by authority/confidence/recency and
// supersession is tracked explicitly instead of overwriting history.
//
// TenantID == nil means master/global scope (same convention as api_keys,
// node_leases, workspaces).
type Memory struct {
	ID               string
	TenantID         *string // nil = master/global
	UserID           *string // owner subject (external-auth identity)
	AgentID          *string // owning agent key/uuid string
	WorkspaceID      *string // nullable FK workspaces(id)
	SessionKey       *string
	Scope            string // global|user|agent|workspace|project|session|thread
	Kind             string // fact|preference|decision|instruction|constraint|project_context|task_state|conversation_summary|observation
	Content          string
	SourceType       string  // session|manual|consolidation|import
	SourceRef        *string // e.g. "session:sess_xyz"
	Confidence       float64 // 0..1, default 0.8
	Authority        float64 // 0..1, default 0.5
	Status           string  // active|superseded|archived, default active
	SupersedesID     *string // memory this replaces
	ContradictsID    *string // memory it conflicts with
	ContentHash      *string // sha256 hex of normalized content
	EmbeddingVersion *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// MemoryWriteOpts carries optional write-behavior flags. Kept minimal on
// purpose; SupersedeTarget is reserved for future explicit-supersede writes.
type MemoryWriteOpts struct {
	SupersedeTarget bool
}

// MemoryQuery is the retrieval request for SearchMemories. The non-empty
// identity fields act as hard gates (see MemoryFabricStore.SearchMemories);
// Scopes/Kinds are optional whitelist filters.
type MemoryQuery struct {
	UserID      string
	AgentID     string
	WorkspaceID string
	SessionKey  string
	Scopes      []string // optional whitelist filter
	Kinds       []string // optional filter
	Limit       int
}

// ScoredMemory pairs a retrieved record with its store-side ranking score
// (authority*0.10 + confidence*0.10 + recency*0.10; semantic/lexical weights
// are applied by callers that have embeddings).
type ScoredMemory struct {
	*Memory
	Score float64
}

// memoryRecencyDecaySeconds is the linear-decay horizon of the recency term
// in the recall score: a memory not updated for 30 days scores 0 recency.
// Mirrors the SQL ORDER BY term in SearchMemories (PG EXTRACT and SQLite
// julianday variants) — change both together.
const memoryRecencyDecaySeconds = 2592000.0

// FilterContradictedMemories drops rows that an active contradiction in the
// result set supersedes: if row B carries ContradictsID == A.ID, A loses
// regardless of its score (B is the newer observation; retrieval already
// ranks B higher for equal authority). Rows contradicting something outside
// the result set are unaffected - their counterpart may be archived or
// simply not recalled. Runs after SearchMemories in both store
// implementations, so callers never see a contradicted-and-replaced fact.
func FilterContradictedMemories(results []ScoredMemory) []ScoredMemory {
	if len(results) < 2 {
		return results
	}
	contradicted := make(map[string]bool)
	for _, r := range results {
		if r.ContradictsID != nil && *r.ContradictsID != "" {
			contradicted[*r.ContradictsID] = true
		}
	}
	if len(contradicted) == 0 {
		return results
	}
	out := results[:0:0]
	for _, r := range results {
		if !contradicted[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// MemoryRecallScore computes the recall ranking score in Go. The SQL ORDER BY
// in SearchMemories uses the identical formula for ordering; computing the
// returned Score here (instead of selecting it as an extra column) keeps the
// shared 20-column memory scan path intact.
func MemoryRecallScore(m *Memory, now time.Time) float64 {
	recency := 1 - now.Sub(m.UpdatedAt).Seconds()/memoryRecencyDecaySeconds
	if recency < 0 {
		recency = 0
	}
	return m.Authority*0.10 + m.Confidence*0.10 + recency*0.10
}

// MemoryFabricStore persists semantic memory records. Implementations must
// scope reads and writes to the tenant from context where the row carries one;
// nil TenantID rows are master/global and only reachable from master scope.
type MemoryFabricStore interface {
	// WriteMemory inserts or upserts one memory. The ID, timestamps, status
	// and content hash are defaulted by the implementation when zero-valued.
	// Deduplication upserts by content hash within the same
	// (tenant,user,agent,workspace,scope) tuple: same hash updates content and
	// updated_at in place (m.ID is rewritten to the existing row id);
	// different content supersedes the existing near-key active row atomically
	// (new row gets supersedes_id, old row flips to superseded status).
	WriteMemory(ctx context.Context, m *Memory) error
	// GetMemory resolves a memory by canonical id. Returns sql.ErrNoRows when
	// the memory does not exist or is not visible from the caller's tenant
	// scope.
	GetMemory(ctx context.Context, id string) (*Memory, error)
	// SearchMemories retrieves active memories visible to the caller ranked by
	// authority/confidence/recency. Hard gates (always applied as WHERE
	// predicates):
	//   - status='active'
	//   - tenant: ctx tenant match, unless IsMasterScope (sees all)
	//   - q.UserID non-empty:      (user_id = ? OR scope='global')
	//   - q.AgentID non-empty:     (agent_id = ? OR scope IN ('global','user'))
	//   - q.WorkspaceID non-empty: (workspace_id = ? OR scope NOT IN
	//     ('workspace','project','session','thread'))
	// Scopes/Kinds whitelist additionally when non-empty; Limit defaults to 50.
	SearchMemories(ctx context.Context, q MemoryQuery) ([]ScoredMemory, error)
	// SupersedeMemory marks oldID superseded and inserts replacement pointing
	// back at it (replacement.SupersedesID = oldID) in one transaction.
	// Returns sql.ErrNoRows when the target does not exist, is not visible
	// from the caller's scope, or is already superseded.
	SupersedeMemory(ctx context.Context, oldID string, replacement *Memory) error
	// ArchiveMemory flips an active/superseded memory to archived. Returns
	// sql.ErrNoRows when nothing matched the caller's scope or the row is
	// already archived (idempotent-archive is NOT silent).
	ArchiveMemory(ctx context.Context, id string) error
}
