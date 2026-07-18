-- System load (load average) history — periodic sampling for the dashboard chart
CREATE TABLE IF NOT EXISTS system_load (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  load1 FLOAT NOT NULL DEFAULT 0,
  load5 FLOAT NOT NULL DEFAULT 0,
  load15 FLOAT NOT NULL DEFAULT 0,
  mem_percent FLOAT NOT NULL DEFAULT 0,
  KEY ix_system_load_timestamp (ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
