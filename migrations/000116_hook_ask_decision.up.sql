-- Approval Engine v2 (Phase 1): the script handler now passes the hook's
-- 'ask' decision through to the dispatcher (which routes it to the approval
-- engine or degrades to block), and the audit writer records the hook's true
-- decision. Extend the hook_executions.decision check vocabulary accordingly.
ALTER TABLE hook_executions DROP CONSTRAINT IF EXISTS hook_executions_decision_check;
ALTER TABLE hook_executions ADD CONSTRAINT hook_executions_decision_check
    CHECK (decision IN ('allow', 'block', 'error', 'timeout', 'ask', 'defer'));
