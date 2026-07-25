-- 0005 - SSL certificate paths (cert + key)
ALTER TABLE domains
  ADD COLUMN cert_path VARCHAR(255) NOT NULL DEFAULT '',
  ADD COLUMN key_path VARCHAR(255) NOT NULL DEFAULT '',
  ADD COLUMN ssl_source VARCHAR(32) NOT NULL DEFAULT '';
