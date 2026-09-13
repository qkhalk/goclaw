-- GoClaw user IDs are arbitrary strings ("system", telegram numeric ids,
-- agent keys) — not UUIDs. cloud_starred.user_id and
-- cloud_sync_pairs.created_by were declared UUID, but every query binds a
-- string user id, so any non-uuid user ("system") failed with
-- `invalid input syntax for type uuid` (SQLSTATE 22P02) — e.g. listing
-- starred items for the system user. cloud_accounts.user_id (000118) was
-- already TEXT; this aligns the newer tables. tenant_id / account ids stay
-- UUID (they are real uuid FKs / row ids).

ALTER TABLE cloud_starred ALTER COLUMN user_id TYPE TEXT;
ALTER TABLE cloud_sync_pairs ALTER COLUMN created_by TYPE TEXT;
