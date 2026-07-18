-- 0007 - DNS records (per-domain zone template)
CREATE TABLE IF NOT EXISTS dns_records (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(253) NOT NULL,             -- "@", "www", "mail" etc. (record name)
  type VARCHAR(16) NOT NULL,             -- A, AAAA, CNAME, MX, TXT, NS, SRV
  value TEXT NOT NULL,
  ttl INT NOT NULL DEFAULT 3600,
  priority INT NOT NULL DEFAULT 0,       -- for MX/SRV
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY ix_dns_domain (domain_id),
  KEY ix_dns_type (type),
  CONSTRAINT fk_dns_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;
