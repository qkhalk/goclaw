-- Restore the Wave 1 decision vocabulary. 'ask' rows (only written while the
-- v2 audit vocabulary was active) map to 'block', matching the pipeline
-- outcome ask degraded to before the approval engine existed. 'defer' maps
-- the same way.
UPDATE hook_executions SET decision = 'block' WHERE decision IN ('ask', 'defer');
ALTER TABLE hook_executions DROP CONSTRAINT IF EXISTS hook_executions_decision_check;
ALTER TABLE hook_executions ADD CONSTRAINT hook_executions_decision_check
    CHECK (decision IN ('allow', 'block', 'error', 'timeout'));
