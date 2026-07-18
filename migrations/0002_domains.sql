-- 0002 — domains table (persistent, test data loaded as seed)
-- NOTE: ssl_enabled instead of ssl (MariaDB reserved word)

CREATE TABLE IF NOT EXISTS domains (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_name         VARCHAR(253) NOT NULL UNIQUE,
  system_user VARCHAR(64)  NOT NULL,
  php_version        VARCHAR(8)   NOT NULL DEFAULT '8.3',
  ssl_enabled        TINYINT(1)   NOT NULL DEFAULT 0,
  ssl_expiry        DATE         NULL,
  status            ENUM('active','passive') NOT NULL DEFAULT 'active',
  ipv4             VARCHAR(45)  NOT NULL DEFAULT '',
  ftp_host         VARCHAR(253) NOT NULL DEFAULT '',
  ftp_user         VARCHAR(64)  NOT NULL DEFAULT '',
  db_host          VARCHAR(64)  NOT NULL DEFAULT 'localhost',
  db_user          VARCHAR(64)  NOT NULL DEFAULT '',
  db_name           VARCHAR(64)  NOT NULL DEFAULT '',
  web_root         VARCHAR(255) NOT NULL DEFAULT '',
  size_kb         BIGINT       NOT NULL DEFAULT 0,
  traffic_kb        BIGINT       NOT NULL DEFAULT 0,
  is_demo          TINYINT(1)   NOT NULL DEFAULT 0,
  notes           TEXT         NULL,
  created_at      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
  KEY ix_domains_status (status),
  KEY ix_domains_system_user (system_user)
) ENGINE=InnoDB;
