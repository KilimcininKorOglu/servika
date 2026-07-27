-- 0056 — tracks each backup's remote-copy state.
ALTER TABLE backups ADD COLUMN IF NOT EXISTS remote_status varchar(32)  NOT NULL DEFAULT '';
ALTER TABLE backups ADD COLUMN IF NOT EXISTS remote_key    varchar(512) NOT NULL DEFAULT '';
ALTER TABLE backups ADD COLUMN IF NOT EXISTS remote_error  varchar(512) NOT NULL DEFAULT '';
