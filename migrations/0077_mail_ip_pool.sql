-- Outbound mail addresses.
--
-- Every domain on the server sent from the same address, so one customer's
-- reputation was every customer's reputation: a single spam run put the whole
-- machine on a blocklist and there was nothing an operator could move a
-- complaining customer onto.
CREATE TABLE mail_ip_pool (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  ip VARCHAR(45) NOT NULL UNIQUE,
  -- Disabled addresses stay in the table with their scan history rather than
  -- being deleted, so an operator can take one out of rotation while a
  -- blocklist delisting is pending and put it back afterwards.
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  note VARCHAR(255) NOT NULL DEFAULT '',
  -- Reverse DNS as it was last measured. An address whose PTR does not
  -- forward-confirm is refused by most large providers, so this is recorded at
  -- the moment it is added rather than discovered when mail starts bouncing.
  ptr_name VARCHAR(253) NOT NULL DEFAULT '',
  ptr_ok TINYINT(1) NOT NULL DEFAULT 0,
  -- Blocklist state from the last background scan. NULL last_scan_at means the
  -- address has never been scanned, which is different from "scanned and clean".
  dnsbl_listed TINYINT(1) NOT NULL DEFAULT 0,
  dnsbl_zones VARCHAR(512) NOT NULL DEFAULT '',
  last_scan_at DATETIME NULL DEFAULT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- A domain sends from the pool address named here. Empty means the server's
-- default address, which is what every domain did before this column existed.
ALTER TABLE mail_domains
  ADD COLUMN outbound_ip VARCHAR(45) NOT NULL DEFAULT '';
