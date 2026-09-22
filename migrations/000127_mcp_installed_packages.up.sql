-- MCP installed packages (Tool Store tier-3 installer): one row per tool
-- server installed from git. (tenant_id, name) unique — name matches
-- mcp_servers.name so uninstall can join registry + files.
CREATE TABLE mcp_installed_packages (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL DEFAULT '0193a5b0-7000-7000-8000-000000000001' REFERENCES tenants(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL DEFAULT 'catalog',
    repo         TEXT NOT NULL,
    ref          TEXT NOT NULL,
    commit_sha   TEXT NOT NULL DEFAULT '',
    runtime      TEXT NOT NULL,
    entry        TEXT NOT NULL,
    install_dir  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'installing',
    error        TEXT,
    tool_count   INT  NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX idx_mcp_installed_packages_tenant ON mcp_installed_packages (tenant_id);
