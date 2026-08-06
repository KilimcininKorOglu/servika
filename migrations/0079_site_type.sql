-- 0079_site_type.sql - Records what a domain is FOR, so provisioning can differ.
--
-- Every domain used to receive a MySQL database and a MySQL user whether or not
-- anything would ever connect to them. A site that serves only HTML, CSS and
-- JavaScript has no use for either, so the pair sat unused and widened the
-- attack surface for nothing.
--
-- Existing domains take the default and keep behaving exactly as before.
ALTER TABLE domains ADD COLUMN site_type ENUM('php','wordpress','static') NOT NULL DEFAULT 'php';
