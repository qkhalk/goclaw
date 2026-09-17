-- 000126: per-account agent permission level (enterprise governance).
-- The web UI is unaffected; this gates what agents may do through the
-- cloud_*/mail_* tools. Legacy rows default to 'read' (the pre-column
-- behavior). Levels: none | read | write | full.
ALTER TABLE cloud_accounts
  ADD COLUMN agent_access TEXT NOT NULL DEFAULT 'read';
