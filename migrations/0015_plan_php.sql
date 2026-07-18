-- 0015 - service_plans: default PHP version (inherited by new domains)
ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS php_version VARCHAR(8) NOT NULL DEFAULT '8.3';
