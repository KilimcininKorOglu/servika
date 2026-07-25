-- 0026 - rename domain_redis.sk to system_user
-- Runs exactly once (tracked in schema_migrations), so a plain rename is safe.

ALTER TABLE domain_redis CHANGE COLUMN sk system_user VARCHAR(32) NOT NULL;
