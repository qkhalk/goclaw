-- Cloud starred items: per-user bookmarks of remote files/folders (Drive-style
-- "starred"). Providers do not expose star metadata through the rclone rc API,
-- so GoClaw stores it locally. User-level (not admin) — every caller manages
-- their own stars within the tenant. Removing the underlying account cascades.
CREATE TABLE cloud_starred (
    id         UUID PRIMARY KEY,
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    account_id UUID NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    name       TEXT NOT NULL,
    is_dir     BOOLEAN NOT NULL DEFAULT false,
    starred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One star per (tenant, user, account, path) — re-starring is a no-op.
CREATE UNIQUE INDEX uq_cloud_starred_path ON cloud_starred (tenant_id, user_id, account_id, path);
CREATE INDEX idx_cloud_starred_user ON cloud_starred (tenant_id, user_id, starred_at DESC);
