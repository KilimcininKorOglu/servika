-- 0014 - nginx FastCGI cache + browser cache toggle
ALTER TABLE nginx_settings
  ADD COLUMN fastcgi_cache TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN fastcgi_cache_minutes INT NOT NULL DEFAULT 60,
  ADD COLUMN browser_cache TINYINT(1) NOT NULL DEFAULT 1,
  ADD COLUMN browser_cache_days INT NOT NULL DEFAULT 30;
