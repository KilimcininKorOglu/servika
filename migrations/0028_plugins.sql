-- Plugin registry for out-of-process services connected through Unix sockets.

CREATE TABLE IF NOT EXISTS plugins (
  id           INT AUTO_INCREMENT PRIMARY KEY,
  name         VARCHAR(64)  NOT NULL,
  label        VARCHAR(128) NOT NULL,
  version      VARCHAR(32)  NOT NULL DEFAULT '',
  enabled      TINYINT(1)   NOT NULL DEFAULT 0,
  socket       VARCHAR(255) NOT NULL DEFAULT '',
  ui           TINYINT(1)   NOT NULL DEFAULT 0,
  health       VARCHAR(16)  NOT NULL DEFAULT 'unknown',
  last_checked DATETIME     NULL,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_plugins_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
