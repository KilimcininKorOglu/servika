-- 0062 - Widen db_accounts.db_pass_plain for at-rest encryption.
--
-- The column now stores an AES-256-GCM ciphertext (prefix "enc:v1:" + base64 of
-- nonce||sealed) bound to the database user, instead of the raw password. The
-- encrypted form is longer than the original VARCHAR(255) could hold for a
-- maximum-length password, so widen it. Existing plaintext rows keep working and
-- are re-written encrypted by the startup backfill (credentials.BackfillDBPasswords).
ALTER TABLE db_accounts MODIFY COLUMN db_pass_plain VARCHAR(512) NOT NULL;
