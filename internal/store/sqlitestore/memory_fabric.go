//go:build sqlite || sqliteonly

package sqlitestore

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

// SQLiteMemoryFabricStore implements store.MemoryFabricStore backed by
// SQLite (desktop/lite edition).
type SQLiteMemoryFabricStore struct {
	db *sql.DB
}

func NewSQLiteMemoryFabricStore(db *sql.DB) *SQLiteMemoryFabricStore {
	return &SQLiteMemoryFabricStore{db: db}
}

const memoryFabricColumns = `id, tenant_id, user_id, agent_id, workspace_id,
 session_key, scope, kind, content, source_type, source_ref, confidence,
 authority, status, supersedes_id, contradicts_id, content_hash,
 embedding_version, created_at, updated_at`

func scanMemoryFabric(row interface{ Scan(...any) error }) (*store.Memory, error) {
	var m store.Memory
	var userID, agentID, workspaceID, sessionKey, sourceRef,
		supersedesID, contradictsID, contentHash, embeddingVersion sql.NullString
	// Timestamps are TEXT in SQLite (modernc driver returns strings); scan
	// through sqliteTime so RFC3339 text round-trips into time.Time.
	var createdAt, updatedAt sqliteTime
	if err := row.Scan(&m.ID, &m.TenantID, &userID, &agentID, &workspaceID,
		&sessionKey, &m.Scope, &m.Kind, &m.Content, &m.SourceType, &sourceRef,
		&m.Confidence, &m.Authority, &m.Status, &supersedesID, &contradictsID,
		&contentHash, &embeddingVersion, &createdAt, &updatedAt); err != nil {
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
	m.CreatedAt = createdAt.Time
	m.UpdatedAt = updatedAt.Time
	return &m, nil
}

// memoryTenantFilter appends the context-tenant condition to a WHERE clause
// being built with ?N placeholders. Master scope (IsMasterScope) sees all
// rows; with a tenant present the filter is exact.
func memoryTenantFilter(ctx context.Context, where string, args []any) (string, []any) {
	if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
		args = append(args, store.TenantIDFromContext(ctx).String())
		where += fmt.Sprintf(" AND tenant_id = ?%d", len(args))
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
			fmt.Sprintf("%s IS ?%d", t.col, startParam+len(args)))
	}
	args = append(args, m.Scope)
	clauses = append(clauses, fmt.Sprintf("scope = ?%d", startParam+len(args)))
	return strings.Join(clauses, " AND "), args
}

