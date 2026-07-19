# Servika

Turns a clean **AlmaLinux 10** server into a complete hosting control panel with a single command. nginx, MariaDB, multiple PHP versions, Valkey (Redis), phpMyAdmin, ModSecurity WAF, and a firewall are installed and configured automatically. Both **x86_64** and **ARM64 (aarch64)** are supported.

## One-line installation

Run as **root** on a clean AlmaLinux 10 server with at least 2 GB of RAM:

```bash
curl -fsSL https://raw.githubusercontent.com/KilimcininKorOglu/servika/main/install.sh | bash
```

Installation takes about 15 to 20 minutes while packages download. When finished, the panel address and credentials are displayed.

## After installation

- **Panel:** `https://SERVER_IP:8443` (accept the self-signed certificate warning)
- **Login:** username **`root`**, password = **the server's root password**
  (the panel verifies the administrator against the system root account via /etc/shadow; there is no separate panel password)

## What it installs

| Component       | Details                                                                                                     |
|-----------------|-------------------------------------------------------------------------------------------------------------|
| **Web**         | nginx (panel on :8443, customer sites on :80/:443)                                                          |
| **PHP**         | 7.4 / 8.0 / 8.1 / 8.2 / 8.3 / 8.4 / 8.5 / 8.6 (Remi), per-domain version selection and FPM pool             |
| **Database**    | MariaDB 10.11 (`panel` DB) with phpMyAdmin at `/pma/`                                                       |
| **Cache**       | Valkey (Redis 7.x compatible), isolated per-tenant object cache with automatic WordPress integration        |
| **Security**    | nftables firewall, ModSecurity v3 + OWASP CRS WAF, SELinux enforcing support, ClamAV malware scanning       |
| **Performance** | Automatic MariaDB, nginx, and OPcache tuning (`servika-optimize`), XFS user quota with per-plan disk limits |

## Panel features

- Domain and subdomain management with DNS editing, templates, DNSSEC, and bulk operations
- One-click **WordPress** installation and WP-CLI toolkit for plugin, theme, user, and repair management
- Per-tenant **Redis object cache** with one-click enable, automatic WordPress drop-in, and ACL isolation
- **File manager** with code editor, archive extraction (ZIP, TAR, RAR), bulk copy/move, and search
- **Cron** job editor and **Git / GitHub** deployment with deploy keys and webhook auto-pull
- **SSL** via Let's Encrypt or self-signed, with LE rate-limit resilience (reuse-before-issue, never-drop-443)
- Per-domain **PHP versions** with independent settings, **PHP extensions** manager, and PECL support
- **FTP** accounts through Pure-FTPd with MySQL backend and per-user SSH chroot jails
- **Firewall** (nftables) with IP bans, allowlists, port blocking, and ready-made templates
- **WAF** (ModSecurity + OWASP CRS) per-domain or plan-default, with Detection and Blocking modes
- **Password-protected directories** via htpasswd with nginx integration
- Backup manager with local retention, remote SFTP/FTP destinations, scheduling, and point-in-time restore
- Service plans with resource limits (CPU, RAM, disk, I/O, inodes, MariaDB governor, process caps)
- **Monitoring**, **statistics** (nginx traffic analysis), **system logs**, and **load history** charts
- **2FA** with TOTP and QR enrollment for administrator login

## System requirements

- **AlmaLinux 10** (also works on RHEL 10 and Rocky Linux 10)
- **x86_64** or **ARM64 (aarch64)** architecture
- At least **2 GB RAM** and 2 vCPUs
- Root access and internet connection

## Post-installation utilities

The installer places these tools under `/usr/local/bin`:

```bash
servika-update              # Safely update the panel from GitHub with pre-update DB dump and automatic rollback
servika-db-backup           # Back up the panel database with gzip integrity checks and 14-day retention
servika-optimize            # Retune MariaDB, nginx, and PHP-FPM for available server resources
servika-redis-setup         # Install or repair the Valkey (Redis) infrastructure
servika-wp-redis <domain>   # Connect or disconnect Redis cache for a domain's WordPress installations
servika-ftp-setup           # Install or repair the Pure-FTPd MySQL backend
servika-backup-all          # Run scheduled backups for all domains (cron: daily at 03:00 UTC)
servika-jail <user>         # Create a per-user chroot SSH jail with sshd Match group isolation
servika-repair              # Repair permissions, SELinux contexts, and ownership idempotently
servika-restore             # Restore core panel files from the canonical release with integrity verification
servika-waf-setup           # Install or repair ModSecurity v3 + OWASP CRS with nginx -t gating
```

## Updating from SSH

Run the updater as root on an installed server:

```bash
servika-update              # Download and apply the latest release
servika-update --dry-run    # Show what would change without applying it
servika-update --force      # Reapply even when the release hash is unchanged
servika-update --branch X   # Update from another branch
```

