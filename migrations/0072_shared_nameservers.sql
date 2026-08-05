-- 0072_shared_nameservers.sql
-- Move the DNS template from vanity nameservers to a shared nameserver pair.
--
-- BEFORE (wrong model): the template produced, for every customer domain,
--
--   ns1  IN A  {IP}
--   @    IN NS ns1.{DOMAIN}.
--   @    IN NS ns2.{DOMAIN}.
--
-- These are "vanity nameservers". For them to resolve, a separate glue record
-- (child host) must be registered at the registrar of EVERY customer domain,
-- which is not workable for shared hosting. The practical consequence was that
-- no customer could be told which nameservers to point their domain at, so
-- every domain had to be wired up by hand with A records instead.
--
-- AFTER: every zone publishes the panel's SHARED nameserver pair
-- (ns1.provider.example / ns2.provider.example). Glue is then needed exactly
-- once, for the provider's own domain.
--
-- Resellers may publish their own white-label pair; the domains of that
-- reseller's customers then use the reseller's nameservers.

-- Panel-wide default pair. Deliberately NULL until an operator sets it: the
-- pair is never derived from panel_settings.custom_domain, because a panel
-- hosted at a subdomain (cloud.provider.example) would derive
-- ns1.cloud.provider.example, publishing a nameserver the provider does not
-- own and silently breaking every customer domain.
ALTER TABLE panel_settings
  ADD COLUMN ns1_hostname VARCHAR(255) NULL,
  ADD COLUMN ns2_hostname VARCHAR(255) NULL;

-- Per-reseller nameservers (white-label). No row means the panel-wide pair.
CREATE TABLE IF NOT EXISTS reseller_nameservers (
  user_id    BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  ns1        VARCHAR(255) NOT NULL,
  ns2        VARCHAR(255) NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_reseller_ns_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Move the template onto the new model.
--
-- The WHERE conditions are deliberately narrow: only untouched built-in rows
-- are rewritten. If an administrator customized the template (wrote a different
-- value), the row is left exactly as it is and nobody's hand-built setup is
-- rewritten underneath them.
UPDATE dns_template SET value = '{NS1}' WHERE type = 'NS' AND value = 'ns1.{DOMAIN}';
UPDATE dns_template SET value = '{NS2}' WHERE type = 'NS' AND value = 'ns2.{DOMAIN}';

-- ns1/ns2 A records under the CUSTOMER's domain are meaningless in the shared
-- model: the nameserver now lives in the provider's (or reseller's) own domain.
DELETE FROM dns_template WHERE type = 'A' AND name IN ('ns1', 'ns2') AND value = '{IP}';
