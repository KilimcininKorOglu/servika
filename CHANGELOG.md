# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
