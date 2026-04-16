-- 000054 down — Remove name column from agent_hooks.
ALTER TABLE agent_hooks DROP COLUMN IF EXISTS name;
