-- Cloud sync pairs: one-way (additive mirror) folder sync between two
-- connected accounts, run by the SyncService worker in internal/cloud.
-- interval_minutes = 0 means manual ("run now") only. last_run_at is the run
-- START time (set when the pair enters "running") so the due check never
-- re-fires a pair that is still executing.
CREATE TABLE cloud_sync_pairs (
    id                 UUID PRIMARY KEY,
    tenant_id          UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_account_id  UUID NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
    source_path        TEXT NOT NULL DEFAULT '/',
    target_account_id  UUID NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
    target_path        TEXT NOT NULL DEFAULT '/',
    interval_minutes   INTEGER NOT NULL DEFAULT 0,
    enabled            BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at        TIMESTAMPTZ,
    last_status        TEXT,
    last_error         TEXT,
    created_by         UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cloud_sync_pairs_tenant ON cloud_sync_pairs (tenant_id);
