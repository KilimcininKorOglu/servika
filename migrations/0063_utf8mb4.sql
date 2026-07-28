-- 0063 - Convert user-text tables to utf8mb4.
--
-- The DSN already connects as utf8mb4 (SERVIKA_DB_DSN charset=utf8mb4), but the
-- 0001-era tables carry no explicit charset and inherit whatever the panel
-- database default was at CREATE time. If that default is latin1/utf8mb3,
-- 4-byte characters (emoji, some CJK) are silently replaced with '?' on write
-- instead of returning an error, causing quiet data loss in human-entered text.
--
-- Convert the tables that hold arbitrary user text to utf8mb4_unicode_ci. Tables
-- with only ASCII identifiers (domain_name, system_user) or numeric limits are
-- left alone. The widest UNIQUE index here is domains.domain_name VARCHAR(253)
-- = 1012 bytes under utf8mb4, well within the 3072-byte DYNAMIC row-format limit
-- that modern MariaDB defaults to, so the conversion needs no index changes.
ALTER TABLE users         CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
ALTER TABLE customers     CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
ALTER TABLE service_plans CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
ALTER TABLE audit_log     CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
ALTER TABLE domains       CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
