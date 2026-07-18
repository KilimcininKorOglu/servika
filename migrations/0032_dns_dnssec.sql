-- 0032_dns_dnssec.sql - Adds opt-in DNSSEC state for managed domains.
-- Existing domains remain unsigned until DNSSEC is explicitly enabled.
ALTER TABLE domains ADD COLUMN IF NOT EXISTS dnssec_active TINYINT(1) NOT NULL DEFAULT 0;
