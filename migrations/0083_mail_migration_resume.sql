-- Let a mailbox migration survive a restart.
--
-- 0080 deliberately left this table without a password column, and until the
-- work was queued that was the right call: the credential only had to live as
-- long as the request that used it. Queuing changed the shape. A job can now sit
-- waiting behind three others for hours, and holding it in memory alone meant a
-- panel restart threw away every waiting AND every running copy, however far it
-- had got.
--
-- The objection to a column was that this is the customer's password for ANOTHER
-- provider. That is true, and it is also true of source_password in
-- migration_jobs (0067) and of every stored backup destination, FTP account and
-- GitHub token in this panel. The answer is the same one those use rather than a
-- second, weaker rule: internal/secret seals it with AES-256-GCM under
-- SERVIKA_SECRET_KEY, which lives in the environment and not in this database,
-- so a stolen dump alone decrypts nothing. The additional authenticated data is
-- the remote host, so a row edited to point at another server stops decrypting
-- instead of replaying the password at it.
--
-- It is cleared the moment the job reaches a terminal state, exactly as
-- migration_jobs does, so the ciphertext exists only while a copy is actually
-- pending or running.
ALTER TABLE mail_migration_jobs
  ADD COLUMN remote_password VARCHAR(1024) NULL AFTER remote_user,
  ADD COLUMN credentials_cleared TINYINT(1) NOT NULL DEFAULT 0 AFTER remote_password;

-- Rows that predate this column have no credential to clear, and marking them
-- keeps "cleared = 0" meaning "a live secret is here" rather than "unknown".
UPDATE mail_migration_jobs SET credentials_cleared = 1
 WHERE status IN ('done', 'failed', 'cancelled');
