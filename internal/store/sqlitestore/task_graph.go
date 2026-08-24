//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteTaskGraphStore implements store.TaskGraphStore backed by SQLite
// (desktop/lite edition).
type SQLiteTaskGraphStore struct {
	db *sql.DB
}

func NewSQLiteTaskGraphStore(db *sql.DB) *SQLiteTaskGraphStore {
	return &SQLiteTaskGraphStore{db: db}
}

const taskNodeColumns = `id, tenant_id, workspace_id, parent_id, owner_agent_id,
 session_key, title, status, priority, depends_on,
 result_ref, created_at, updated_at`

func scanTaskNode(row interface{ Scan(...any) error }) (*store.TaskNode, error) {
	var t store.TaskNode
	var workspaceID, parentID, ownerAgentID, sessionKey, resultRef sql.NullString
	var dependsOn []byte
	// created/updated are TEXT in SQLite (modernc driver returns strings);
	// scan through sqliteTime so RFC3339 text round-trips into time.Time.
	var createdAt, updatedAt sqliteTime
	if err := row.Scan(&t.ID, &t.TenantID, &workspaceID, &parentID,
		&ownerAgentID, &sessionKey, &t.Title, &t.Status, &t.Priority,
		&dependsOn, &resultRef, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	t.WorkspaceID = nilStrPtr(workspaceID)
	t.ParentID = nilStrPtr(parentID)
	t.OwnerAgentID = nilStrPtr(ownerAgentID)
	t.SessionKey = nilStrPtr(sessionKey)
	t.ResultRef = nilStrPtr(resultRef)
	t.DependsOn = decodeDependsOn(dependsOn)
	t.CreatedAt = createdAt.Time
	t.UpdatedAt = updatedAt.Time
	return &t, nil
}

// encodeDependsOn marshals the dependency id list to its JSON array storage
// form. Nil and empty slices both persist as '[]' (the column is NOT NULL).
func encodeDependsOn(ids []string) (string, error) {
	if len(ids) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("task depends_on: %w", err)
	}
	return string(b), nil
}

// decodeDependsOn parses a stored JSON array of task ids back into a slice.
// NULL/empty columns and malformed payloads degrade to an empty (non-nil)
// slice so callers never see a nil DependsOn.
func decodeDependsOn(data []byte) []string {
	out := []string{}
	if len(data) == 0 {
		return out
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return []string{}
	}
	return out
}

// CreateTask inserts a new task node. ID defaults to a fresh UUIDv7;
// timestamps are defaulted when zero-valued and status falls back to
// 'pending'. tenant_id is taken from the TaskNode row itself (nil =
// master/global) — the gateway layer resolves it from the client session
// before calling the store.
func (s *SQLiteTaskGraphStore) CreateTask(ctx context.Context, t *store.TaskNode) error {
	if strings.TrimSpace(t.ID) == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	if !store.ValidTaskStatus(t.Status) {
		t.Status = store.TaskStatusPending
	}
	dependsOn, err := encodeDependsOn(t.DependsOn)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO task_graph
		 (id, tenant_id, workspace_id, parent_id, owner_agent_id, session_key,
		  title, status, priority, depends_on, result_ref, created_at, updated_at)
		 VALUES (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13)`,
		t.ID, nilStr(derefStr(t.TenantID)), nilStr(derefStr(t.WorkspaceID)),
		nilStr(derefStr(t.ParentID)), nilStr(derefStr(t.OwnerAgentID)),
		nilStr(derefStr(t.SessionKey)), t.Title, t.Status, t.Priority,
		dependsOn, nilStr(derefStr(t.ResultRef)),
		t.CreatedAt.UTC(), t.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteTaskGraphStore) GetTask(ctx context.Context, id string) (*store.TaskNode, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+taskNodeColumns+` FROM task_graph WHERE id = ?1`, id)
	return scanTaskNode(row)
}

// ListTasks returns the whole tree for a workspace ordered roots-first:
// `(parent_id IS NULL) DESC` sorts true (roots) ahead of children in both PG
// and SQLite (NULLS FIRST needs SQLite >= 3.30), then priority DESC, oldest
// created_at first inside each group so sibling order is stable across reads.
func (s *SQLiteTaskGraphStore) ListTasks(ctx context.Context, workspaceID string) ([]*store.TaskNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+taskNodeColumns+` FROM task_graph WHERE workspace_id = ?1
		 ORDER BY (parent_id IS NULL) DESC, priority DESC, created_at ASC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.TaskNode
	for rows.Next() {
		t, err := scanTaskNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTaskStatus transitions a task's status and records result_ref when
// provided (non-nil); updated_at is refreshed by the implementation.
func (s *SQLiteTaskGraphStore) UpdateTaskStatus(ctx context.Context, id, status string, resultRef *string) error {
	if !store.ValidTaskStatus(status) {
		return fmt.Errorf("update task status: unknown status %q", status)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE task_graph SET status = ?2,
		 result_ref = COALESCE(?3, result_ref), updated_at = ?4
		 WHERE id = ?1`,
		id, status, nilStr(derefStr(resultRef)), now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("task not found: %w", sql.ErrNoRows)
	}
	return nil
}

// DeleteTask hard-deletes a task node; child nodes cascade via the
// parent_id self-FK. Missing rows surface as sql.ErrNoRows like GetTask.
func (s *SQLiteTaskGraphStore) DeleteTask(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM task_graph WHERE id = ?1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("task not found")
	}
	return nil
}
