# Installation

## Requirements

- **AlmaLinux 10** (RHEL 10 and Rocky Linux 10 also supported)
- At least **2 GB RAM** and 2 vCPUs
- Root access
- Internet connection

## One-Line Install

Run as **root** on a clean server:

```bash
curl -fsSL https://raw.githubusercontent.com/servika/servika/main/install.sh | bash
```

The installer takes 5 to 10 minutes. It provisions nginx, MariaDB, multi-version PHP-FPM, Valkey (Redis), phpMyAdmin, and an nftables firewall automatically.

When finished, the panel address and login credentials are printed to the terminal.

## First Login

- **URL:** `https://<SERVER_IP>:8443`
- **Username:** `root`
- **Password:** the server's root password (authenticated via PAM)

The browser shows a certificate warning for the self-signed certificate. Accept it to proceed. A valid Let's Encrypt certificate can be configured later through **Domains > SSL**.

## Post-Install

### Install additional PHP versions

```bash
# In the panel: Tools & Settings > PHP Versions
# Or via API: the panel auto-detects available Remi packages
```

Available versions: 7.4, 8.0, 8.1, 8.2, 8.3 (appstream), 8.4, 8.5, 8.6 (all via Remi).

### Set up Redis cache

```bash
servika-redis-setup          # One-time Redis infrastructure installation
```

### Create your first domain

In the panel: **Domains > Add Domain**. A `Starter` service plan is selected by default. Each domain gets its own Linux system user, home directory, nginx vhost, and PHP-FPM pool.

### Enable automatic backups

```bash
systemctl enable --now servika-backup-all.timer   # Daily at 03:00 UTC
systemctl enable --now servika-db-backup.timer     # Daily at 03:30 UTC
```

## What Gets Installed

| Component  | Details                                               |
|------------|-------------------------------------------------------|
| nginx      | Panel on :8443, customer sites on :80/:443            |
| PHP-FPM    | 7.4 through 8.6 via Remi, per-domain version and pool |
| MariaDB    | 10.11 with `panel` database                           |
| phpMyAdmin | Available at `/pma/` on every domain                  |
| Valkey     | Redis-compatible object cache, per-tenant isolation   |
| BIND       | Authoritative DNS with DNSSEC support                 |
| Pure-FTPd  | FTP with MySQL backend, per-domain accounts           |
| nftables   | Firewall managed through panel                        |
| ClamAV     | Antivirus with on-demand scanning                     |

## Installation is NOT Idempotent

The installer generates new JWT secrets and database credentials on each run. Do not re-run it on an existing installation. Use these tools instead:

- `servika-repair` — repair permissions, SELinux contexts, ownership
- `servika-optimize` — retune MariaDB, nginx, PHP configuration
- `servika-update` — update the panel to the latest release

## Environment Configuration

The panel reads these environment variables from `/etc/servika/env`:

| Variable                   | Default       | Description                   |
|----------------------------|---------------|-------------------------------|
| `SERVIKA_LISTEN`           | `:8080`       | Listen address                |
| `SERVIKA_DB_DSN`           | (required)    | MariaDB DSN                   |
| `SERVIKA_JWT_SECRET`       | (required)    | JWT signing key, ≥32 chars    |
| `SERVIKA_JWT_LIFETIME_SEC` | `3600`        | JWT expiry in seconds         |
| `SERVIKA_ENV`              | `production`  | Environment name              |
| `SERVIKA_PUBLIC_IPV4`      | (auto-detect) | Public IPv4 for DNS templates |
