-- 0014 - nginx FastCGI cache + browser cache toggle (idempotent)
ALTER TABLE nginx_settings
  ADD COLUMN IF NOT EXISTS fastcgi_cache TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS fastcgi_cache_minutes INT NOT NULL DEFAULT 60,
  ADD COLUMN IF NOT EXISTS browser_cache TINYINT(1) NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS browser_cache_days INT NOT NULL DEFAULT 30;
