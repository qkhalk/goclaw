package methods

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// NOTE(orchestrator): these method-name constants are declared locally for the
// paseo phase 5 wave cutover and will be relocated to pkg/protocol (same flow
// as jobs.go in phase 2).
const (
	MethodMemoryWrite     = "memory.write"
	MethodMemoryGet       = "memory.get"
	MethodMemorySearch    = "memory.search"
	MethodMemorySupersede = "memory.supersede"
	MethodMemoryArchive   = "memory.archive"
)

// Defaults applied when the caller omits the optional scoring parameters,
// mirroring the store.Memory field defaults (plan §7.1).
const (
	defaultMemoryConfidence = 0.8
	defaultMemoryAuthority  = 0.5
	defaultMemoryLimit      = 50
	maxMemoryLimit          = 200
)

// MemoryFabricMethods implements the memory.* WS surface over the semantic
// memory fabric store (plan §7/§9/§11): writing, reading, searching
// score-ordered memories plus supersede/archive lifecycle transitions.
type MemoryFabricMethods struct {
	memories store.MemoryFabricStore
}

func NewMemoryFabricMethods(memories store.MemoryFabricStore) *MemoryFabricMethods {
	return &MemoryFabricMethods{memories: memories}
}

// Register wires the memory.* methods into the method router.
func (m *MemoryFabricMethods) Register(router *gateway.MethodRouter) {
	router.Register(MethodMemoryWrite, m.handleWrite)
	router.Register(MethodMemoryGet, m.handleGet)
	router.Register(MethodMemorySearch, m.handleSearch)
	router.Register(MethodMemorySupersede, m.handleSupersede)
	router.Register(MethodMemoryArchive, m.handleArchive)
}

