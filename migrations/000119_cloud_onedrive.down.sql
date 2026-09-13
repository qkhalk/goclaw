-- Revert to google-only. Fails if onedrive rows exist (delete them first).
ALTER TABLE cloud_accounts DROP CONSTRAINT cloud_accounts_provider_check;
ALTER TABLE cloud_accounts ADD CONSTRAINT cloud_accounts_provider_check
    CHECK (provider IN ('google'));
