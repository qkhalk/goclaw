-- Revert publisher trust anchors + signed package columns (Phase 3 W2).
ALTER TABLE skills
    DROP COLUMN IF EXISTS signature,
    DROP COLUMN IF EXISTS publisher_id,
    DROP COLUMN IF EXISTS signed_at;

DROP INDEX IF EXISTS idx_publisher_keys_publisher;
DROP TABLE IF EXISTS publisher_keys;