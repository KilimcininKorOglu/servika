-- Subdomains — separate docroot + vhost under the parent domain's user
CREATE TABLE IF NOT EXISTS subdomains (
  id INT AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  subdomain VARCHAR(63) NOT NULL,
  fqdn VARCHAR(253) NOT NULL,
  php_version VARCHAR(8) NOT NULL DEFAULT '8.3',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_subdomain_fqdn (fqdn),
  KEY ix_subdomain_domain (domain_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
