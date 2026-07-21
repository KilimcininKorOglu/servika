-- 0040 TOTP replay protection: the totp_last_step column tracks the most recently
-- accepted 30-second time-step so the same code cannot be reused within the window.
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_step BIGINT NOT NULL DEFAULT 0 AFTER totp_secret;
