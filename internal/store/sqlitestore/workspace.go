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

// SQLiteWorkspaceStore implements store.WorkspaceStore backed by SQLite
// (desktop/lite edition).
type SQLiteWorkspaceStore struct {
	db *sql.DB
}

func NewSQLiteWorkspaceStore(db *sql.DB) *SQLiteWorkspaceStore {
	return &SQLiteWorkspaceStore{db: db}
}

const workspaceColumns = `id, tenant_id, owner_id, name, root_path, description,
 status, repo_url, branch, worktree_path, created_at, updated_at`

func scanWorkspace(row interface{ Scan(...any) error }) (*store.Workspace, error) {
	var w store.Workspace
	var description, repoURL, branch, worktreePath sql.NullString
	// created/updated are TEXT in SQLite (modernc driver returns strings);
	// scan through sqliteTime so RFC3339 text round-trips into time.Time.
	var createdAt, updatedAt sqliteTime
	if err := row.Scan(&w.ID, &w.TenantID, &w.OwnerID, &w.Name, &w.RootPath,
		&description, &w.Status, &repoURL, &branch, &worktreePath,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	w.Description = nilStrPtr(description)
	w.RepoURL = nilStrPtr(repoURL)
	w.Branch = nilStrPtr(branch)
	w.WorktreePath = nilStrPtr(worktreePath)
	w.CreatedAt = createdAt.Time
	w.UpdatedAt = updatedAt.Time
	return &w, nil
}

// nilStrPtr normalizes a scanned NULL column to a nil pointer (nil for NULL,
// pointer to the value otherwise).
func nilStrPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

// CreateWorkspace inserts a new workspace. ID defaults to a fresh UUIDv7;
// timestamps and status are defaulted when zero-valued. tenant_id is taken
// from the Workspace row itself (nil = master/global) — the gateway layer
// resolves it from the client session before calling the store.
func (s *SQLiteWorkspaceStore) CreateWorkspace(ctx context.Context, ws *store.Workspace) error {
	if strings.TrimSpace(ws.ID) == "" {
		ws.ID = uuid.NewString()
	}
	now := time.Now()
	if ws.CreatedAt.IsZero() {
		ws.CreatedAt = now
	}
	if ws.UpdatedAt.IsZero() {
		ws.UpdatedAt = now
	}
	if !store.ValidWorkspaceStatus(ws.Status) {
		ws.Status = store.WorkspaceStatusActive
	}
	var tenantArg any
	if ws.TenantID != nil && *ws.TenantID != "" {
		tenantArg = *ws.TenantID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces
		 (id, tenant_id, owner_id, name, root_path, description, status,
		  repo_url, branch, worktree_path, created_at, updated_at)
		 VALUES (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12)`,
		ws.ID, tenantArg, ws.OwnerID, ws.Name, ws.RootPath,
		nilStr(derefStr(ws.Description)), ws.Status,
		nilStr(derefStr(ws.RepoURL)), nilStr(derefStr(ws.Branch)), nilStr(derefStr(ws.WorktreePath)),
		ws.CreatedAt.UTC(), ws.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteWorkspaceStore) GetWorkspace(ctx context.Context, id string) (*store.Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE id = ?1`, id)
	return scanWorkspace(row)
}

func (s *SQLiteWorkspaceStore) ListWorkspaces(ctx context.Context, tenantID *string, ownerID string, includeArchived bool) ([]*store.Workspace, error) {
	where := "owner_id = ?1"
	args := []any{ownerID}
	if tenantID != nil && *tenantID != "" {
		args = append(args, *tenantID)
		where += " AND tenant_id = ?2"
	} else {
		where += " AND tenant_id IS NULL"
	}
	if !includeArchived {
		args = append(args, store.WorkspaceStatusArchived)
		where += fmt.Sprintf(" AND status <> ?%d", len(args))
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE `+where+` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.Workspace
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkspaceStore) UpdateWorkspace(ctx context.Context, ws *store.Workspace) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET name = ?2, description = ?3, status = ?4,
		 repo_url = ?5, branch = ?6, worktree_path = ?7, updated_at = ?8
		 WHERE id = ?1`,
		ws.ID, ws.Name, nilStr(derefStr(ws.Description)), ws.Status,
		nilStr(derefStr(ws.RepoURL)), nilStr(derefStr(ws.Branch)), nilStr(derefStr(ws.WorktreePath)), now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("workspace not found: %w", sql.ErrNoRows)
	}
	ws.UpdatedAt = now
	return nil
}

func (s *SQLiteWorkspaceStore) DeleteWorkspace(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workspaces WHERE id = ?1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("workspace not found")
	}
	return nil
}
