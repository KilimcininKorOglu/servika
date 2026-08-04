-- Canonical hostname redirect for a root domain. 'off' keeps the current
-- behaviour, where the vhost answers on both the apex and www with the same
-- content and neither is canonical. 'to_www' and 'to_apex' drop the other host
-- from the main server_name and answer it with a dedicated 301 block instead.
ALTER TABLE domains
  ADD COLUMN www_redirect VARCHAR(16) NOT NULL DEFAULT 'off';
