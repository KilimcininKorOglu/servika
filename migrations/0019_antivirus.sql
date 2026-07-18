-- Antivirus / malware scan results
CREATE TABLE IF NOT EXISTS av_scans (
  id INT AUTO_INCREMENT PRIMARY KEY,
  domain_id INT NOT NULL,
  started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  finished_at TIMESTAMP NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'running',
  scanned INT NOT NULL DEFAULT 0,
  infected INT NOT NULL DEFAULT 0,
  engine VARCHAR(48) NOT NULL DEFAULT '',
  KEY ix_antivirus_scan_domain (domain_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS av_findings (
  id INT AUTO_INCREMENT PRIMARY KEY,
  scan_id INT NOT NULL,
  domain_id INT NOT NULL,
  file VARCHAR(512) NOT NULL,
  signature VARCHAR(255) NOT NULL,
  engine VARCHAR(32) NOT NULL DEFAULT '',
  quarantined TINYINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  KEY ix_antivirus_finding_scan (scan_id),
  KEY ix_antivirus_finding_domain (domain_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
