-- 0026 - rename domain_redis.sk to system_user (idempotent)
-- Column was already renamed on previous runs; skip if system_user exists.

SET @sk_exists := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='domain_redis' AND COLUMN_NAME='sk');
SET @sqlstmt := IF(@sk_exists > 0, 'ALTER TABLE domain_redis CHANGE COLUMN sk system_user VARCHAR(32) NOT NULL', 'SELECT 1');
PREPARE stmt FROM @sqlstmt;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