The updater preserves `/etc/servika/env` and `/home/c_*` customer sites. Before exposing migrations, it creates a full MariaDB `panel` database dump and aborts if the dump fails. It then updates the binary, frontend (atomic verified swap), migrations, operations tools, and systemd units before restarting Servika and verifying `/healthz`. If the new release fails the health check, the previous binary and pre-update database dump are restored automatically.

The update can also be started from **Tools and Settings > Panel Update**. If `servika-update` is missing from an older installation, the panel downloads it automatically. To bootstrap the tool manually when the panel is unavailable:

```bash
curl -fsSL https://raw.githubusercontent.com/KilimcininKorOglu/servika/main/assets/ops/servika-update \
  -o /usr/local/bin/servika-update && chmod +x /usr/local/bin/servika-update

servika-update
```

## Panel database backups

The `servika-db-backup.timer` unit runs daily at 03:30 with a randomized delay of up to five minutes. Backups are stored under `/var/backups/servika/db` with directory mode `0700` and file mode `0600`. Dumps are retained for 14 days and receive their final filename only after gzip integrity and minimum-size checks pass.

Create a backup manually:

```bash
servika-db-backup
```

Restore a selected backup while the panel is stopped:

```bash
systemctl stop servika
gunzip -c /var/backups/servika/db/panel-YYYY-MM-DD-HHMMSS.sql.gz | mysql
systemctl start servika
```

Panel database backups are separate from customer site and database backups managed by `servika-backup-all`.

## Core repair

When core panel files become corrupted (0-byte frontend, missing binary), restore them from the canonical release without touching customer data:

```bash
servika-restore              # Restore core files from the canonical release
servika-restore --dry-run    # Diagnose only — show what is broken, touch nothing
servika-restore --no-restart # Repair files but do not restart the service
```

## Notes

- Installation is **not idempotent**. Each run generates new JWT and database secrets. Use `servika-repair` or `servika-optimize` instead of rerunning the installer.
- The panel is served over HTTP/2 with self-signed SSL on port 8443. A real domain and Let's Encrypt certificate can be configured through the panel.
- ARM64 (aarch64) is fully supported. Remi provides complete package parity with x86_64 for EL10 (3100+ packages).

---

## Building from source and development

This project is fully **open source** under the MIT license. You can build and develop it from source instead of using the prebuilt binaries. Contributions are welcome.

### Requirements

- **Go 1.23+** for the backend
- **Node.js 20+** and **npm** for the frontend
- MariaDB/MySQL access for runtime execution; migrations and seed data are applied on startup

### Backend (Go)

Release binaries target `GOAMD64=v1` (amd64) and default `GOARM64` (arm64). Use `scripts/build-assets.sh` when publishing release binaries.

```bash
# Build a single static binary
CGO_ENABLED=0 GOAMD64=v1 go build -trimpath -o servika-server ./cmd/server

# Build for ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o servika-server ./cmd/server

# Run with environment variables
SERVIKA_JWT_SECRET="$(openssl rand -hex 32)" \
SERVIKA_DB_DSN="root@unix(/var/lib/mysql/mysql.sock)/panel" \
./servika-server
```

The backend API is available under `/api/v1`, health check at `/healthz`. In production, administrator login verifies the system root account through /etc/shadow. For development, seed a separate administrator:

```bash
go run scripts/seed_admin.go -dsn '<DSN>' -username admin -password 'CHOOSE_A_PASSWORD' -email 'admin@example.com'
# Alternatively, use the SERVIKA_SEED_PASSWORD environment variable.
```

### Frontend (React + Vite + TypeScript)

```bash
cd frontend
npm install
npm run dev        # Development server on :5185, proxies /api to VITE_API_PROXY
npm run build      # Production build output in frontend/dist/
```

Set `VITE_API_PROXY` to the backend address (defaults to `http://localhost:8080`):

```bash
VITE_API_PROXY=http://localhost:8080 npm run dev
```

### Repository structure

```
cmd/server/       Go entry point (main)
internal/         Backend packages (domains, wordpress, dns, redis, firewall, files, ...)
frontend/src/     React interface (pages/, components/, lib/, store/)
migrations/       SQL schema migrations applied at startup
scripts/          Build-time tools (build-assets.sh, seed_admin.go)
assets/           Prebuilt release artifacts used by the installer
install.sh        One-line bootstrap that downloads the repository and runs servika-install.sh
```

Prebuilt binaries and archives in `assets/` enable `curl | bash` installation without compiling. When publishing changes, rebuild Go assets with `scripts/build-assets.sh` and regenerate the frontend archive from the `npm run build` output.

## Contributing and license

- Contributions through issues and pull requests are welcome.
- License: **MIT**. See [LICENSE](LICENSE).