// memoryJSON is the camelCase wire form of store.Memory.
type memoryJSON struct {
	ID               string    `json:"id"`
	TenantID         *string   `json:"tenantId"`
	UserID           *string   `json:"userId"`
	AgentID          *string   `json:"agentId"`
	WorkspaceID      *string   `json:"workspaceId"`
	SessionKey       *string   `json:"sessionKey"`
	Scope            string    `json:"scope"`
	Kind             string    `json:"kind"`
	Content          string    `json:"content"`
	SourceType       string    `json:"sourceType"`
	SourceRef        *string   `json:"sourceRef"`
	Confidence       float64   `json:"confidence"`
	Authority        float64   `json:"authority"`
	Status           string    `json:"status"`
	SupersedesID     *string   `json:"supersedesId"`
	ContradictsID    *string   `json:"contradictsId"`
	ContentHash      *string   `json:"contentHash"`
	EmbeddingVersion *string   `json:"embeddingVersion"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func toMemoryJSON(m *store.Memory) memoryJSON {
	return memoryJSON{
		ID:               m.ID,
		TenantID:         m.TenantID,
		UserID:           m.UserID,
		AgentID:          m.AgentID,
		WorkspaceID:      m.WorkspaceID,
		SessionKey:       m.SessionKey,
		Scope:            m.Scope,
		Kind:             m.Kind,
		Content:          m.Content,
		SourceType:       m.SourceType,
		SourceRef:        m.SourceRef,
		Confidence:       m.Confidence,
		Authority:        m.Authority,
		Status:           m.Status,
		SupersedesID:     m.SupersedesID,
		ContradictsID:    m.ContradictsID,
		ContentHash:      m.ContentHash,
		EmbeddingVersion: m.EmbeddingVersion,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

// clamp01 constrains v to the closed [0,1] interval used by confidence and
// authority scoring inputs.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// memoryWriteParams is the shared parameter block of memory.write and the
// write-shaped portion of memory.supersede.
type memoryWriteParams struct {
	Content     string   `json:"content"`
	Scope       string   `json:"scope"`
	Kind        string   `json:"kind"`
	WorkspaceID string   `json:"workspaceId"`
	SessionKey  string   `json:"sessionId"`
	SourceRef   string   `json:"sourceRef"`
	Confidence  *float64 `json:"confidence"`
	Authority   *float64 `json:"authority"`
}

// buildMemory validates the shared write params and constructs the store
// record. On validation failure it sends the error response itself and
// returns nil; ok=false means the caller must stop. Tenant/user identity is
// stamped from the client (matching workspace.create); AgentID stays nil
// because the gateway cannot know which agent produced the memory yet.
func (m *MemoryFabricMethods) buildMemory(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, p *memoryWriteParams) (*store.Memory, bool) {
	locale := store.LocaleFromContext(ctx)
	scope := p.Scope
	if scope == "" {
		scope = "agent"
	}
	if !store.ValidMemoryScope(scope) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "scope must be one of global|user|agent|workspace|project|session|thread")))
		return nil, false
	}
	kind := p.Kind
	if kind == "" {
		kind = "fact"
	}
	if !store.ValidMemoryKind(kind) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "kind must be one of fact|preference|decision|instruction|constraint|project_context|task_state|conversation_summary|observation")))
		return nil, false
	}
	confidence := defaultMemoryConfidence
	if p.Confidence != nil {
		confidence = clamp01(*p.Confidence)
	}
	authority := defaultMemoryAuthority
	if p.Authority != nil {
		authority = clamp01(*p.Authority)
	}
	mem := &store.Memory{
		UserID:     strPtr(client.UserID()),
		Scope:      scope,
		Kind:       kind,
		Content:    p.Content,
		SourceType: "manual",
		Confidence: confidence,
		Authority:  authority,
		Status:     "active",
	}
	if tid := tenantString(client); tid != "" {
		mem.TenantID = &tid
	}
	if p.SessionKey != "" {
		sk := p.SessionKey
		mem.SessionKey = &sk
	}
	if p.WorkspaceID != "" {
		wid := p.WorkspaceID
		mem.WorkspaceID = &wid
	}
	if p.SourceRef != "" {
		sr := p.SourceRef
		mem.SourceRef = &sr
	}
	return mem, true
}

// strPtr returns a pointer to s, or nil when s is empty so nullable columns
// stay NULL instead of holding empty strings.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// handleWrite records a memory (memory.write). Requires operator role;
// viewers are forbidden. Params: { content required, scope?, kind?,
// sessionId?, sourceRef?, confidence?, authority? }. The store upserts by
// content hash within the same scope tuple (plan §7.1).
func (m *MemoryFabricMethods) handleWrite(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "memory.write requires operator role")))
		return
	}
	var params memoryWriteParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.Content == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "content")))
		return
	}
	if m.memories == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "memory fabric store not wired"))
		return
	}
	mem, ok := m.buildMemory(ctx, client, req, &params)
	if !ok {
		return
	}
	if err := m.memories.WriteMemory(ctx, mem); err != nil {
		slog.Warn("memory.write_failed", "scope", mem.Scope, "kind", mem.Kind, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "write memory")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"memory": toMemoryJSON(mem)}))
}

// memoryIDParams is the shared { memoryId } parameter block.
type memoryIDParams struct {
	MemoryID string `json:"memoryId"`
}

// parseMemoryID decodes and validates { memoryId }, sending the error
// response itself; ok=false means the caller must stop.
func parseMemoryID(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) (string, bool) {
	locale := store.LocaleFromContext(ctx)
	var params memoryIDParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return "", false
		}
	}
	if params.MemoryID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "memoryId")))
		return "", false
	}
	return params.MemoryID, true
}

// fetchMemory loads one memory by id; a missing row answers not-found while
// any other failure answers internal; ok=false means the caller must stop.
func (m *MemoryFabricMethods) fetchMemory(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, id string) (*store.Memory, bool) {
	locale := store.LocaleFromContext(ctx)
	mem, err := m.memories.GetMemory(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || mem == nil {
		slog.Debug("memory.get_not_found", "memory_id", id, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "memory", id)))
		return nil, false
	}
	if err != nil {
		slog.Warn("memory.get_failed", "memory_id", id, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "get memory")))
		return nil, false
	}
	return mem, true
}

// handleGet reads one memory (memory.get). Viewer-accessible.
// Params: { memoryId }.
func (m *MemoryFabricMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	if m.memories == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "memory fabric store not wired"))
		return
	}
	id, ok := parseMemoryID(ctx, client, req)
	if !ok {
		return
	}
	mem, ok := m.fetchMemory(ctx, client, req, id)
	if !ok {
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"memory": toMemoryJSON(mem)}))
}

// scoredMemoryJSON adds the retrieval score to the wire form; the embedded
// struct flattens so memory fields stay at the top level of each object.
type scoredMemoryJSON struct {
	memoryJSON
	Score float64 `json:"score"`
}

// handleSearch retrieves score-ordered active memories (memory.search).
// Viewer-accessible. Params: { query required, userId?, agentId?,
// workspaceId?, scopes?[], kinds?[], limit? }. Hard-gate scoping (tenant,
// user/agent/workspace visibility) lives in the store (plan §9); limit is
// clamped to 1..200 here.
func (m *MemoryFabricMethods) handleSearch(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	if m.memories == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "memory fabric store not wired"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		Query       string   `json:"query"`
		UserID      string   `json:"userId"`
		AgentID     string   `json:"agentId"`
		WorkspaceID string   `json:"workspaceId"`
		Scopes      []string `json:"scopes"`
		Kinds       []string `json:"kinds"`
		Limit       int      `json:"limit"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.Query == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "query")))
		return
	}
	for _, s := range params.Scopes {
		if !store.ValidMemoryScope(s) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "scopes contains an invalid scope (global|user|agent|workspace|project|session|thread)")))
			return
		}
	}
	for _, k := range params.Kinds {
		if !store.ValidMemoryKind(k) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "kinds contains an invalid kind (fact|preference|decision|instruction|constraint|project_context|task_state|conversation_summary|observation)")))
			return
		}
	}
	limit := params.Limit
	if limit < 1 {
		limit = defaultMemoryLimit
	}
	if limit > maxMemoryLimit {
		limit = maxMemoryLimit
	}
	q := store.MemoryQuery{
		UserID:      params.UserID,
		AgentID:     params.AgentID,
		WorkspaceID: params.WorkspaceID,
		Scopes:      params.Scopes,
		Kinds:       params.Kinds,
		Limit:       limit,
	}
	results, err := m.memories.SearchMemories(ctx, q)
	if err != nil {
		slog.Warn("memory.search_failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "search memories")))
		return
	}
	out := make([]scoredMemoryJSON, 0, len(results))
	for _, r := range results {
		out = append(out, scoredMemoryJSON{memoryJSON: toMemoryJSON(r.Memory), Score: r.Score})
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"memories": out}))
}

