-- 0033_plan_fpm_children.sql - Adds plan-driven PHP-FPM worker limits.
-- Zero derives the value as max(4, ram_mb / 64).
ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS pm_max_children INT NOT NULL DEFAULT 0;
