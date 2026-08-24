-- Node leases: separates device connectivity from authentication and agent
-- sessions (Paseo plan Phase 1). A WebSocket close marks the lease
-- RECONNECTING; it is never a logout. Lease TTL 60s, heartbeat 15s.
CREATE TABLE node_leases (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    node_id       TEXT NOT NULL UNIQUE,
    client_id     TEXT NOT NULL DEFAULT '',
    user_id       VARCHAR(255) NOT NULL,
    tenant_id     UUID REFERENCES tenants(id) ON DELETE CASCADE,
    resume_token  TEXT NOT NULL UNIQUE,
    session_epoch INT NOT NULL DEFAULT 1,
    status        TEXT NOT NULL DEFAULT 'online'
                  CHECK (status IN ('online','reconnecting','offline_grace','expired')),
    issued_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_node_leases_expires_at ON node_leases(expires_at);
CREATE INDEX idx_node_leases_tenant_user ON node_leases(tenant_id, user_id);
