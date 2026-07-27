-- 0054 — ownership and reseller quotas for the multi-user panel (WHM/cPanel model).
--
-- This migration ONLY prepares the schema; the authorization logic does not yet
-- read these columns (see Phase 5D). Existing single-user behaviour is unchanged:
-- the columns default to NULL, and NULL means "unowned / belongs directly to admin".

-- Ownership chain: domain -> customer -> (reseller user | customer account)
--   owner_user_id : which RESELLER manages this customer (NULL = directly admin)
--   user_id       : this customer's PANEL LOGIN account (NULL = no login account;
--                   the customer still uses the legacy FTP-identity /cp path)
ALTER TABLE customers
  ADD COLUMN owner_user_id BIGINT UNSIGNED NULL,
  ADD COLUMN user_id       BIGINT UNSIGNED NULL;

-- Scope queries pivot on these two columns; without an index every request would
-- be a full table scan.
ALTER TABLE customers
  ADD KEY ix_customer_owner (owner_user_id),
  ADD KEY ix_customer_user  (user_id);

-- Reseller limits (the equivalent of WHM's "reseller limits"). 0 = unlimited —
-- the same contract as the existing quota pattern in service_plans.
CREATE TABLE reseller_limits (
  user_id          BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  max_customer     INT    NOT NULL DEFAULT 0,
  max_domain       INT    NOT NULL DEFAULT 0,
  disk_quota_mb    BIGINT NOT NULL DEFAULT 0,
  traffic_quota_mb BIGINT NOT NULL DEFAULT 0,
  -- Plan IDs the reseller may assign to its customers (JSON array).
  -- NULL = no restriction (may assign any plan).
  allowed_plans    JSON NULL,
  created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_reseller_limits_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB;
