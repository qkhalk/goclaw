-- Agent jobs + task graph (Paseo plan Phase 2): durable execution records kept
-- separate from sessions. agent_jobs tracks the execution lifecycle of a unit
-- of work (run / delegation / consolidation); hot state lives in memory /
-- agent_runs, only the persisted subset lands here.
--
--   tenant_id     NULL = master/global scope (same convention as api_keys,
--                 node_leases, workspaces); deleting the tenant cascades.
--   workspace_id  Optional link to the sandboxed workspace (000110) the job
--                 runs in; deleting the workspace keeps the job record but
--                 detaches it (ON DELETE SET NULL).
--   session_key   Conversation the job belongs to; '' for headless jobs.
--   agent_id      Owning agent identifier (key/uuid string across surfaces),
--                 deliberately without an FK — agents are identified by
--                 opaque strings on multiple surfaces.
--   status        Lifecycle: queued -> starting -> running, optionally through
--                 waiting_input / waiting_approval / paused, ending terminal
--                 in completed | failed | cancelled.
--
-- task_graph holds the hierarchical task tree for delegated planning within a
-- workspace: parent/child edges self-reference with ON DELETE CASCADE, and
-- depends_on is a JSON array of task ids expressing ordering constraints.
CREATE TABLE agent_jobs (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id    UUID REFERENCES tenants(id) ON DELETE CASCADE,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE SET NULL,
    session_key  TEXT NOT NULL DEFAULT '',
    agent_id     TEXT,
    kind         TEXT NOT NULL
                 CHECK (kind IN ('run','delegation','consolidation')),
    status       TEXT NOT NULL DEFAULT 'queued'
                 CHECK (status IN ('queued','starting','running','waiting_input',
                                   'waiting_approval','paused','completed',
                                   'failed','cancelled')),
    priority     INT NOT NULL DEFAULT 0,
    title        TEXT NOT NULL DEFAULT '',
    result_ref   TEXT,
    error        TEXT,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Tenant/workspace/status listing (jobs.list filter combinations).
CREATE INDEX idx_agent_jobs_tenant_ws_status
    ON agent_jobs (tenant_id, workspace_id, status);

-- Per-conversation history lookup; empty session_key (headless jobs) is not
-- worth indexing.
CREATE INDEX idx_agent_jobs_session
    ON agent_jobs (session_key) WHERE session_key <> '';

-- Scheduler queue scan: next actionable jobs by priority, newest first;
-- terminal rows drop out of the partial index entirely.
CREATE INDEX idx_agent_jobs_active
    ON agent_jobs (priority DESC, created_at DESC)
    WHERE status NOT IN ('completed','failed','cancelled');

CREATE TABLE task_graph (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id      UUID REFERENCES tenants(id) ON DELETE CASCADE,
    workspace_id   UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    parent_id      UUID REFERENCES task_graph(id) ON DELETE CASCADE,
    owner_agent_id TEXT,
    session_key    TEXT,
    title          TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','running','blocked','done',
                                     'failed','cancelled')),
    priority       INT NOT NULL DEFAULT 0,
    depends_on     JSONB NOT NULL DEFAULT '[]'::jsonb,
    result_ref     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Whole-tree listing scoped to a workspace (tasks.tree).
CREATE INDEX idx_task_graph_tenant_ws
    ON task_graph (tenant_id, workspace_id);

-- Child lookup under a parent node.
CREATE INDEX idx_task_graph_parent ON task_graph (parent_id);

-- Recency ordering for dashboards (newest activity first).
CREATE INDEX idx_task_graph_updated ON task_graph (updated_at DESC);
