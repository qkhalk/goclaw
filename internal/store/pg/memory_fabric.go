package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGMemoryFabricStore implements store.MemoryFabricStore backed by
// PostgreSQL.
type PGMemoryFabricStore struct {
	db *sql.DB
}

func NewPGMemoryFabricStore(db *sql.DB) *PGMemoryFabricStore {
	return &PGMemoryFabricStore{db: db}
}

const memoryFabricColumns = `id, tenant_id, user_id, agent_id, workspace_id,
 session_key, scope, kind, content, source_type, source_ref, confidence,
 authority, status, supersedes_id, contradicts_id, content_hash,
 embedding_version, created_at, updated_at`

func scanMemoryFabric(row interface{ Scan(...any) error }) (*store.Memory, error) {
	var m store.Memory
	var userID, agentID, workspaceID, sessionKey, sourceRef,
		supersedesID, contradictsID, contentHash, embeddingVersion sql.NullString
	if err := row.Scan(&m.ID, &m.TenantID, &userID, &agentID, &workspaceID,
		&sessionKey, &m.Scope, &m.Kind, &m.Content, &m.SourceType, &sourceRef,
		&m.Confidence, &m.Authority, &m.Status, &supersedesID, &contradictsID,
		&contentHash, &embeddingVersion, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	m.UserID = nilStrPtr(userID)
	m.AgentID = nilStrPtr(agentID)
	m.WorkspaceID = nilStrPtr(workspaceID)
	m.SessionKey = nilStrPtr(sessionKey)
	m.SourceRef = nilStrPtr(sourceRef)
	m.SupersedesID = nilStrPtr(supersedesID)
	m.ContradictsID = nilStrPtr(contradictsID)
	m.ContentHash = nilStrPtr(contentHash)
	m.EmbeddingVersion = nilStrPtr(embeddingVersion)
	return &m, nil
}

// memoryTenantFilter appends the context-tenant condition to a WHERE clause
// being built with $N placeholders. Master scope (IsMasterScope) sees all
// rows; with a tenant present the filter is exact.
func memoryTenantFilter(ctx context.Context, where string, args []any) (string, []any) {
	if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
		args = append(args, store.TenantIDFromContext(ctx))
		where += fmt.Sprintf(" AND tenant_id = $%d", len(args))
	}
	return where, args
}

// memoryTupleMatch builds the nullable near-key equality predicates for the
// dedup/supersede lookup: each tuple column must equal the incoming memory's
// value or be NULL together with it. startParam is the next free placeholder
// number in the surrounding statement.
func memoryTupleMatch(startParam int, m *store.Memory) (string, []any) {
	tuple := []struct {
		col string
		val *string
	}{
		{"user_id", m.UserID},
		{"agent_id", m.AgentID},
		{"workspace_id", m.WorkspaceID},
	}
	clauses := make([]string, 0, len(tuple)+2)
	var args []any
	for _, t := range tuple {
		args = append(args, nilStr(derefStr(t.val)))
		clauses = append(clauses,
			fmt.Sprintf("%s IS NOT DISTINCT FROM $%d", t.col, startParam+len(args)-1))
	}
	args = append(args, m.Scope)
	clauses = append(clauses, fmt.Sprintf("scope = $%d", startParam+len(args)-1))
	return strings.Join(clauses, " AND "), args
}

