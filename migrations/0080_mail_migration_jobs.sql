-- Copying a mailbox in from another server.
--
-- The work takes minutes to hours and outlives the request that starts it, so
-- the panel answers 202 and the screen polls this row instead of holding a
-- connection open. It also survives a restart: a job left running when the
-- process died is marked failed at startup rather than being reported as still
-- in progress forever.
--
-- There is deliberately NO password column. The remote credential is used to
-- open the IMAP connection and then only lives in the running job; storing it
-- would put a customer's password for ANOTHER provider in this database, where
-- nothing else needs it and a backup would carry it off the machine.
CREATE TABLE mail_migration_jobs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  mailbox_id BIGINT UNSIGNED NOT NULL,
  -- Where the mail is coming from, kept so the screen can show what a finished
  -- or failed job was actually doing without the customer retyping it.
  remote_host VARCHAR(253) NOT NULL,
  remote_port SMALLINT UNSIGNED NOT NULL,
  remote_security ENUM('ssl','starttls','plain') NOT NULL DEFAULT 'ssl',
  remote_user VARCHAR(320) NOT NULL,
  status ENUM('queued','running','done','failed','cancelled') NOT NULL DEFAULT 'queued',
  -- Totals are 0 until the folder list has been read, so a screen must show the
  -- counts as unknown rather than as "0 of 0 finished".
  folders_total INT UNSIGNED NOT NULL DEFAULT 0,
  folders_done INT UNSIGNED NOT NULL DEFAULT 0,
  messages_total INT UNSIGNED NOT NULL DEFAULT 0,
  messages_done INT UNSIGNED NOT NULL DEFAULT 0,
  bytes_done BIGINT UNSIGNED NOT NULL DEFAULT 0,
  -- A stable reason code, never a sentence: the API is English and the panel
  -- ships twelve languages, so the wording belongs to the screen.
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  started_at TIMESTAMP NULL DEFAULT NULL,
  finished_at TIMESTAMP NULL DEFAULT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  -- One unfinished job per mailbox, enforced by the database rather than by a
  -- check the handler runs before inserting: two clicks arriving together would
  -- both pass such a check and start two copies into the same Maildir. The
  -- column is NULL once a job finishes, and NULLs do not collide in a UNIQUE
  -- index, so the history of finished attempts is kept.
  active_mailbox_id BIGINT UNSIGNED
    AS (CASE WHEN status IN ('queued','running') THEN mailbox_id ELSE NULL END) PERSISTENT,
  UNIQUE KEY uq_mail_migration_active (active_mailbox_id),
  KEY ix_mail_migration_mailbox (mailbox_id),
  KEY ix_mail_migration_status (status),
  CONSTRAINT fk_mail_migration_mailbox FOREIGN KEY (mailbox_id) REFERENCES mailboxes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
