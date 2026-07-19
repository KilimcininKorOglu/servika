-- 0038 - WAF (ModSecurity v3 + OWASP CRS) plan + domain settings
--
-- WAF is enabled per-plan and per-domain with overrides. Module loading is
-- global but HARMLESS: no behavior changes until a vhost says "modsecurity on"
-- (per-domain opt-in). This migration only adds settings columns; the module + CRS
-- installation is handled by the ops script (servika-waf-setup).
--
-- SEMANTICS:
--   service_plans (plan defaults, NOT NULL):
--     waf_enabled  0/1   (0 = plan starts with WAF off; safe default)
--     waf_mode     'on' | 'detect' | 'off'   ('on' = block, 'detect' = log only)
--     waf_paranoia 1..4  (CRS paranoia level; higher = stricter + more false positives)
--   domains (per-domain OVERRIDE, NULL = inherit from plan):
--     waf_enabled  NULL=inherit / 0=off / 1=on
--     waf_mode     NULL=inherit / 'on' / 'detect' / 'off'
--     waf_paranoia NULL or 0 = inherit / 1..4 = override
--
-- Effective value = domain override (non-NULL/non-0) > plan value > (no plan) off.
--
-- Idempotent: MariaDB 10.5+ supports ADD COLUMN IF NOT EXISTS; safe on every startup.

ALTER TABLE service_plans
  ADD COLUMN IF NOT EXISTS waf_enabled  TINYINT     NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS waf_mode     VARCHAR(10) NOT NULL DEFAULT 'on',
  ADD COLUMN IF NOT EXISTS waf_paranoia TINYINT     NOT NULL DEFAULT 1;

ALTER TABLE domains
  ADD COLUMN IF NOT EXISTS waf_enabled  TINYINT     NULL DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS waf_mode     VARCHAR(10) NULL DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS waf_paranoia TINYINT     NULL DEFAULT NULL;
