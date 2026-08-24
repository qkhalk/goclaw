-- Memory fabric (Paseo plan Phase 5): semantic memory records, kept distinct
-- from the file-based memory_documents/chunks tables and episodic_summaries.
-- Each row is an atomic, scoped fact with provenance and lifecycle metadata so
-- that retrieval can rank by authority/confidence/recency and supersession can
-- be tracked explicitly instead of overwriting history.
--
--   tenant_id     NULL = master/global scope (same convention as api_keys,
--                 node_leases, workspaces); deleting the tenant cascades.
--   workspace_id  Optional link to the workspace (000110) the memory is bound
--                 to; deleting the workspace cascades.
--   user_id /     Owner subject and owning agent. user_id mirrors the
--   agent_id      external-auth VARCHAR(255) identity used by agents.owner_id;
--                 agent_id is an opaque key/uuid string without an FK —
--                 agents are identified by opaque strings on multiple
--                 surfaces.
--   scope         Visibility ring: global | user | agent | workspace |
--                 project | session | thread.
--   kind          Record type: fact | preference | decision | instruction |
--                 constraint | project_context | task_state |
--                 conversation_summary | observation.
--   status        Lifecycle: active -> superseded | archived. supersedes_id /
--                 contradicts_id reference prior rows (ON DELETE SET NULL) to
--                 keep the lineage auditable after a delete.
--   content_hash  SHA-256 hex of normalized content; deduplication upserts by
--                 (tenant, user, agent, workspace, scope) + hash.
CREATE TABLE memories (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id         UUID REFERENCES tenants(id) ON DELETE CASCADE,
    user_id           VARCHAR(255),
    agent_id          TEXT,
    workspace_id      UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    session_key       TEXT,
    scope             TEXT NOT NULL
                      CHECK (scope IN ('global','user','agent','workspace',
                                       'project','session','thread')),
    kind              TEXT NOT NULL
                      CHECK (kind IN ('fact','preference','decision',
                                      'instruction','constraint',
                                      'project_context','task_state',
                                      'conversation_summary','observation')),
    content           TEXT NOT NULL,
    source_type       TEXT NOT NULL DEFAULT 'manual'
                      CHECK (source_type IN ('session','manual',
                                             'consolidation','import')),
    source_ref        TEXT,
    confidence        REAL NOT NULL DEFAULT 0.8
                      CHECK (confidence >= 0 AND confidence <= 1),
    authority         REAL NOT NULL DEFAULT 0.5
                      CHECK (authority >= 0 AND authority <= 1),
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','superseded','archived')),
    supersedes_id     UUID REFERENCES memories(id) ON DELETE SET NULL,
    contradicts_id    UUID REFERENCES memories(id) ON DELETE SET NULL,
    content_hash      VARCHAR(64),
    embedding_version VARCHAR(64),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Dedup/supersede key: content-hash upsert within the same ownership tuple.
CREATE INDEX idx_memories_scope_tuple
    ON memories (tenant_id, user_id, agent_id, workspace_id, scope);

-- Retrieval hard gate: only active rows are ever searched.
CREATE INDEX idx_memories_status_active
    ON memories (status) WHERE status = 'active';

-- Hash lookup during writes; NULL hashes (not yet embedded/normalized) are
-- excluded from the partial index.
CREATE INDEX idx_memories_content_hash
    ON memories (content_hash) WHERE content_hash IS NOT NULL;

-- Lineage walks when resolving a superseded memory back to its replacement.
CREATE INDEX idx_memories_supersedes
    ON memories (supersedes_id) WHERE supersedes_id IS NOT NULL;

-- Recency term of the retrieval score (newest first).
CREATE INDEX idx_memories_updated ON memories (updated_at DESC);
