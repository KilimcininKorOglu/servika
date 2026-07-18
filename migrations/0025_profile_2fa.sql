ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret  varchar(64) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled tinyint(1)  NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS pref_theme  varchar(8)  NOT NULL DEFAULT 'system';
ALTER TABLE users ADD COLUMN IF NOT EXISTS pref_lang   varchar(8)  NOT NULL DEFAULT 'en';
UPDATE users SET email='', full_name='' WHERE username='root' AND email='admin@local' AND full_name='System Administrator';
