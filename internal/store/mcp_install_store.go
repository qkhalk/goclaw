package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MCPInstalledPackage records one tool-server installed from git by the MCP
// installer (Tool Store → "Tải & cài"). The row survives restarts so installs
// can be audited (commit SHA), repaired (status=installing after a crash),
// and uninstalled (install_dir is the only place the files live).
type MCPInstalledPackage struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"` // slug, matches mcp_servers.name
	DisplayName string    `json:"display_name" db:"display_name"`
	Source      string    `json:"source" db:"source"` // "catalog" | "custom"
	Repo        string    `json:"repo" db:"repo"`
	Ref         string    `json:"ref" db:"ref"`
	CommitSHA   string    `json:"commit_sha" db:"commit_sha"`
	Runtime     string    `json:"runtime" db:"runtime"` // "node" | "python"
	Entry       string    `json:"entry" db:"entry"`
	InstallDir  string    `json:"install_dir" db:"install_dir"`
	Status      string    `json:"status" db:"status"` // "installing" | "installed" | "failed"
	Error       *string   `json:"error,omitempty" db:"error"`
	ToolCount   int       `json:"tool_count" db:"tool_count"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// MCP install package status values.
const (
	MCPInstallStatusInstalling = "installing"
	MCPInstallStatusInstalled  = "installed"
	MCPInstallStatusFailed     = "failed"
)

// MCPInstallStore persists installed tool packages. Tenant-scoped via ctx
// like MCPServerStore; (tenant_id, name) is unique.
type MCPInstallStore interface {
	// UpsertPackage inserts or updates the package row for (tenant, name).
	UpsertPackage(ctx context.Context, p *MCPInstalledPackage) error
	// GetPackageByName returns the package for (tenant, name) or
	// sql.ErrNoRows.
	GetPackageByName(ctx context.Context, name string) (*MCPInstalledPackage, error)
	// ListPackages returns all packages for the ctx tenant.
	ListPackages(ctx context.Context) ([]MCPInstalledPackage, error)
	// DeletePackage removes the package row for (tenant, name).
	DeletePackage(ctx context.Context, name string) error
}
