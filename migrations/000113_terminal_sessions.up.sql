-- Terminal session metadata (Paseo plan Phase 4 / §25): one row per terminal
-- tab. Only metadata + lifecycle is durable; raw PTY output lives in an
-- in-memory ring buffer and is never persisted.
--
--   tenant_id     NULL = master/global scope (same convention as api_keys,
--                 node_leases, workspaces); deleting the tenant cascades.
--   user_id       Owning external-auth identity (mirrors agents.owner_id);
--                 ownership gates attach/input/close.
--   workspace_id  Workspace (000110) the terminal runs in; deleting the
--                 workspace cascades.
--   cwd / shell   Working directory (inside the workspace root) and the shell
--                 binary that was resolved at spawn time.
--   status        Lifecycle: running -> exited (shell died) | closed (user or
--                 admin closed the tab). exit_code is stamped when the shell
--                 process exits and is otherwise NULL.
CREATE TABLE terminal_sessions (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id    UUID REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      VARCHAR(255) NOT NULL,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    cwd          TEXT NOT NULL DEFAULT '',
    shell        TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'running'
                 CHECK (status IN ('running','exited','closed')),
    exit_code    INT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_terminal_sessions_tenant_user
    ON terminal_sessions (tenant_id, user_id, status);

CREATE INDEX idx_terminal_sessions_workspace
    ON terminal_sessions (workspace_id);

CREATE INDEX idx_terminal_sessions_updated ON terminal_sessions (updated_at DESC);
