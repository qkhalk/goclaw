-- Tenant policies: typed per-tenant quota/allowlist/resource-limit/suspension
-- configuration. This is the typed home for settings that were previously only
-- free-form in tenants.settings (which stays untouched for back-compat).
--
--   quota           JSONB mirror of config.QuotaConfig shape (providers/channels/
--                   groups keyed by QuotaWindow). Tenant overrides global for
--                   the same key.
--   allowed_providers/allowed_models  If non-empty, restrict which LLM providers/
--                   models a tenant may use at chat entry (fail-closed: empty
--                   list means "not restricted").
--   max_agents/max_sessions/max_teams  Per-tenant resource caps (NULL = no cap).
--   status           'active' (default) or 'suspended'; a suspended policy blocks
--                   auth/connect + run entry, mirroring tenants.status.
CREATE TABLE IF NOT EXISTS tenant_policies (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    quota              JSONB NOT NULL DEFAULT '{}'::jsonb,
    allowed_providers  TEXT[] NOT NULL DEFAULT '{}',
    allowed_models     TEXT[] NOT NULL DEFAULT '{}',
    max_agents         INT NULL,
    max_sessions       INT NULL,
    max_teams          INT NULL,
    status             TEXT NOT NULL DEFAULT 'active',
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tenant_policies_tenant
    ON tenant_policies (tenant_id);