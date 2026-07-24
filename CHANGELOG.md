# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
