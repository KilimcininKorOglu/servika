-- One-time tokens for opening webmail as a mailbox.
--
-- The panel holds only a SHA512-CRYPT hash of a mailbox password, so it cannot
-- log a customer in by replaying their password. Roundcube authenticates through
-- Dovecot's master user instead, and this table is what carries that credential
-- from the panel to Roundcube: once, briefly, in a POST body, never in a URL.
CREATE TABLE webmail_tokens (
  token VARCHAR(64) NOT NULL PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  mailbox_id BIGINT UNSIGNED NOT NULL,
  email VARCHAR(320) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP NOT NULL,
  used TINYINT(1) NOT NULL DEFAULT 0,
  KEY ix_webmail_domain (domain_id),
  CONSTRAINT fk_webmail_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
