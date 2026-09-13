-- Cloud account access: tenant-wide sharing + per-scope account bindings.
-- shared=true exposes one user's connected account to every agent in the
-- tenant (the enterprise "company drive" pattern); bindings pin a provider
-- account per scope: tenant default / one user / one group chat.
ALTER TABLE cloud_accounts ADD COLUMN shared BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE cloud_account_bindings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    scope_type  TEXT NOT NULL CHECK (scope_type IN ('tenant', 'user', 'group')),
    scope_key   TEXT NOT NULL DEFAULT '',
    provider    TEXT NOT NULL,
    account_id  UUID NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT cloud_account_bindings_scope_check
        CHECK ((scope_type = 'tenant' AND scope_key = '') OR (scope_type <> 'tenant' AND scope_key <> ''))
);

-- One account per (scope, provider): re-assigning upserts instead of piling up.
CREATE UNIQUE INDEX cloud_account_bindings_uq
    ON cloud_account_bindings (tenant_id, scope_type, scope_key, provider);

CREATE INDEX cloud_account_bindings_lookup ON cloud_account_bindings (tenant_id, scope_type);
