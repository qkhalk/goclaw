-- Skill review/curation lifecycle (Phase 3 W1).
--
-- Extends skills.status with a review-state machine. status is a plain
-- VARCHAR(20) (not an enum), so the new states are introduced as TEXT
-- constants validated in the store layer — no ALTER TYPE / DDL lock.
--
-- Status lifecycle:
--   draft          lands here on create/upload/publish (owner-only)
--   pending_review owner submitted for admin review (public skills)
--   approved       admin approved (awaiting publish)
--   published      discoverable by agents (replaces the legacy 'active')
--   rejected       admin rejected with review_note
--   suspended      admin pulled from discovery (reversible)
--
-- Legacy rows: 'active' -> 'published' so all existing skills remain
-- discoverable. 'archived' (missing deps) and 'deleted' stay unchanged.
ALTER TABLE skills
    ADD COLUMN IF NOT EXISTS reviewed_by  UUID,
    ADD COLUMN IF NOT EXISTS reviewed_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS review_note  TEXT;

UPDATE skills SET status = 'published' WHERE status = 'active';

-- Discovery index now targets 'published' instead of 'active'.
DROP INDEX IF EXISTS idx_skills_visibility;
CREATE INDEX idx_skills_visibility ON skills(visibility) WHERE status = 'published';

-- Review queue lookup: tenant-scoped pending/approved lists for the curator UI.
CREATE INDEX IF NOT EXISTS idx_skills_review_queue
    ON skills(tenant_id, status)
    WHERE status IN ('pending_review', 'approved', 'rejected', 'suspended', 'draft');