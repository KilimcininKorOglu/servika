-- 0051_github_pat_encrypt.sql
-- Widen github_connections.pat so it can hold AES-GCM ciphertext (base64 of
-- nonce+ciphertext+tag, prefixed) instead of a plaintext token.
-- Applied exactly once, tracked in schema_migrations.

ALTER TABLE github_connections MODIFY COLUMN pat varchar(512) NOT NULL;
