-- Node runtime registry (inheritance plan Phase 2): one row per registered
-- compute node daemon. Distinct from node_leases (000109, UI-tab presence)
-- and from pairing (channel sender trust): a node is a daemon-run execution
-- target that authenticates with its own bearer key.
--
--   node_key_hash  SHA-256 hex of the bearer key the daemon presents at
--                  nodes.register. The plaintext is revealed exactly once at
--                  key creation (nodes.list create:true) and never stored.
--   platform       "os/arch" reported by the daemon at registration.
--   capabilities   JSONB array of advertised capabilities (exec, fs, browser).
--   trust          pending (default, nothing executes) | trusted | revoked
--                  (terminal — a revoked node needs a freshly created key).
CREATE TABLE nodes (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id     UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name          VARCHAR(255) NOT NULL,
    node_key_hash CHAR(64) NOT NULL UNIQUE,
    platform      TEXT NOT NULL DEFAULT '',
    capabilities  JSONB NOT NULL DEFAULT '[]'::jsonb,
    trust         TEXT NOT NULL DEFAULT 'pending'
                  CHECK (trust IN ('pending','trusted','revoked')),
    last_seen_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at    TIMESTAMPTZ
);

CREATE INDEX idx_nodes_tenant_trust ON nodes (tenant_id, trust);

CREATE INDEX idx_nodes_tenant_created ON nodes (tenant_id, created_at DESC);
