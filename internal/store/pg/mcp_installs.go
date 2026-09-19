package pg

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGMCPInstallStore implements store.MCPInstallStore backed by Postgres.
type PGMCPInstallStore struct {
	db *sql.DB
}

func NewPGMCPInstallStore(db *sql.DB) *PGMCPInstallStore {
	return &PGMCPInstallStore{db: db}
}

const mcpInstallCols = `id, tenant_id, name, display_name, source, repo, ref, commit_sha,
	runtime, entry, install_dir, status, error, tool_count, created_at, updated_at`

// tenantClause appends "tenant_id = $N" as a WHERE (no prior condition) or
// AND (prior WHERE) clause, mirroring the pattern in mcp_servers.go.
func tenantClause(q string, hasWhere bool, argIdx int) string {
	join := " WHERE"
	if hasWhere {
		join = " AND"
	}
	return fmt.Sprintf("%s%s tenant_id = $%d", q, join, argIdx)
}

func (s *PGMCPInstallStore) UpsertPackage(ctx context.Context, p *store.MCPInstalledPackage) error {
	if p.ID == uuid.Nil {
		p.ID = store.GenNewID()
	}
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	p.TenantID = tenantID

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mcp_installed_packages (id, tenant_id, name, display_name, source, repo, ref, commit_sha,
		   runtime, entry, install_dir, status, error, tool_count, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now(),now())
		 ON CONFLICT (tenant_id, name) DO UPDATE SET
		   display_name = EXCLUDED.display_name,
		   source       = EXCLUDED.source,
		   repo         = EXCLUDED.repo,
		   ref          = EXCLUDED.ref,
		   commit_sha   = EXCLUDED.commit_sha,
		   runtime      = EXCLUDED.runtime,
		   entry        = EXCLUDED.entry,
		   install_dir  = EXCLUDED.install_dir,
		   status       = EXCLUDED.status,
		   error        = EXCLUDED.error,
		   tool_count   = EXCLUDED.tool_count,
		   updated_at   = now()`,
		p.ID, tenantID, p.Name, p.DisplayName, p.Source, p.Repo, p.Ref, p.CommitSHA,
		p.Runtime, p.Entry, p.InstallDir, p.Status, ste(p.Error), p.ToolCount)
	return err
}

// ste stringifies a *string for nullable columns (nil → NULL).
func ste(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func (s *PGMCPInstallStore) GetPackageByName(ctx context.Context, name string) (*store.MCPInstalledPackage, error) {
	q := `SELECT ` + mcpInstallCols + ` FROM mcp_installed_packages WHERE name = $1`
	args := []any{name}
	if !store.IsCrossTenant(ctx) {
		tenantID := store.TenantIDFromContext(ctx)
		if tenantID == uuid.Nil {
			return nil, sql.ErrNoRows
		}
		q = tenantClause(q, true, 2)
		args = append(args, tenantID)
	}
	list, err := s.queryPackages(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	return &list[0], nil
}

func (s *PGMCPInstallStore) ListPackages(ctx context.Context) ([]store.MCPInstalledPackage, error) {
	q := `SELECT ` + mcpInstallCols + ` FROM mcp_installed_packages`
	var args []any
	if !store.IsCrossTenant(ctx) {
		tenantID := store.TenantIDFromContext(ctx)
		if tenantID == uuid.Nil {
			return []store.MCPInstalledPackage{}, nil
		}
		q = tenantClause(q, false, 1)
		args = append(args, tenantID)
	}
	return s.queryPackages(ctx, q+" ORDER BY name", args...)
}

func (s *PGMCPInstallStore) DeletePackage(ctx context.Context, name string) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return fmt.Errorf("tenant_id required")
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM mcp_installed_packages WHERE name = $1 AND tenant_id = $2`, name, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *PGMCPInstallStore) queryPackages(ctx context.Context, q string, args ...any) ([]store.MCPInstalledPackage, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []store.MCPInstalledPackage
	for rows.Next() {
		var p store.MCPInstalledPackage
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.DisplayName, &p.Source, &p.Repo, &p.Ref,
			&p.CommitSHA, &p.Runtime, &p.Entry, &p.InstallDir, &p.Status, &p.Error, &p.ToolCount,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
