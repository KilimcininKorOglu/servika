-- 0036 - db_accounts: allow one database user to own multiple databases by dropping the
-- db_user UNIQUE constraint. In the "New Database" modal the customer can select an EXISTING
-- user and attach a new database to it (cPanel/Plesk model: one user to many databases).
-- db_name UNIQUE is preserved (every database name stays unique). GRANT and the db_accounts row
-- are per-database. Idempotent: MariaDB 10.5+ supports "DROP INDEX IF EXISTS"; no-op when absent.
ALTER TABLE db_accounts DROP INDEX IF EXISTS db_user;