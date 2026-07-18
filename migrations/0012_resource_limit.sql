-- 0012 - service_plans resource limit columns (cgroups + xfs_quota + MySQL)
-- Idempotent: MariaDB 10.5+ supports ADD COLUMN IF NOT EXISTS.

ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS cpu_percent           INT NOT NULL DEFAULT 100,
  ADD COLUMN IF NOT EXISTS ram_mb              INT NOT NULL DEFAULT 512,
  ADD COLUMN IF NOT EXISTS max_process         INT NOT NULL DEFAULT 50,
  ADD COLUMN IF NOT EXISTS inode_quota          INT NOT NULL DEFAULT 50000,
  ADD COLUMN IF NOT EXISTS io_weight          INT NOT NULL DEFAULT 100,
  ADD COLUMN IF NOT EXISTS mysql_max_connections  INT NOT NULL DEFAULT 25;
