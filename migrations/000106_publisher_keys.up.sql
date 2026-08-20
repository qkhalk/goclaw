-- Publisher trust anchors + signed package columns (Phase 3 W2).
--
-- publisher_keys is the trust anchor for skill signing. A skill manifest is
-- signed with an ed25519 private key; the matching public key must be
-- registered here (fingerprint = hex(sha256(public_key)) computed in Go) or
-- the published skill is rejected on install/import.
--
-- fingerprint is globally UNIQUE: a publisher key is a public-key identity,
-- not a per-tenant row. Publisher identity is expressed through
-- publisher_id/publisher_type on each key row.

CREATE TABLE IF NOT EXISTS publisher_keys (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    publisher_id   UUID        NOT NULL,
    publisher_type TEXT        NOT NULL DEFAULT 'tenant',
    public_key     BYTEA       NOT NULL,
    fingerprint    TEXT        NOT NULL UNIQUE,
    status         TEXT        NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_publisher_keys_publisher
    ON publisher_keys (publisher_id, status);

-- Signed-package metadata on skills. signature covers the skill manifest
-- (see internal/crypto/signature.go); publisher_id links back to the tenant
-- or user that owns the signing key.
ALTER TABLE skills
    ADD COLUMN IF NOT EXISTS signature    BYTEA,
    ADD COLUMN IF NOT EXISTS publisher_id UUID,
    ADD COLUMN IF NOT EXISTS signed_at    TIMESTAMPTZ;