-- Global panel settings for custom panel domain and TLS status.
CREATE TABLE IF NOT EXISTS panel_settings (
  id TINYINT UNSIGNED NOT NULL PRIMARY KEY DEFAULT 1,
  custom_domain VARCHAR(255) NULL,
  ssl_status ENUM('none','active','failed') NOT NULL DEFAULT 'none',
  ssl_error TEXT NULL,
  ssl_expires DATE NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO panel_settings (id) SELECT 1 FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM panel_settings);
