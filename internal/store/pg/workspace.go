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

// PGWorkspaceStore implements store.WorkspaceStore backed by PostgreSQL.
type PGWorkspaceStore struct {
	db *sql.DB
}

func NewPGWorkspaceStore(db *sql.DB) *PGWorkspaceStore {
	return &PGWorkspaceStore{db: db}
}

const workspaceColumns = `id, tenant_id, owner_id, name, root_path, description,
 status, repo_url, branch, worktree_path, created_at, updated_at`

func scanWorkspace(row interface{ Scan(...any) error }) (*store.Workspace, error) {
	var w store.Workspace
	var description, repoURL, branch, worktreePath sql.NullString
	if err := row.Scan(&w.ID, &w.TenantID, &w.OwnerID, &w.Name, &w.RootPath,
		&description, &w.Status, &repoURL, &branch, &worktreePath,
		&w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	w.Description = nilStrPtr(description)
	w.RepoURL = nilStrPtr(repoURL)
	w.Branch = nilStrPtr(branch)
	w.WorktreePath = nilStrPtr(worktreePath)
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
func (s *PGWorkspaceStore) CreateWorkspace(ctx context.Context, ws *store.Workspace) error {
	if strings.TrimSpace(ws.ID) == "" {
		ws.ID = store.GenNewID().String()
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
	var tenantID any
	if ws.TenantID != nil && *ws.TenantID != "" {
		tid, err := uuid.Parse(*ws.TenantID)
		if err != nil {
			return fmt.Errorf("workspace tenant_id: %w", err)
		}
		tenantID = tid
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces
		 (id, tenant_id, owner_id, name, root_path, description, status,
		  repo_url, branch, worktree_path, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		ws.ID, tenantID, ws.OwnerID, ws.Name, ws.RootPath,
		nilStr(derefStr(ws.Description)), ws.Status,
		nilStr(derefStr(ws.RepoURL)), nilStr(derefStr(ws.Branch)), nilStr(derefStr(ws.WorktreePath)),
		ws.CreatedAt, ws.UpdatedAt,
	)
	return err
}

func (s *PGWorkspaceStore) GetWorkspace(ctx context.Context, id string) (*store.Workspace, error) {
	wid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("workspace id %q: %w", id, err)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE id = $1`, wid)
	return scanWorkspace(row)
}

func (s *PGWorkspaceStore) ListWorkspaces(ctx context.Context, tenantID *string, ownerID string, includeArchived bool) ([]*store.Workspace, error) {
	where := "owner_id = $1"
	args := []any{ownerID}
	if tenantID != nil && *tenantID != "" {
		tid, err := uuid.Parse(*tenantID)
		if err != nil {
			return nil, fmt.Errorf("workspace tenant_id %q: %w", *tenantID, err)
		}
		args = append(args, tid)
		where += fmt.Sprintf(" AND tenant_id = $%d", len(args))
	} else {
		where += " AND tenant_id IS NULL"
	}
	if !includeArchived {
		where += fmt.Sprintf(" AND status <> $%d", len(args)+1)
		args = append(args, store.WorkspaceStatusArchived)
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

func (s *PGWorkspaceStore) UpdateWorkspace(ctx context.Context, ws *store.Workspace) error {
	wid, err := uuid.Parse(ws.ID)
	if err != nil {
		return fmt.Errorf("workspace id %q: %w", ws.ID, err)
	}
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET name = $2, description = $3, status = $4,
		 repo_url = $5, branch = $6, worktree_path = $7, updated_at = $8
		 WHERE id = $1`,
		wid, ws.Name, nilStr(derefStr(ws.Description)), ws.Status,
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

func (s *PGWorkspaceStore) DeleteWorkspace(ctx context.Context, id string) error {
	wid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("workspace id %q: %w", id, err)
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workspaces WHERE id = $1`, wid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("workspace not found")
	}
	return nil
}
