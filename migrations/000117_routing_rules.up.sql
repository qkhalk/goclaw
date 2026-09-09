-- Routing rules (inheritance plan Phase 4): tenant-scoped inbound routing
-- rules evaluated between config-binding peer matches and channel matches.
--
--   match           JSONB, all fields optional; a set field must equal the
--                   inbound message's value (camelCase keys, matching the WS
--                   wire shape): {channel, accountId, peerKind, peerId, guildId}.
--                   The column is named match_config because `match` is a
--                   reserved SQL keyword.
--   priority        ASC — the lowest number has the highest precedence;
--                   first matching enabled rule wins.
--   target_agent_id Owning agent (FK); resolution joins agents to route by
--                   agent_key.
CREATE TABLE routing_rules (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    priority        INT NOT NULL DEFAULT 100,
    match_config    JSONB NOT NULL DEFAULT '{}'::jsonb,
    target_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_routing_rules_eval
    ON routing_rules (tenant_id, enabled, priority, created_at);
