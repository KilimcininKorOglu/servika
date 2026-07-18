-- 0024_schema_drift_fix.sql
-- Adds schema elements that were previously introduced without a migration.
-- Idempotent: CREATE TABLE IF NOT EXISTS + ADD COLUMN IF NOT EXISTS (MariaDB).
-- No-op when every required schema element already exists.

-- ── domains: web backend + backup settings ──
ALTER TABLE domains ADD COLUMN IF NOT EXISTS web_backend      varchar(32)  NOT NULL DEFAULT 'php-fpm';
ALTER TABLE domains ADD COLUMN IF NOT EXISTS backup_freq      varchar(16)  NOT NULL DEFAULT 'none';
ALTER TABLE domains ADD COLUMN IF NOT EXISTS backup_hour      tinyint(4)   NOT NULL DEFAULT 3;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS backup_retention tinyint(4)   NOT NULL DEFAULT 7;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS last_backup_at   timestamp    NULL DEFAULT NULL;

-- ── backup_destinations (remote backup targets: sftp/ftp) ──
CREATE TABLE IF NOT EXISTS backup_destinations (
  id          bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  domain_id   bigint(20) unsigned NOT NULL,
  type         varchar(8)   NOT NULL DEFAULT 'sftp',
  host        varchar(253) NOT NULL,
  port        int(11)      NOT NULL DEFAULT 22,
  username   varchar(128) NOT NULL,
  password      varchar(255) NOT NULL,
  remote_dir  varchar(255) NOT NULL DEFAULT '/',
  enabled       tinyint(4)   NOT NULL DEFAULT 1,
  last_upload timestamp    NULL DEFAULT NULL,
  last_status   varchar(32)  NOT NULL DEFAULT '',
  last_error    varchar(512) NOT NULL DEFAULT '',
  created_at  timestamp    NULL DEFAULT current_timestamp(),
  updated_at  timestamp    NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (id),
  UNIQUE KEY uq_backup_destination_domain (domain_id),
  CONSTRAINT fk_backup_destination_domain FOREIGN KEY (domain_id) REFERENCES domains (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ── github_connections (Git deploy: PAT + repo/branch + webhook) ──
CREATE TABLE IF NOT EXISTS github_connections (
  id            bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  domain_id     bigint(20) unsigned NOT NULL,
  pat           varchar(255) NOT NULL,
  login         varchar(64)  NOT NULL,
  full_name      varchar(128) NOT NULL DEFAULT '',
  avatar_url    varchar(255) NOT NULL DEFAULT '',
  selected_repo   varchar(255) NOT NULL DEFAULT '',
  selected_branch varchar(64)  NOT NULL DEFAULT '',
  webhook_id    bigint(20)   NOT NULL DEFAULT 0,
  webhook_url   varchar(255) NOT NULL DEFAULT '',
  created_at    timestamp    NULL DEFAULT current_timestamp(),
  updated_at    timestamp    NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (id),
  UNIQUE KEY uq_github_connection_domain (domain_id),
  CONSTRAINT fk_github_connection_domain FOREIGN KEY (domain_id) REFERENCES domains (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
