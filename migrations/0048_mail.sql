-- 0048 - native mail hosting metadata for Postfix and Dovecot.
-- The panel stores virtual mail domains and mailboxes in the central database.
-- Postfix and Dovecot can read these rows through live SQL maps, so mailbox
-- create, suspend, and delete operations do not require generated config files.

CREATE TABLE IF NOT EXISTS mail_domains (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL UNIQUE,
  domain_name VARCHAR(253) NOT NULL UNIQUE,
  system_user VARCHAR(64) NOT NULL,
  uid_n INT NOT NULL,
  gid_n INT NOT NULL,
  maildir_root VARCHAR(255) NOT NULL,
  dkim_selector VARCHAR(32) NOT NULL DEFAULT 'default',
  status ENUM('active','suspended') NOT NULL DEFAULT 'active',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  KEY ix_mail_domains_status (status),
  CONSTRAINT fk_mail_domains_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mailboxes (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  mail_domain_id BIGINT UNSIGNED NOT NULL,
  local_part VARCHAR(64) NOT NULL,
  email VARCHAR(320) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  maildir VARCHAR(255) NOT NULL,
  quota_bytes BIGINT UNSIGNED NOT NULL DEFAULT 0,
  status ENUM('active','suspended') NOT NULL DEFAULT 'active',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_mailbox_domain_local (domain_id, local_part),
  KEY ix_mailbox_domain (domain_id),
  KEY ix_mailbox_status (status),
  CONSTRAINT fk_mailbox_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE,
  CONSTRAINT fk_mailbox_maildomain FOREIGN KEY (mail_domain_id) REFERENCES mail_domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mail_aliases (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  source VARCHAR(320) NOT NULL UNIQUE,
  destination TEXT NOT NULL,
  status ENUM('active','suspended') NOT NULL DEFAULT 'active',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  KEY ix_alias_domain (domain_id),
  CONSTRAINT fk_alias_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mail_send_log (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  mailbox_id BIGINT UNSIGNED NOT NULL,
  domain_id BIGINT UNSIGNED NOT NULL,
  ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  ok TINYINT(1) NOT NULL DEFAULT 1,
  KEY ix_sendlog_mailbox_ts (mailbox_id, ts),
  CONSTRAINT fk_sendlog_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE service_plans ADD COLUMN IF NOT EXISTS mailbox_quota_mb INT NOT NULL DEFAULT 0;
