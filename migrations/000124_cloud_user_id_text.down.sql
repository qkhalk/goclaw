-- Restore the pre-000124 column types. Only safe while every stored value is
-- a valid UUID (any "system"/numeric user id written since the upgrade makes
-- the cast fail — delete or remap those rows first).

ALTER TABLE cloud_starred ALTER COLUMN user_id TYPE UUID USING user_id::uuid;
ALTER TABLE cloud_sync_pairs ALTER COLUMN created_by TYPE UUID USING NULLIF(created_by, '')::uuid;
