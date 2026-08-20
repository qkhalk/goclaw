-- Fine-grained RBAC: per-tenant custom roles + role_permissions.
-- tenant_users.role stays the back-compat anchor; builtin roles (owner/admin/
-- operator/member/viewer) resolve to their existing tier. Custom roles are
-- defined per-tenant here and override the tier default on a
-- resource:action = effect basis.
CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT,
    builtin     BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_tenant_name
    ON roles (tenant_id, name);

-- role_permissions: one row per (role, permission) grant/deny override.
-- permission is a resource:action pair (e.g. 'agent:create'); effect is
-- 'allow' (default) or 'deny'. deny always wins over allow at evaluation.
CREATE TABLE IF NOT EXISTS role_permissions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    effect     TEXT NOT NULL DEFAULT 'allow',
    UNIQUE (role_id, permission)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_role_permissions_role_perm
    ON role_permissions (role_id, permission);

-- member_role_assignments: per-member custom-role grants. Builtin role on
-- tenant_users.role stays authoritative; custom roles here union with it
-- during RBAC evaluation (deny wins over allow).
CREATE TABLE IF NOT EXISTS member_role_assignments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    VARCHAR(255) NOT NULL,
    role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, role_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_member_role_assignments_unique
    ON member_role_assignments (tenant_id, user_id, role_id);

CREATE INDEX IF NOT EXISTS idx_member_role_assignments_role
    ON member_role_assignments (role_id);