// handleSupersede replaces one memory atomically (memory.supersede). Requires
// operator role; viewers are forbidden. Params: { oldId required,
// content required, ...write fields }. A missing target answers not-found.
func (m *MemoryFabricMethods) handleSupersede(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "memory.supersede requires operator role")))
		return
	}
	if m.memories == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "memory fabric store not wired"))
		return
	}
	var params struct {
		memoryWriteParams
		OldID string `json:"oldId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.OldID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "oldId")))
		return
	}
	if params.Content == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "content")))
		return
	}
	replacement, ok := m.buildMemory(ctx, client, req, &params.memoryWriteParams)
	if !ok {
		return
	}
	if err := m.memories.SupersedeMemory(ctx, params.OldID, replacement); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "memory", params.OldID)))
			return
		}
		slog.Warn("memory.supersede_failed", "old_id", params.OldID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "supersede memory")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"memory": toMemoryJSON(replacement)}))
}

// handleArchive soft-deletes one memory (memory.archive). Requires operator
// role; viewers are forbidden. Params: { memoryId }. A missing row answers
// not-found.
func (m *MemoryFabricMethods) handleArchive(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "memory.archive requires operator role")))
		return
	}
	if m.memories == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "memory fabric store not wired"))
		return
	}
	id, ok := parseMemoryID(ctx, client, req)
	if !ok {
		return
	}
	if err := m.memories.ArchiveMemory(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "memory", id)))
			return
		}
		slog.Warn("memory.archive_failed", "memory_id", id, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "archive memory")))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"archived": true}))
}
