-- 0035_plan_db_governor.sql - Adds native MariaDB resource limits to service plans.
-- Zero means unlimited; db_max_query_seconds controls slow-query termination.
ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS db_max_queries_per_hour INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS db_max_updates_per_hour INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS db_max_query_seconds INT NOT NULL DEFAULT 0;
