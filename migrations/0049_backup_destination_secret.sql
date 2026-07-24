-- 0049_backup_destination_secret.sql
-- Widen backup_destinations.password so it can hold AES-GCM ciphertext
-- (base64 of nonce+ciphertext+tag, prefixed) instead of plaintext.
-- Idempotent: MODIFY COLUMN is a no-op when the column is already varchar(512).

ALTER TABLE backup_destinations MODIFY COLUMN password varchar(512) NOT NULL;
