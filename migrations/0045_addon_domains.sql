ALTER TABLE domains ADD COLUMN parent_domain_id BIGINT UNSIGNED NULL;
ALTER TABLE domains ADD COLUMN parked TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE domains ADD INDEX ix_domains_parent (parent_domain_id);