// insertMemory stamps timestamps and inserts one row. Shared by WriteMemory
// and SupersedeMemory so both apply identical defaults.
func (s *SQLiteMemoryFabricStore) insertMemory(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, tid string, m *store.Memory) error {
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
		 VALUES (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15,?16,?17,?18,?19,?20)`,
		m.ID, nilStr(tid), nilStr(derefStr(m.UserID)), nilStr(derefStr(m.AgentID)),
		nilStr(derefStr(m.WorkspaceID)), nilStr(derefStr(m.SessionKey)),
		m.Scope, m.Kind, m.Content, m.SourceType, nilStr(derefStr(m.SourceRef)),
		m.Confidence, m.Authority, m.Status,
		nilStr(derefStr(m.SupersedesID)), nilStr(derefStr(m.ContradictsID)),
		nilStr(derefStr(m.ContentHash)), nilStr(derefStr(m.EmbeddingVersion)),
		m.CreatedAt.UTC(), m.UpdatedAt.UTC())
	return err
}

// WriteMemory upserts a memory within the same (tenant,user,agent,workspace,
// scope) tuple: identical content hash updates content/updated_at in place
// (m.ID is rewritten to the existing row id); different content supersedes
// the newest active near-key atomically — new row carries supersedes_id and
// the old row flips to 'superseded' in the same transaction. Global-scope
// rows never supersede (they are shared by definition).
func (s *SQLiteMemoryFabricStore) WriteMemory(ctx context.Context, m *store.Memory) error {
	if strings.TrimSpace(m.ID) == "" {
		m.ID = uuid.NewString()
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory write begin tx: %w", err)
	}
	defer tx.Rollback()

	// Newest active near-key duplicate (same ownership tuple, non-global).
	dupWhere, dupArgs := memoryTupleMatch(1, m)
	if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
		dupArgs = append(dupArgs, store.TenantIDFromContext(ctx).String())
		dupWhere += fmt.Sprintf(" AND tenant_id = ?%d", len(dupArgs))
	}
	// kindParam must be computed AFTER the tenant filter is appended (the
	// placeholder number depends on the arg count). SQLite does not error on
	// the mismatched comparison like PG does — the dedup lookup silently
	// matched nothing, so every write skipped dedup/supersession.
	kindParam := len(dupArgs) + 1
	dupQuery := `SELECT id, content_hash FROM memories
		WHERE status = 'active' AND scope <> 'global' AND content_hash IS NOT NULL AND (` +
		dupWhere + `) AND kind = ?` + fmt.Sprint(kindParam) + `
		ORDER BY updated_at DESC LIMIT 1`
	dupArgs = append(dupArgs, m.Kind)

	var existingID, existingHash sql.NullString
	err = tx.QueryRowContext(ctx, dupQuery, dupArgs...).Scan(&existingID, &existingHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		tid := ""
		if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
			tid = tenantIDForInsert(ctx).String()
		}
		if err := s.insertMemory(ctx, tx, tid, m); err != nil {
			return fmt.Errorf("memory write insert: %w", err)
		}
	case err != nil:
		return fmt.Errorf("memory write dedup lookup: %w", err)
	case existingHash.Valid && existingHash.String == derefStr(m.ContentHash):
		// Identical content: refresh in place, keep the canonical id.
		now := time.Now()
		res, uerr := tx.ExecContext(ctx,
			`UPDATE memories SET content = ?2, updated_at = ?3 WHERE id = ?1`,
			existingID, m.Content, now.UTC().Format(time.RFC3339Nano))
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
		tid := ""
		if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
			tid = tenantIDForInsert(ctx).String()
		}
		if err := s.insertMemory(ctx, tx, tid, m); err != nil {
			return fmt.Errorf("memory write supersede insert: %w", err)
		}
		if _, uerr := tx.ExecContext(ctx,
			`UPDATE memories SET status = 'superseded', updated_at = updated_at
			 WHERE id = ?1`, existingID); uerr != nil {
			return fmt.Errorf("memory write supersede update: %w", uerr)
		}
	}
	return tx.Commit()
}

// GetMemory resolves one memory by id, scoped to the context tenant
// (master scope sees all). sql.ErrNoRows when not found / not visible.
func (s *SQLiteMemoryFabricStore) GetMemory(ctx context.Context, id string) (*store.Memory, error) {
	where := "id = ?1"
	args := []any{id}
	where, args = memoryTenantFilter(ctx, where, args)
	row := s.db.QueryRowContext(ctx,
		`SELECT `+memoryFabricColumns+` FROM memories WHERE `+where, args...)
	return scanMemoryFabric(row)
}

// inPlaceholders appends the values from vals that pass valid to args and
// returns a comma-joined placeholder list ("?3,?4") for an IN (...) predicate.
// Empty string means no valid values — apply no filter.
func inPlaceholders(args *[]any, vals []string, valid func(string) bool) string {
	var ph []string
	for _, v := range vals {
		if !valid(v) {
			continue
		}
		*args = append(*args, v)
		ph = append(ph, fmt.Sprintf("?%d", len(*args)))
	}
	return strings.Join(ph, ",")
}

// SearchMemories applies the retrieval hard gates (status='active' plus
// identity gates on the non-empty query fields), optional scope/kind
// whitelists, and ranks by authority/confidence/recency computed in SQL via
// julianday math. LIMIT defaults to 50 when q.Limit is not positive.
func (s *SQLiteMemoryFabricStore) SearchMemories(ctx context.Context, q store.MemoryQuery) ([]store.ScoredMemory, error) {
	where := "status = 'active'"
	var args []any

	// Tenant hard gate: master scope is unrestricted, everyone else is pinned
	// to their own tenant's rows.
	if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
		args = append(args, store.TenantIDFromContext(ctx).String())
		where += fmt.Sprintf(" AND tenant_id = ?%d", len(args))
	}
	if q.UserID != "" {
		args = append(args, q.UserID)
		n := len(args)
		where += fmt.Sprintf(" AND (user_id = ?%d OR scope = '%s')", n, store.MemoryScopeGlobal)
	}
	if q.AgentID != "" {
		args = append(args, q.AgentID)
		n := len(args)
		where += fmt.Sprintf(" AND (agent_id = ?%d OR scope IN ('%s','%s'))",
			n, store.MemoryScopeGlobal, store.MemoryScopeUser)
	}
	if q.WorkspaceID != "" {
		args = append(args, q.WorkspaceID)
		n := len(args)
		where += fmt.Sprintf(` AND (workspace_id = ?%d OR scope NOT IN ('%s','%s','%s','%s'))`,
			n, store.MemoryScopeWorkspace, store.MemoryScopeProject,
			store.MemoryScopeSession, store.MemoryScopeThread)
	}
	if q.SessionKey != "" {
		args = append(args, q.SessionKey)
		where += fmt.Sprintf(" AND session_key = ?%d", len(args))
	}
	// Scope/kind whitelists are unions (IN (...)), not per-element ANDs:
	// AND-chaining two whitelist values yields a contradiction that always
	// matches nothing. Invalid values are skipped, an all-invalid list applies
	// no filter.
	if ph := inPlaceholders(&args, q.Scopes, store.ValidMemoryScope); ph != "" {
		where += " AND scope IN (" + ph + ")"
	}
	if ph := inPlaceholders(&args, q.Kinds, store.ValidMemoryKind); ph != "" {
		where += " AND kind IN (" + ph + ")"
	}
	limit := 50
	if q.Limit > 0 {
		limit = q.Limit
	}
	// The score is not selected as a column: the shared 20-column memory scan
	// must stay exact-match for database/sql Scan. Ordering uses the same
	// formula MemoryRecallScore computes (see its doc for the sync note).
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+memoryFabricColumns+`
		 FROM memories WHERE `+where+`
		 ORDER BY authority*0.10 + confidence*0.10 +
		   MAX(0, 1 - (julianday('now')-julianday(updated_at))/2592000.0)*0.10 DESC,
		   updated_at DESC
		 LIMIT `+fmt.Sprint(limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []store.ScoredMemory{}
	now := time.Now()
	for rows.Next() {
		m, err := scanMemoryFabric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, store.ScoredMemory{Memory: m, Score: store.MemoryRecallScore(m, now)})
	}
	return store.FilterContradictedMemories(out), rows.Err()
}

// SupersedeMemory marks oldID superseded and inserts replacement pointing back
// at it in one transaction. The target must exist, be visible from the
// caller's tenant scope and still be active — otherwise sql.ErrNoRows.
func (s *SQLiteMemoryFabricStore) SupersedeMemory(ctx context.Context, oldID string, replacement *store.Memory) error {
	replacement.SupersedesID = &oldID
	if replacement.ID == "" {
		replacement.ID = uuid.NewString()
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory supersede begin tx: %w", err)
	}
	defer tx.Rollback()

	where := "id = ?1 AND status = 'active'"
	args := []any{oldID}
	where, args = memoryTenantFilter(ctx, where, args)
	res, err := tx.ExecContext(ctx,
		`UPDATE memories SET status = 'superseded' WHERE `+where, args...)
	if err != nil {
		return fmt.Errorf("memory supersede update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	tid := ""
	if !store.IsCrossTenant(ctx) && !store.IsMasterScope(ctx) {
		tid = tenantIDForInsert(ctx).String()
	}
	if err := s.insertMemory(ctx, tx, tid, replacement); err != nil {
		return fmt.Errorf("memory supersede insert: %w", err)
	}
	return tx.Commit()
}

// ArchiveMemory flips an active or superseded memory to archived. Zero
// affected rows (absent / out of scope / already archived) return
// sql.ErrNoRows — archive is deliberately not silently idempotent.
func (s *SQLiteMemoryFabricStore) ArchiveMemory(ctx context.Context, id string) error {
	where := "id = ?1 AND status <> 'archived'"
	args := []any{id}
	where, args = memoryTenantFilter(ctx, where, args)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx,
		`UPDATE memories SET status = 'archived', updated_at = ?2 WHERE `+where,
		append(args, now)...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
