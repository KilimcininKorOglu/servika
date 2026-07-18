-- Password-protected directories (.htpasswd / nginx auth_basic)
CREATE TABLE IF NOT EXISTS protected_directories (
  id INT AUTO_INCREMENT PRIMARY KEY,
  domain_id INT NOT NULL,
  path VARCHAR(255) NOT NULL,
  username VARCHAR(64) NOT NULL,
  htpasswd_file VARCHAR(255) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_protected_directory (domain_id, path, username),
  KEY ix_protected_directory_domain (domain_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
