-- 0052_ftp_password_enc.sql
-- Adds an AES-256-GCM encrypted copy of the FTP password. password_md5 now holds
-- a SHA-512-crypt ($6$) hash for Pure-FTPd (MYSQLCrypt=crypt) and customer login,
-- which cannot be reversed; ftp_password_enc preserves the reversible value needed
-- by the password-reveal endpoint and the SSH password sync.
-- Idempotent: ADD COLUMN (MariaDB). No-op when already present.

ALTER TABLE ftp_accounts ADD COLUMN ftp_password_enc text NULL;
