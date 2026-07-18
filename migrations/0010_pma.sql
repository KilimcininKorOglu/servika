-- 0010 - phpMyAdmin single-use SSO tokens
CREATE TABLE IF NOT EXISTS pma_tokens (
  token VARCHAR(64) NOT NULL PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  db_user VARCHAR(80) NOT NULL,
  db_pass VARCHAR(255) NOT NULL,
  db_name VARCHAR(80) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP NOT NULL,
  used TINYINT(1) NOT NULL DEFAULT 0,
  KEY ix_pma_domain (domain_id),
  CONSTRAINT fk_pma_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;
