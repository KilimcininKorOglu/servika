-- 0050_token_version.sql
-- Adds a per-identity token version so JWT sessions can be revoked without
-- rotating the global signing secret. Incrementing the version invalidates
-- every token previously issued for that identity.
-- Idempotent: ADD COLUMN (MariaDB). No-op when already present.

ALTER TABLE users        ADD COLUMN token_version int unsigned NOT NULL DEFAULT 0;
ALTER TABLE ftp_accounts ADD COLUMN token_version int unsigned NOT NULL DEFAULT 0;
