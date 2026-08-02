-- Backup and restore jobs. A job groups one bulk operation (nightly scheduler run,
-- multi-domain manual backup, multi-domain restore) so the panel can show a single
-- row with live progress instead of unrelated per-domain records.
--
-- status: running -> done (no failures) | partial (some failed) | failed (all failed)
-- detail: JSON array of per-domain restore results; only used by restore jobs, because
-- backup jobs are already itemized through backups.job_id.
CREATE TABLE backup_jobs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  type VARCHAR(16) NOT NULL DEFAULT 'manual',
  operation VARCHAR(16) NOT NULL DEFAULT 'backup',
  status VARCHAR(16) NOT NULL DEFAULT 'running',
  total INT NOT NULL DEFAULT 0,
  completed INT NOT NULL DEFAULT 0,
  succeeded INT NOT NULL DEFAULT 0,
  failed INT NOT NULL DEFAULT 0,
  size_b BIGINT NOT NULL DEFAULT 0,
  active_domain VARCHAR(255) NOT NULL DEFAULT '',
  restore_mode VARCHAR(16) NOT NULL DEFAULT '',
  started_by VARCHAR(64) NOT NULL DEFAULT '',
  detail MEDIUMTEXT,
  started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TIMESTAMP NULL,
  KEY ix_backup_jobs_open (operation, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Links each archive to the job that produced it, so a job detail page can list its
-- own backups. NULL for single-domain backups taken outside a job.
ALTER TABLE backups ADD COLUMN job_id BIGINT UNSIGNED NULL;
CREATE INDEX ix_backups_job ON backups (job_id);
