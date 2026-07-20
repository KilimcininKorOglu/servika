-- 0039 - per-domain PHP debug mode (robust fatal error visibility)
--
-- Adds debug_mode to php_settings. When a customer enables "PHP Debug Mode",
-- fatal errors (E_ERROR/E_PARSE/...) are reliably caught via
-- register_shutdown_function + error_get_last() and written to both the screen
-- and a per-domain debug log file.
--
-- CRITICAL: If the application runtime calls error_reporting(0), the pool's
-- php_admin_value[display_errors/error_reporting] CANNOT override that. The
-- only reliable way to surface fatals is an auto_prepend-loaded shutdown-handler
-- shim; when debug_mode=1, renderTenantPool enables this shim (auto_prepend_file).
--
-- SEMANTICS (php_settings, per-domain):
--   debug_mode 0 = off (display_errors=off, error_reporting = user setting)
--   debug_mode 1 = on  (display_errors=on + error_reporting=E_ALL + auto_prepend shim)

SET @exist := (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='php_settings' AND COLUMN_NAME='debug_mode');
SET @sqlstmt := IF(@exist = 0, 'ALTER TABLE php_settings ADD COLUMN debug_mode TINYINT(1) NOT NULL DEFAULT 0 AFTER error_reporting', 'SELECT 1');
PREPARE stmt FROM @sqlstmt;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
