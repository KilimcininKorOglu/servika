-- Scope protected directories to a subdomain. subdomain_id 0 means the parent
-- domain's own document root, so existing rows keep their current meaning.
ALTER TABLE protected_directories
  ADD COLUMN subdomain_id INT NOT NULL DEFAULT 0;

-- The uniqueness rule must include the scope, otherwise the same path cannot be
-- protected on both the domain and one of its subdomains.
ALTER TABLE protected_directories
  DROP INDEX uq_protected_directory;

ALTER TABLE protected_directories
  ADD UNIQUE KEY uq_protected_directory (domain_id, subdomain_id, path, username);

ALTER TABLE protected_directories
  ADD KEY ix_protected_directory_subdomain (subdomain_id);
