ALTER TABLE approval_requests DROP COLUMN IF EXISTS grant_expires_at;
ALTER TABLE approval_requests DROP COLUMN IF EXISTS args_digest;
ALTER TABLE approval_requests DROP COLUMN IF EXISTS session_key;
