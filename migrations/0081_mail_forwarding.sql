-- Per-mailbox forwarding.
--
-- The panel already had two ways to send mail elsewhere: a domain alias, which
-- has no mailbox behind it, and a Sieve filter, which needs a condition. Neither
-- expresses "send everything that arrives here on, and say whether a copy
-- stays", which is what a customer leaving one address for another is asking
-- for, and what every other panel calls forwarding.
CREATE TABLE mail_forwarding (
  mailbox_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  -- Comma-separated addresses, validated before they are written. A row exists
  -- only while forwarding is on, so there is no enabled flag to disagree with an
  -- empty list.
  destinations VARCHAR(1024) NOT NULL,
  -- Off by default: forwarding that silently stops filling the mailbox is how a
  -- customer discovers years later that nothing was ever kept.
  keep_copy TINYINT(1) NOT NULL DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_mail_forwarding_mailbox FOREIGN KEY (mailbox_id) REFERENCES mailboxes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
