-- 0043_custom_vhost.sql, per-domain administrator-managed raw nginx vhost content.
ALTER TABLE domains ADD COLUMN IF NOT EXISTS custom_vhost_enabled TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS custom_vhost_content TEXT NULL;
