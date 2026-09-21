//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteMCPInstallStore implements store.MCPInstallStore backed by SQLite.
type SQLiteMCPInstallStore struct {
	db *sql.DB
}

func NewSQLiteMCPInstallStore(db *sql.DB) *SQLiteMCPInstallStore {
	return &SQLiteMCPInstallStore{db: db}
}

const mcpInstallSelectCols = `id, tenant_id, name, display_name, source, repo, ref, commit_sha,
	runtime, entry, install_dir, status, error, tool_count, created_at, updated_at`

func (s *SQLiteMCPInstallStore) UpsertPackage(ctx context.Context, p *store.MCPInstalledPackage) error {
	if p.ID == uuid.Nil {
		p.ID = store.GenNewID()
	}
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	p.TenantID = tenantID
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mcp_installed_packages (id, tenant_id, name, display_name, source, repo, ref, commit_sha,
		   runtime, entry, install_dir, status, error, tool_count, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT (tenant_id, name) DO UPDATE SET
		   display_name = excluded.display_name,
		   source       = excluded.source,
		   repo         = excluded.repo,
		   ref          = excluded.ref,
		   commit_sha   = excluded.commit_sha,
		   runtime      = excluded.runtime,
		   entry        = excluded.entry,
		   install_dir  = excluded.install_dir,
		   status       = excluded.status,
		   error        = excluded.error,
		   tool_count   = excluded.tool_count,
		   updated_at   = excluded.updated_at`,
		p.ID.String(), tenantID.String(), p.Name, p.DisplayName, p.Source, p.Repo, p.Ref, p.CommitSHA,
		p.Runtime, p.Entry, p.InstallDir, p.Status, ste(p.Error), p.ToolCount, now, now)
	return err
}

// ste stringifies a *string for nullable columns (nil → NULL).
func ste(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func (s *SQLiteMCPInstallStore) GetPackageByName(ctx context.Context, name string) (*store.MCPInstalledPackage, error) {
	q := `SELECT ` + mcpInstallSelectCols + ` FROM mcp_installed_packages WHERE name = ?`
	args := []any{name}
	if !store.IsCrossTenant(ctx) {
		tenantID := store.TenantIDFromContext(ctx)
		if tenantID == uuid.Nil {
			return nil, sql.ErrNoRows
		}
		q += ` AND tenant_id = ?`
		args = append(args, tenantID.String())
	}
	list, err := s.queryPackages(ctx, q+" LIMIT 1", args...)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	return &list[0], nil
}

func (s *SQLiteMCPInstallStore) ListPackages(ctx context.Context) ([]store.MCPInstalledPackage, error) {
	q := `SELECT ` + mcpInstallSelectCols + ` FROM mcp_installed_packages`
	var args []any
	if !store.IsCrossTenant(ctx) {
		tenantID := store.TenantIDFromContext(ctx)
		if tenantID == uuid.Nil {
			return []store.MCPInstalledPackage{}, nil
		}
		q += ` WHERE tenant_id = ?`
		args = append(args, tenantID.String())
	}
	return s.queryPackages(ctx, q+" ORDER BY name", args...)
}

func (s *SQLiteMCPInstallStore) DeletePackage(ctx context.Context, name string) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return fmt.Errorf("tenant_id required")
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM mcp_installed_packages WHERE name = ? AND tenant_id = ?`, name, tenantID.String())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLiteMCPInstallStore) queryPackages(ctx context.Context, q string, args ...any) ([]store.MCPInstalledPackage, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []store.MCPInstalledPackage
	for rows.Next() {
		var p store.MCPInstalledPackage
		var id, tenantID, createdAt, updatedAt string
		if err := rows.Scan(&id, &tenantID, &p.Name, &p.DisplayName, &p.Source, &p.Repo, &p.Ref,
			&p.CommitSHA, &p.Runtime, &p.Entry, &p.InstallDir, &p.Status, &p.Error, &p.ToolCount,
			&createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if p.ID, err = uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("parse id: %w", err)
		}
		if p.TenantID, err = uuid.Parse(tenantID); err != nil {
			return nil, fmt.Errorf("parse tenant_id: %w", err)
		}
		if p.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
			p.CreatedAt = time.Time{}
		}
		if p.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt); err != nil {
			p.UpdatedAt = time.Time{}
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
