-- 0037 - Per-tenant XFS user quota: domain-level disk + inode quota OVERRIDE.
-- CloudLinux parity: disk + inode quotas are enforced via XFS *user* quota per tenant
-- (c_<system_user>). Files are already owned c_<system_user>:c_<system_user> so user
-- quota maps exactly; tenants cannot chown out of it, providing escape protection.
--
-- service_plans already carries disk_quota_mb (0006) + inode_quota (0011) as plan-level
-- defaults. This migration adds per-domain OVERRIDE columns: 0 = inherit from plan
-- (no override). Effective quota = domain override (>0) > plan value > (no plan)
-- default 5120 MB / 500000 inodes.
--
-- Idempotent: MariaDB 10.5+ supports ADD COLUMN IF NOT EXISTS; safe on every startup.

ALTER TABLE domains
  ADD COLUMN IF NOT EXISTS disk_quota_mb INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS inode_quota   INT NOT NULL DEFAULT 0;
