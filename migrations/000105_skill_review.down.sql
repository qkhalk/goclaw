-- Revert skill review/curation lifecycle (Phase 3 W1).
DROP INDEX IF EXISTS idx_skills_review_queue;

-- Restore discovery index to the legacy 'active' predicate.
DROP INDEX IF EXISTS idx_skills_visibility;
CREATE INDEX idx_skills_visibility ON skills(visibility) WHERE status = 'active';

UPDATE skills SET status = 'active' WHERE status = 'published';

ALTER TABLE skills
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS review_note;