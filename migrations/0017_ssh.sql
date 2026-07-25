-- 0017 - host-based SSH access (per-domain shell toggle)
ALTER TABLE domains
  ADD COLUMN ssh_access TINYINT(1) NOT NULL DEFAULT 0;
