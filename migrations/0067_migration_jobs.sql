-- Site migration from cPanel / Plesk / DirectAdmin installations.
--
-- Two tables:
--   migration_jobs  : one migration session (source server + credentials + settings + progress)
--   migration_items : every account/domain inside that session (single or bulk)
--
-- Credentials are NEVER stored in plain text. internal/secret (AES-256-GCM)
-- encrypts them at rest and the job clears them when it finishes
-- (credentials_cleared = 1).

CREATE TABLE IF NOT EXISTS migration_jobs (
  id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  source_type         ENUM('cpanel','plesk','directadmin') NOT NULL,
  source_host         VARCHAR(253) NOT NULL,
  source_port         INT NOT NULL DEFAULT 22,
  source_user         VARCHAR(64) NOT NULL DEFAULT 'root',
  source_password     VARCHAR(1024) NULL,
  source_key          TEXT NULL,
  credentials_cleared TINYINT(1) NOT NULL DEFAULT 0,
  mode                ENUM('single','bulk') NOT NULL DEFAULT 'single',
  status              ENUM('pending','discovery','running','done','failed','cancelled','interrupted')
                        NOT NULL DEFAULT 'pending',
  total               INT NOT NULL DEFAULT 0,
  completed           INT NOT NULL DEFAULT 0,
  failed              INT NOT NULL DEFAULT 0,
  settings            TEXT NULL,
  error_text          TEXT NULL,
  started_by          VARCHAR(64) NULL,
  started_at          DATETIME NULL,
  finished_at         DATETIME NULL,
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY ix_migration_jobs_status (status),
  KEY ix_migration_jobs_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS migration_items (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  job_id         BIGINT UNSIGNED NOT NULL,
  source_account VARCHAR(64) NOT NULL,
  domain_name    VARCHAR(253) NOT NULL,
  status         ENUM('pending','running','done','failed','skipped') NOT NULL DEFAULT 'pending',
  domain_id      BIGINT UNSIGNED NULL,
  file_bytes     BIGINT NOT NULL DEFAULT 0,
  db_count       INT NOT NULL DEFAULT 0,
  dns_count      INT NOT NULL DEFAULT 0,
  error_text     TEXT NULL,
  started_at     DATETIME NULL,
  finished_at    DATETIME NULL,
  PRIMARY KEY (id),
  KEY ix_migration_items_job (job_id),
  KEY ix_migration_items_status (status),
  CONSTRAINT fk_migration_items_job FOREIGN KEY (job_id)
    REFERENCES migration_jobs(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
