-- OneDrive (Microsoft) provider support: widen the provider check from
-- google-only. Existing google rows are unaffected; the unique index already
-- keys on (tenant, user, provider, email) so nothing else changes.
ALTER TABLE cloud_accounts DROP CONSTRAINT cloud_accounts_provider_check;
ALTER TABLE cloud_accounts ADD CONSTRAINT cloud_accounts_provider_check
    CHECK (provider IN ('google', 'onedrive'));
