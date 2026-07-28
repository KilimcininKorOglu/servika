-- 0064 - Reseller scope for the security log.
--
-- audit_log.reseller_id records which tenant an entry belongs to:
--     0  -> root (panel owner) action or a global resource.
--     >0 -> the owning reseller's users.id. A reseller sees ONLY its own scope;
--           root sees everything. The entry is scoped to the AFFECTED account's
--           owner, not the actor, so that when root changes a reseller's limits
--           the reseller sees it in its own log (transparency).
--
-- NO BACKFILL: domain names and usernames are reusable identifiers over time, so
-- matching historical rows to a current reseller would leak one tenant's past
-- into another's log. Legacy rows stay at reseller_id=0 (root-only); new writes
-- already carry the correct scope.
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS reseller_id BIGINT UNSIGNED NOT NULL DEFAULT 0;
ALTER TABLE audit_log ADD INDEX IF NOT EXISTS ix_audit_scope (reseller_id, id);
