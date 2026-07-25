-- 0012 - service_plans resource limit columns (cgroups + xfs_quota + MySQL)
-- Applied exactly once, tracked in schema_migrations.

ALTER TABLE service_plans
  ADD COLUMN cpu_percent           INT NOT NULL DEFAULT 100,
  ADD COLUMN ram_mb              INT NOT NULL DEFAULT 512,
  ADD COLUMN max_process         INT NOT NULL DEFAULT 50,
  ADD COLUMN inode_quota          INT NOT NULL DEFAULT 50000,
  ADD COLUMN io_weight          INT NOT NULL DEFAULT 100,
  ADD COLUMN mysql_max_connections  INT NOT NULL DEFAULT 25;
