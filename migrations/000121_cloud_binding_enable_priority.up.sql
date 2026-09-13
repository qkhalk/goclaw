-- Cloud binding rules: enable/disable + priority. enabled lets an admin keep
-- a rule configured without it affecting account resolution; priority breaks
-- ties between rules of the same scope tier (lower wins). Legacy rows upgrade
-- to enabled with the default priority so existing behavior is unchanged.
ALTER TABLE cloud_account_bindings ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE cloud_account_bindings ADD COLUMN priority INTEGER NOT NULL DEFAULT 100;
