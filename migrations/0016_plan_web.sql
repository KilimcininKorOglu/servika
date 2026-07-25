-- 0016 - service_plans: nginx web-server defaults (inherited by new domains)
ALTER TABLE service_plans
  ADD COLUMN fastcgi_cache TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN client_max_body_mb INT NOT NULL DEFAULT 64,
  ADD COLUMN nginx_extra_directives TEXT NULL;
