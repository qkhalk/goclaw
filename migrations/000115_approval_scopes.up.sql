-- Approval Engine v2 (Phase 1): policy scopes + expiry + restart reinstatement.
-- session_key scopes allow-for-session grants to the requesting session and
-- identifies the originating session in the approval queue.
-- args_digest is the canonical sha256 of (tool name, tool arguments); an
-- allow-once grant keyed by it lets a retried/resumed identical call pass
-- without a second prompt.
-- grant_expires_at bounds the lifetime of the granted scope on resolved rows
-- (distinct from expired_at, which bounds how long a request stays resolvable).
ALTER TABLE approval_requests ADD COLUMN session_key TEXT NOT NULL DEFAULT '';
ALTER TABLE approval_requests ADD COLUMN args_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE approval_requests ADD COLUMN grant_expires_at TIMESTAMPTZ;
