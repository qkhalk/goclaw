-- 000054 — Add `name` column to agent_hooks.
--
-- User-facing label for hooks. Nullable so existing rows don't break.
-- Builtin hooks set name from the YAML spec at seed time.

ALTER TABLE agent_hooks ADD COLUMN IF NOT EXISTS name VARCHAR(255);
