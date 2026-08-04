# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.5] - 2026-08-04

### Changed
- Upgraded the panel to React 19 and React Router 8, closing a routing advisory that no 7.x release fixes, and patched a path traversal in the build-time postcss dependency.
- Panel pages no longer paint one frame of the previous domain's data when you switch domain, directory, log file, or filter: the file-manager selection, log tail, monitoring panels, audit-log spinner, and search deep links now settle in the same render as the change.
- Removed unreachable components and their translations, and added an ESLint gate over the whole frontend so this class of rendering defect is caught before it ships.

### Fixed
- The file editor marked a saved file as unsaved again right after saving, leaving the Save button enabled with nothing left to write.
- The antivirus page could leave a scan poll running after the scan finished or the page changed.
- The elapsed time and estimated remaining time of a finished site migration kept counting up instead of stopping where the job ended.
- The stored FTP and database password shown in the connection dialog, and the server status load error, stayed in the previous language after a language switch.

## [1.1.4] - 2026-08-02

### Added
- The release announcement in the update panel is now published per language and rendered in the language the panel is displaying, instead of always in English. Any language without its own text falls back to English.

## [1.1.3] - 2026-08-02

### Added
- Live site migration from cPanel, Plesk, and DirectAdmin over SSH: discovers every account and domain on the source server, then transfers files, databases, DNS records, and SSL in one job with a three-step wizard, live progress, and a cancellable background run.
- Granular backup restore with five modes (full, files only, databases only, a single file, a single database); a restore never deletes files missing from the backup by default, and selected files land in a separate folder instead of overwriting the live site.
- Every domain-owned database is now packaged in each backup together with a manifest, replacing the previous main-database-only archive.
- Bulk backup and restore now run as tracked jobs with live progress, per-domain results, and a job detail page.
- Ten further interface languages (German, French, Italian, Spanish, Portuguese, Brazilian Portuguese, Romanian, Japanese, Czech, and Simplified Chinese), bringing the panel to twelve.
- The update panel now shows the running build date.

### Changed
- Tenant nginx access and error logs are no longer readable by other tenants; the log directory and its files are closed on every startup.
- Tenant Redis accounts can no longer enumerate key names: scan and randomkey are denied for new accounts and withdrawn from existing ones at startup.
- The repair tool no longer loosens tenant home isolation to 0711/0755; it now enforces the same 0710/0750 model as the provisioner, with an nginx-group fallback for filesystems that ignore ACLs.
- Archive extraction rejects a crafted uncompressed-size header that could overflow the size limit.
- Operations scripts and long-running maintenance job logs are English only again, and the repository slug now points at ServikaPanel/servika.

### Fixed
- A domain served nothing (HTTP 403) on filesystems that silently ignore ACLs; nginx read access is now verified before the ACL model is trusted, and the group fallback carries the whole document root.
- The panel update card could spin forever when the service restart interrupted its status stream; the result is now also read from the update log, and a finished update reports whether it succeeded.
- Audit entries created by the panel itself were recorded as 127.0.0.1 and were indistinguishable from real visitors; they are now labelled as system.
- A missing SSH jail asset was ignored silently, leaving tenants with an unconfined shell and no record of it.
- Restored the literal update-count keys on the Chinese WordPress page.

## [1.1.2] - 2026-08-01

### Added
- Full Turkish/English localization across the panel (react-i18next), with a live language switcher, a per-user preference, and a server-default language chosen at install.
- Localized live logs for long-running maintenance jobs (update, optimize, CVE, KernelCare) and the `servika-update`/`servika-optimize` scripts, following the panel default language.
- Reseller multi-user panel accounts with a role-based sidebar, reseller-scoped management UI, and admin UI to view and edit reseller disk/traffic/customer/domain quotas.
- Reseller-scoped hosting endpoints and quota enforcement across domains, WordPress, and backup lists; reseller suspension now cascades to its customers' hosting.
- cPanel full-account import: web files, multiple databases, mailboxes and forwarders, cron jobs, and SSL certificates.
- Server-wide DNS/SSL/mail/database overview lists and a per-reseller-scoped security (audit) log.
- Mail spam filtering, Sieve rules, per-mailbox send limits, and Postfix queue management.
- S3 and Backblaze B2 remote backup destinations.
- BIND DNS zone import/export (backend and UI).
- Optional Let's Encrypt SSL when creating a domain; domain SSL indicator, open-in-new-tab, and reseller column.
- Global panel search in the top bar, admin server-hostname management, branded welcome and server-wide 404 pages, an inline nginx vhost editor, and auto-expanding file-manager directory tree.

