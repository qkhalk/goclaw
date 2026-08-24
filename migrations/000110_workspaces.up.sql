-- Workspaces domain table (Paseo plan Phase 2): a workspace is a sandboxed
-- root directory owned by a user within a tenant scope, optionally bound to a
-- git repository/branch and/or a linked git worktree checkout.
--
--   tenant_id  NULL = master/global scope (same convention as api_keys and
--              node_leases); deleting the tenant cascades to its workspaces.
--   owner_id   Owning user identifier. VARCHAR(255) to match agents.owner_id /
--              tenant_users.user_id — this schema has no users table (user
--              identity lives in external auth), so there is no FK target.
--   status     'active' (default) or 'archived'; archived workspaces stay on
--              disk but disappear from default listings until restored.
CREATE TABLE workspaces (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id     UUID REFERENCES tenants(id) ON DELETE CASCADE,
    owner_id      VARCHAR(255) NOT NULL,
    name          TEXT NOT NULL,
    root_path     TEXT NOT NULL,
    description   TEXT,
    status        TEXT NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','archived')),
    repo_url      TEXT,
    branch        TEXT,
    worktree_path TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Name uniqueness per owner across scopes: COALESCE maps NULL tenant_id
-- (master/global) onto the zero-UUID sentinel so two global workspaces with
-- the same owner+name collide instead of silently duplicating (NULLs are
-- distinct in unique indexes; same technique as idx_usage_snapshots_unique).
CREATE UNIQUE INDEX idx_workspaces_tenant_owner_name ON workspaces (
    COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid),
    owner_id, name
);

-- Owner-scoped listing filtered by lifecycle status (workspace.list default
-- excludes archived; includeArchived widens the filter).
CREATE INDEX idx_workspaces_tenant_owner_status
    ON workspaces (tenant_id, owner_id, status);

-- Recency ordering for cross-workspace dashboards (newest activity first).
CREATE INDEX idx_workspaces_updated ON workspaces (updated_at DESC);
