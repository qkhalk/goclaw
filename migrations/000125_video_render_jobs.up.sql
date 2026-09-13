-- Video render jobs: storyboard-to-MP4 render requests, processed by the
-- videoworker sidecar. Tenant-scoped; user_id is TEXT (GoClaw user IDs are
-- arbitrary strings — see migration 000124).
CREATE TABLE video_render_jobs (
    id                TEXT NOT NULL PRIMARY KEY,
    tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id           TEXT NOT NULL DEFAULT '',
    agent_id          TEXT NOT NULL DEFAULT '',
    session_key       TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'queued',
    engine            TEXT NOT NULL DEFAULT 'ffmpeg',
    storyboard_json   TEXT NOT NULL,
    output_path       TEXT NOT NULL DEFAULT '',
    output_size_bytes BIGINT NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    expires_at        TIMESTAMPTZ
);

CREATE INDEX idx_video_jobs_tenant_created
    ON video_render_jobs (tenant_id, created_at DESC);
CREATE INDEX idx_video_jobs_status
    ON video_render_jobs (status);
CREATE INDEX idx_video_jobs_expires
    ON video_render_jobs (expires_at)
    WHERE status IN ('done', 'failed', 'cancelled');
