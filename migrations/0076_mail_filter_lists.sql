-- Server-wide sender allow and block lists.
--
-- Rspamd scores every message, and a score is the right answer most of the time.
-- What it cannot do is take an instruction: a supplier whose mail keeps landing
-- in spam, or a sender an operator has decided never to accept again, had no
-- expression in the panel at all.
CREATE TABLE mail_filter_list (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  kind ENUM('allow','block') NOT NULL,
  -- What the value is matched against. Separate maps in Rspamd, so the type has
  -- to be stored rather than guessed from the shape of the value.
  match_type ENUM('address','domain','ip') NOT NULL,
  value VARCHAR(320) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  -- The same value in the same list twice is not two rules. Uniqueness is per
  -- kind and type, so an address may legitimately be allowed while its whole
  -- domain is blocked.
  UNIQUE KEY ux_mail_filter (kind, match_type, value)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
