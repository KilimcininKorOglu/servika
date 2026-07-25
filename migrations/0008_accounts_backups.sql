-- 0008 - Customers + domains.customer_id + backups
-- NOTE: resellers structure removed in 0012 (see sprint note).
CREATE TABLE customers (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  email VARCHAR(255) NOT NULL,
  plan_id BIGINT UNSIGNED NULL,
  status ENUM('active','passive') NOT NULL DEFAULT 'active',
  notes VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  KEY ix_customer_plan (plan_id)
) ENGINE=InnoDB;

ALTER TABLE domains
  ADD COLUMN customer_id BIGINT UNSIGNED NULL,
  ADD KEY ix_domains_customer (customer_id);

CREATE TABLE backups (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  type VARCHAR(16) NOT NULL DEFAULT 'full',
  file VARCHAR(255) NOT NULL,
  size_b BIGINT NOT NULL DEFAULT 0,
  notes VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  KEY ix_backup_domain (domain_id),
  CONSTRAINT fk_backup_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;
