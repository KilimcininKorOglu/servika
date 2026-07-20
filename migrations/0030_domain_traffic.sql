-- 0030 - domain traffic accounting (reserved-word safe)

CREATE TABLE IF NOT EXISTS domain_traffic (
  domain_id BIGINT UNSIGNED NOT NULL,
  `year_month` CHAR(7) NOT NULL,
  bytes BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (domain_id, `year_month`),
  KEY ix_domain_traffic_month (`year_month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS domain_traffic_cursor (
  domain_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  `offset` BIGINT NOT NULL DEFAULT 0,
  `size` BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