### Changed
- Encrypt database account passwords at rest (AES-256-GCM) and store FTP/SSH credentials as yescrypt/SHA-512-crypt hashes instead of cleartext.
- Trust proxy IP headers only with the shared proxy secret; heal panel proxy trust and clean deprovision orphans on startup.
- Pre-scan the cPanel import archive with member validation before extraction; validate backup destinations and harden SFTP against ssh argument injection; reject dangerous nginx directives with quote-aware tokenization.
- Reject NUL in the root password before chpasswd and close a username timing side-channel on login.
- Verify release bundles against the published SHA256SUMS before install/update.
- Collapse to a single role-based token (drop the FTP login bridge), store client session state in cookies (never localStorage), convert user-text tables to utf8mb4, standardize page containers to full width, lazy-load route pages, and modernize internal code to current Go idioms.

### Fixed
- Revoke live sessions on credential and authorization changes; reject replayed GitHub webhook deliveries.
- Harden file uploads against disk exhaustion and quota bypass; symlink-safe permission reset for public_html; handle unix.Close errors on directory fd releases.
- Bound manual backup work and cap manual retention; fully tear down the systemd slice on delete; pin TMPDIR with a single-pass cPanel import and mail policy hardening.
- DNS-aware www SAN and guard PHP-FPM pool writes for deleted users; prevent addon domains from overwriting the parent vhost; allow safe tenant preview via CSP frame-ancestors and make the domain preview refreshable.
- Detect edits to already-applied migration files; resolve clean-install verification errors.

## [1.1.1] - 2026-07-27

### Changed
- Upgraded the chi router to v5.3.0 and edwards25519 to v1.1.1, clearing three dependency advisories reported by govulncheck (none on a reachable code path).
- Modernized internal code paths to current Go idioms (`min`, `maps.Copy`, `slices.ContainsFunc`, `strings.FieldsSeq`, `atomic.Int64`, `WaitGroup.Go`) with no change in behavior.

## [1.1.0] - 2026-07-26

### Added
- Subdomain management pages and a nested subdomain list under each domain.
- WordPress, Composer, and log tooling scoped to individual subdomains.
- Subdomain traffic aggregated into the parent domain statistics.
- Password-protected directories scoped to subdomains.
- Global subdomain list endpoint.
- Subdomain detail endpoint.
- Per-subdomain PHP version switching.

### Changed
- Adopted `SplitSeq` and integer range loops.
- Updated the installation and configuration guide.

### Fixed
- acme.sh now recovers from a rejected account contact address, which previously
  blocked certificate issuance for every domain on the host permanently.
- phpMyAdmin signon token expiry is evaluated with the MySQL clock, so tokens are
  no longer discarded instantly on hosts whose database timezone is not UTC.
- Laravel installs preserve the tenant inode quota and surface install failures.

## [1.0.3] - 2026-07-25

### Added
- Optional offsite upload for panel database backups over FTP/SFTP with remote retention.
- RED (Rate, Errors, Duration) metrics instrumentation for the HTTP API.

### Changed
- Release packaging is now gated on the full validation suite via a reusable CI workflow.
- Added unit-test coverage for validation helpers across critical backend modules.
- Metrics handler now checks response-writer write results.
- Removed the unused phpMyAdmin root accessor.
- Release notes are sourced from the matching CHANGELOG section.

