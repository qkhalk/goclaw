-- Cloud accounts: per-user OAuth connections to cloud providers (Google first).
-- Mirrors the mcp_oauth_tokens pattern (000084): token columns are stored
-- AES-256-GCM encrypted by the store layer ("aes-gcm:" prefix).
CREATE TABLE cloud_accounts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id          TEXT NOT NULL,
    provider         TEXT NOT NULL,
    email            TEXT NOT NULL,
    display_name     TEXT NOT NULL DEFAULT '',
    scopes           TEXT NOT NULL DEFAULT '[]',
    access_token     TEXT NOT NULL,
    refresh_token    TEXT NOT NULL DEFAULT '',
    token_expires_at TIMESTAMPTZ,
    status           TEXT NOT NULL DEFAULT 'active',
    status_message   TEXT NOT NULL DEFAULT '',
    settings         JSONB NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT cloud_accounts_provider_check CHECK (provider IN ('google'))
);

-- One connection per (tenant, user, provider, email): reconnecting the same
-- account upserts instead of piling up duplicate rows.
CREATE UNIQUE INDEX cloud_accounts_uq
    ON cloud_accounts (tenant_id, user_id, provider, email);

CREATE INDEX cloud_accounts_lookup ON cloud_accounts (tenant_id, user_id, provider);
