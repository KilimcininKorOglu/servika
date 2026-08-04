-- 0068 - per-subdomain PHP and nginx settings.
--
-- A subdomain gains the same customisation a domain has. Rather than cloning php_settings
-- and nginx_settings into subdomain_* twins (which would force every future setting to be
-- maintained in two schemas), both tables gain a subdomain_id discriminator and a composite
-- primary key. subdomain_id = 0 means the domain's own row, so every existing row keeps its
-- meaning and its data untouched.
--
-- No foreign key to subdomains(id) is possible because of the 0 sentinel, so the rows of a
-- removed subdomain are deleted explicitly in internal/subdomain when it is dropped.

ALTER TABLE php_settings
  DROP PRIMARY KEY,
  ADD COLUMN subdomain_id INT NOT NULL DEFAULT 0 AFTER domain_id,
  ADD PRIMARY KEY (domain_id, subdomain_id);

ALTER TABLE nginx_settings
  DROP PRIMARY KEY,
  ADD COLUMN subdomain_id INT NOT NULL DEFAULT 0 AFTER domain_id,
  ADD PRIMARY KEY (domain_id, subdomain_id);

-- Web backend selection per subdomain, mirroring domains.web_backend.
ALTER TABLE subdomains
  ADD COLUMN web_backend VARCHAR(32) NOT NULL DEFAULT 'php-fpm';
