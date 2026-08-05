-- Pin the SFTP host key of a backup destination on first use (trust on first
-- use). Empty until the first successful scan; after that every connection is
-- verified against this exact key, so a man in the middle cannot collect the
-- destination password or the backup contents by answering in the host's place.
ALTER TABLE backup_destinations
  ADD COLUMN IF NOT EXISTS host_key varchar(1024) NOT NULL DEFAULT '';
