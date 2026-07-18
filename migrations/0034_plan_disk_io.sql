-- 0034_plan_disk_io.sql - Adds absolute disk bandwidth and IOPS limits to service plans.
-- Zero means unlimited; IOWeight remains the relative-priority control.
ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS io_read_mbps INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS io_write_mbps INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS io_read_iops INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS io_write_iops INT NOT NULL DEFAULT 0;