### Fixed
- Update and restore now deploy immutable tagged release bundles instead of mutable branch snapshots.
- Scheduled domain backups run through a single authoritative runner, no longer duplicated by a root cron.
- Laravel install and deploy jobs are finalized server-side without depending on client polling.
- Subscription deletion now requires administrator authorization.
- Archive extraction enforces decompression-bomb size and member limits.
- Domain HTTPS health probes verify the TLS certificate chain and hostname.
- The build requires a patched Go toolchain and gates releases on a vulnerability scan.
- phpMyAdmin signon tokens are exchanged in a POST body instead of a URL query string.
- Request lifecycle logging added to the API middleware stack.
- Customer FTP passwords are no longer stored as cleartext at rest.
- Installer verifies third-party downloads before use.
- Panel CSP tightened to `script-src 'self'`, isolating phpMyAdmin and webmail.
- Session tokens are delivered via an HttpOnly cookie instead of localStorage.
- Update rollback restores every release-owned asset, not only the binary and database.
- Stored GitHub access tokens are encrypted at rest and kept out of repository URLs.
- Auto-registered GitHub webhooks require TLS verification.
- GitHub webhook HMAC signatures are verified before pulling.
- Migrations apply once each inside a transaction.
- Server-side JWT session revocation is supported.
- Remote backup destination credentials are encrypted at rest.
- CORS reflects only the same origin instead of a wildcard.
- Manual backup creation is rate-limited and serialized per domain.
- Expensive file-manager operations are throttled per IP.
- The public Git webhook endpoint is throttled.
- Every response carries a request id for error correlation.
- Raw exception details are hidden from end users.
- Load sampler failures are logged instead of discarded.
- The readiness probe verifies the database dependency.
- Laravel deploy jobs fail when a critical step fails.
- Orphaned tenant metadata is deleted on domain removal.
- File uploads are exempt from the JSON body limit.
- System operation error details are redacted from API responses.
- Download filenames are safely encoded in Content-Disposition.
- Repository credentials are redacted from API responses.
- Database plan limits are enforced atomically, including during WordPress install.
- Tenant file reads and archive extraction resolve through symlink-safe file descriptors.
- SSRF to internal targets from customer-controlled hosts is blocked.
- Suspension is enforced on phpMyAdmin signon token creation.
- A FastCGI cache key is defined for tenant vhosts.
- Restores fail when the database import fails.
- The inconsistent upload extension block was removed.

## [1.0.2] - 2026-07-24

### Changed
- CI now uses golangci-lint v2 for Go 1.25 compatibility.

### Fixed
- Subdomain, PHP extension, resource-limit, system, SSH, and Laravel handlers
  no longer report failed system applies as success.
- Credential and resource teardown failures (MySQL, Redis, Git) are now
  surfaced instead of silently swallowed.
- Safety-guard count checks in quota, accounts, plans, and PHP-version paths
  fail closed on query errors instead of proceeding as if the limit passed.
- DNS record mutations surface zone-write failures instead of reporting success.
- Backup dumps abort on mysqldump failure instead of archiving corrupt dumps.
- TOTP login fails closed when the replay-protection step cannot be persisted.
- safeio propagates write-path Close errors (e.g. ENOSPC) and checks all Close
  results; removed dead chown helpers.

## [1.0.1] - 2026-07-24

First tagged release. Servika is a self-hosted web hosting control panel for
AlmaLinux/RHEL 10, covering domains, mail, databases, PHP, DNS, TLS, tenant
isolation, and resource governance.

### Added
- Dashboard with drag-and-drop widget layout, live load/memory charts, CVE
  security widget, KernelCare integration, panel version footer, and
  click-to-copy server IP.
- Domain management: addon domains, redirects, per-domain access controls,
  raw custom nginx vhost overrides, and Laravel toolkit.
- Native mail stack: mailboxes, forwarder aliases, OpenDKIM, Postfix virtual
  mail, and Roundcube webmail.
- Per-domain PHP management: eight PHP-FPM versions for AlmaLinux 10, debug
  mode toggle with log panel, and isolated per-tenant PHP-FPM services.
- Databases: one DB user owning multiple databases and a MySQL query governor.
- Resource governance: absolute disk I/O limits, MariaDB governor, systemd
  slice enforcement, and XFS user quota with reboot-required sentinel.
- Security: ModSecurity + OWASP CRS WAF, native Go yescrypt auth, TOTP 2FA
  with QR and replay protection, per-IP login rate limiting, and POSIX ACL
  tenant home isolation.
- Anonymous version-check telemetry, panel self-update flow, maintenance mode,
  and a file manager with metadata, RAR archives, and web preview.
- Multi-arch release pipeline (linux amd64 + arm64) with CI and GitHub Release
  workflows, and a binary-release-based installer.

### Changed
- Centralized configuration path and production environment loading.
- Restructured build assets into a multi-arch directory layout and version
  injection via ldflags.

### Fixed
- Hardened file operations against TOCTOU and symlink attacks with openat2.
- Prevented chpasswd/lftp command injection and web-root PHP webshell uploads.
- Sealed username enumeration and heuristic caching of JSON API responses.
- Made schema migrations idempotent and restored tenant limits on startup heal.