// insertMemory inserts one row; tx may be the pool itself (*sql.DB) inside a
// transaction. Shared by WriteMemory and SupersedeMemory so both stamp the
// same defaults (timestamps are always implementation-stamped).
func insertMemory(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, tid uuid.UUID, m *store.Memory) error {
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	_, err := q.ExecContext(ctx,
		`INSERT INTO memories
		 (id, tenant_id, user_id, agent_id, workspace_id, session_key, scope,
		  kind, content, source_type, source_ref, confidence, authority,
		  status, supersedes_id, contradicts_id, content_hash,
		  embedding_version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		uuid.Must(uuid.Parse(m.ID)), tid,
		nilStr(derefStr(m.UserID)), nilStr(derefStr(m.AgentID)),
		parseUUIDOrNil(derefStr(m.WorkspaceID)), nilStr(derefStr(m.SessionKey)),
		m.Scope, m.Kind, m.Content, m.SourceType, nilStr(derefStr(m.SourceRef)),
		m.Confidence, m.Authority, m.Status,
		parseUUIDOrNil(derefStr(m.SupersedesID)), parseUUIDOrNil(derefStr(m.ContradictsID)),
		nilStr(derefStr(m.ContentHash)), nilStr(derefStr(m.EmbeddingVersion)),
		m.CreatedAt, m.UpdatedAt)
	return err
}

// WriteMemory upserts a memory within the same (tenant,user,agent,workspace,
// scope) tuple: identical content hash updates content/updated_at in place
// (m.ID is rewritten to the existing row id); different content supersedes
// the newest active near-key atomically — new row carries supersedes_id and
// the old row flips to 'superseded' in the same transaction. Global-scope
// rows never supersede (they are shared by definition).
func (s *PGMemoryFabricStore) WriteMemory(ctx context.Context, m *store.Memory) error {
	if strings.TrimSpace(m.ID) == "" {
		m.ID = store.GenNewID().String()
	}
	if m.Confidence == 0 {
		m.Confidence = 0.8
	}
	if m.Authority == 0 {
		m.Authority = 0.5
	}
	if !store.ValidMemoryStatus(m.Status) {
		m.Status = store.MemoryStatusActive
	}
	if derefStr(m.ContentHash) == "" {
		hash := store.HashMemoryContent(m.Content)
		m.ContentHash = &hash
	}
	tid := tenantIDForInsert(ctx)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory write begin tx: %w", err)
	}
	defer tx.Rollback()

	// Newest active near-key duplicate (same ownership tuple, non-global).
	dupWhere, dupArgs := memoryTupleMatch(1, m)
	kindParam := len(dupArgs) + 1
	if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
		dupArgs = append(dupArgs, store.TenantIDFromContext(ctx))
		dupWhere += fmt.Sprintf(" AND tenant_id = $%d", len(dupArgs))
	}
	dupQuery := `SELECT id, content_hash FROM memories
		WHERE status = 'active' AND scope <> 'global' AND content_hash IS NOT NULL AND (` +
		dupWhere + `) AND kind = $` + fmt.Sprint(kindParam) + `
		ORDER BY updated_at DESC LIMIT 1`
	dupArgs = append(dupArgs, m.Kind)

	var existingID, existingHash sql.NullString
	err = tx.QueryRowContext(ctx, dupQuery, dupArgs...).Scan(&existingID, &existingHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := insertMemory(ctx, tx, tid, m); err != nil {
			return fmt.Errorf("memory write insert: %w", err)
		}
	case err != nil:
		return fmt.Errorf("memory write dedup lookup: %w", err)
	case existingHash.Valid && existingHash.String == derefStr(m.ContentHash):
		// Identical content: refresh in place, keep the canonical id.
		now := time.Now()
		res, uerr := tx.ExecContext(ctx,
			`UPDATE memories SET content = $2, updated_at = $3 WHERE id = $1`,
			existingID, m.Content, now)
		if uerr != nil {
			return fmt.Errorf("memory write upsert update: %w", uerr)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("memory write upsert: %w", sql.ErrNoRows)
		}
		m.ID = existingID.String
		m.UpdatedAt = now
	default:
		// Same kind, different content: new row supersedes the old one.
		supersede := m.SupersedesID == nil || derefStr(m.SupersedesID) == ""
		if supersede {
			m.SupersedesID = &existingID.String
		}
		if err := insertMemory(ctx, tx, tid, m); err != nil {
			return fmt.Errorf("memory write supersede insert: %w", err)
		}
		if _, uerr := tx.ExecContext(ctx,
			`UPDATE memories SET status = 'superseded', updated_at = updated_at
			 WHERE id = $1`, existingID); uerr != nil {
			return fmt.Errorf("memory write supersede update: %w", uerr)
		}
	}
	return tx.Commit()
}

// GetMemory resolves one memory by id, scoped to the context tenant
// (master scope sees all). sql.ErrNoRows when not found / not visible.
func (s *PGMemoryFabricStore) GetMemory(ctx context.Context, id string) (*store.Memory, error) {
	mid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("memory id %q: %w", id, err)
	}
	where := "id = $1"
	args := []any{mid}
	where, args = memoryTenantFilter(ctx, where, args)
	row := s.db.QueryRowContext(ctx,
		`SELECT `+memoryFabricColumns+` FROM memories WHERE `+
			strings.TrimPrefix(where, " AND "), args...)
	return scanMemoryFabric(row)
}

// SearchMemories applies the retrieval hard gates (status='active' plus
// identity gates on the non-empty query fields), optional scope/kind
// whitelists, and ranks by authority/confidence/recency computed in SQL.
// LIMIT defaults to 50 when q.Limit is not positive.
func (s *PGMemoryFabricStore) SearchMemories(ctx context.Context, q store.MemoryQuery) ([]store.ScoredMemory, error) {
	where := "status = 'active'"
	var args []any

	// Tenant hard gate: master scope is unrestricted, everyone else is pinned
	// to their own tenant's rows.
	if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
		args = append(args, store.TenantIDFromContext(ctx))
		where += fmt.Sprintf(" AND tenant_id = $%d", len(args))
	}
	if q.UserID != "" {
		args = append(args, q.UserID)
		n := len(args)
		where += fmt.Sprintf(" AND (user_id = $%d OR scope = '%s')", n, store.MemoryScopeGlobal)
	}
	if q.AgentID != "" {
		args = append(args, q.AgentID)
		n := len(args)
		where += fmt.Sprintf(" AND (agent_id = $%d OR scope IN ('%s','%s'))",
			n, store.MemoryScopeGlobal, store.MemoryScopeUser)
	}
	if q.WorkspaceID != "" {
		wid, err := parseUUID(q.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("memory workspace_id %q: %w", q.WorkspaceID, err)
		}
		args = append(args, wid)
		n := len(args)
		where += fmt.Sprintf(` AND (workspace_id = $%d OR scope NOT IN ('%s','%s','%s','%s'))`,
			n, store.MemoryScopeWorkspace, store.MemoryScopeProject,
			store.MemoryScopeSession, store.MemoryScopeThread)
	}
	if q.SessionKey != "" {
		args = append(args, q.SessionKey)
		where += fmt.Sprintf(" AND session_key = $%d", len(args))
	}
	for _, sc := range q.Scopes {
		if !store.ValidMemoryScope(sc) {
			continue
		}
		args = append(args, sc)
		where += fmt.Sprintf(" AND scope = $%d", len(args))
	}
	for _, k := range q.Kinds {
		if !store.ValidMemoryKind(k) {
			continue
		}
		args = append(args, k)
		where += fmt.Sprintf(" AND kind = $%d", len(args))
	}
	limit := 50
	if q.Limit > 0 {
		limit = q.Limit
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+memoryFabricColumns+`,
		   authority*0.10 + confidence*0.10 +
		   GREATEST(0, 1 - EXTRACT(EPOCH FROM (now()-updated_at))/2592000.0) AS score
		 FROM memories WHERE `+where+`
		 ORDER BY score DESC, updated_at DESC
		 LIMIT `+fmt.Sprint(limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []store.ScoredMemory{}
	for rows.Next() {
		m, err := scanMemoryFabric(rows)
		if err != nil {
			return nil, err
		}
		var score float64
		if err := rows.Scan(&score); err != nil { //nolint:govet // score is the trailing column
			return nil, err
		}
		out = append(out, store.ScoredMemory{Memory: m, Score: score})
	}
	return out, rows.Err()
}

// SupersedeMemory marks oldID superseded and inserts replacement pointing back
// at it in one transaction. The target must exist, be visible from the
// caller's tenant scope and still be active — otherwise sql.ErrNoRows.
func (s *PGMemoryFabricStore) SupersedeMemory(ctx context.Context, oldID string, replacement *store.Memory) error {
	oid, err := parseUUID(oldID)
	if err != nil {
		return fmt.Errorf("memory id %q: %w", oldID, err)
	}
	replacement.SupersedesID = &oldID
	if replacement.ID == "" {
		replacement.ID = store.GenNewID().String()
	}
	if replacement.Confidence == 0 {
		replacement.Confidence = 0.8
	}
	if replacement.Authority == 0 {
		replacement.Authority = 0.5
	}
	if !store.ValidMemoryStatus(replacement.Status) {
		replacement.Status = store.MemoryStatusActive
	}
	if derefStr(replacement.ContentHash) == "" {
		hash := store.HashMemoryContent(replacement.Content)
		replacement.ContentHash = &hash
	}
	tid := tenantIDForInsert(ctx)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory supersede begin tx: %w", err)
	}
	defer tx.Rollback()

	where := "id = $1 AND status = 'active'"
	args := []any{oid}
	where, args = memoryTenantFilter(ctx, where, args)
	res, err := tx.ExecContext(ctx,
		`UPDATE memories SET status = 'superseded' WHERE `+
			strings.TrimPrefix(where, " AND "), args...)
	if err != nil {
		return fmt.Errorf("memory supersede update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if err := insertMemory(ctx, tx, tid, replacement); err != nil {
		return fmt.Errorf("memory supersede insert: %w", err)
	}
	return tx.Commit()
}

// ArchiveMemory flips an active or superseded memory to archived. Zero
// affected rows (absent / out of scope / already archived) return
// sql.ErrNoRows — archive is deliberately not silently idempotent.
func (s *PGMemoryFabricStore) ArchiveMemory(ctx context.Context, id string) error {
	mid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("memory id %q: %w", id, err)
	}
	where := "id = $1 AND status <> 'archived'"
	args := []any{mid}
	where, args = memoryTenantFilter(ctx, where, args)
	res, err := s.db.ExecContext(ctx,
		`UPDATE memories SET status = 'archived', updated_at = now()
		 WHERE `+strings.TrimPrefix(where, " AND "), args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
