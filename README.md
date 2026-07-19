# Servika

Turns a clean **AlmaLinux 10** server into a complete hosting control panel with a single command. nginx, MariaDB, multiple PHP versions, Valkey (Redis), phpMyAdmin, and a firewall are installed and configured automatically.

## One-line installation

Run as **root** on a clean AlmaLinux 10 server with at least 2 GB of RAM:

```bash
curl -fsSL https://raw.githubusercontent.com/servika/servika/main/install.sh | bash
```

Installation takes approximately 5 to 10 minutes while packages download. When it finishes, the panel address and login information are displayed.

## After installation

- **Panel:** `https://SERVER_IP:8443` (accept the browser warning for the self-signed certificate)
- **Login:** username **`root`**, password = **the server's root password**
  (the panel authenticates the administrator against the operating system's root account through PAM; there is no separate panel password)

## What does it install?

| Component       | Details                                                                                                     |
|-----------------|-------------------------------------------------------------------------------------------------------------|
| **Web**         | nginx (panel on :8443 and customer sites on :80/:443)                                                       |
| **PHP**         | 7.4 / 8.0 / 8.1 / 8.2 / 8.3 / 8.4 / 8.5 / 8.6 (Remi), independent version selection and FPM pool per domain |
| **Database**    | MariaDB 10.11 (`panel` DB) and phpMyAdmin (`/pma/`)                                                         |
| **Cache**       | Valkey (Redis), isolated per-tenant object cache with automatic WordPress integration                       |
| **Security**    | nftables firewall, SELinux compatibility, and ClamAV                                                        |
| **Performance** | Automatic MariaDB, nginx, and OPcache tuning (`servika-optimize`)                                           |

## Panel features

- Domain and subdomain management, DNS editing with templates and bulk operations
- One-click **WordPress** installation and WP-CLI for plugin, theme, user management, and repair
- Per-tenant **Redis object cache** with one-click controls and automatic WordPress integration
- **File manager**, **cron** jobs, and **Git / GitHub** deployment with webhooks
- **SSL** through Let's Encrypt, per-domain PHP versions, PHP extensions, and nginx settings
- **FTP** accounts through Pure-FTPd and per-user SSH chroot jails
- **Firewall** interface with IP bans, allowlists, port blocking, templates, ClamAV scanning, and password-protected directories
- Backup manager with local and remote SFTP/FTP targets and scheduling, plus monitoring, logs, and statistics
- Service plans and resource limits, with **Starter** selected by default when creating a domain
- **2FA** with TOTP for administrator login

## System requirements

- **AlmaLinux 10** (also works on RHEL 10 and Rocky Linux 10)
- At least **2 GB RAM** and 2 vCPUs for five PHP versions, MariaDB, and Valkey
- Root access and an internet connection

## Post-installation utilities

The installation places these utilities in `/usr/local/bin`:

```bash
servika-update            # Safely update the panel from GitHub
servika-db-backup         # Back up the panel database with integrity checks and retention
servika-optimize          # Retune MariaDB, nginx, and PHP for server resources
servika-redis-setup       # Install or repair the Valkey (Redis) infrastructure
servika-wp-redis <domain> # Connect or disconnect Redis cache for a domain's WordPress installation
servika-ftp-setup         # Install or repair the Pure-FTPd MySQL backend
servika-backup-all        # Run scheduled backups for all domains (cron: daily at 03:00 UTC)
servika-jail <user>       # Create a per-user chroot SSH jail, similar to a cPanel jailed shell
servika-repair            # Repair permissions, SELinux contexts, and ownership idempotently
```

## Updating from SSH

Run the updater as root on an installed server:

```bash
servika-update            # Download and apply the latest release
servika-update --dry-run  # Show what would change without applying it
servika-update --force    # Reapply even when the binary is unchanged
servika-update --branch X # Update from another branch
```

The updater preserves `/etc/servika/env` and `/home/c_*` customer sites. Before exposing migrations, it creates a full MariaDB `panel` database dump and stops without changing the binary, frontend, or migrations if that dump fails. It then updates the binary, frontend, migrations, operations tools, and systemd units before restarting Servika and verifying `/healthz`. If the new release fails the health check, the previous binary and pre-update database dump are restored automatically.

The update can also be started from **Tools and Settings > Panel Update**. If `servika-update` is missing from an older installation, the panel downloads it automatically. To bootstrap the tool manually when the panel is unavailable, run:

```bash
curl -fsSL https://raw.githubusercontent.com/servika/servika/main/assets/ops/servika-update \
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

## Notes

- Installation is **not idempotent**. Each run generates new JWT and database secrets. Use `servika-repair` or `servika-optimize` instead of rerunning the installer.
- The panel is served over HTTP/2 with self-signed SSL on port 8443. A real domain and Let's Encrypt certificate can be configured through the panel.

---

## Building from source and development

This project is fully **open source** under the MIT license. Instead of installing the prebuilt binary, you can build and develop it from source. Contributions are welcome.

### Requirements

- **Go 1.23+** for the backend
- **Node.js 20+** and **npm** for the frontend
- MariaDB/MySQL access for execution; the backend applies migrations and administrator seed data on startup

### Backend (Go)

> Release binaries must target `GOAMD64=v1`. Binaries built for newer AMD64 microarchitectures may not start on customer servers whose CPUs lack AVX2 and related instructions. Use `scripts/build-assets.sh` when publishing release binaries.

```bash
# Build a single static binary compatible with baseline AMD64 CPUs
CGO_ENABLED=0 GOAMD64=v1 go build -trimpath -o servika-server ./cmd/server

# Run with environment variables
SERVIKA_JWT_SECRET="$(openssl rand -hex 32)" \
SERVIKA_DB_DSN="root@unix(/var/lib/mysql/mysql.sock)/panel" \
./servika-server
```

The backend API is available under `/api/v1`, and the health check is `/healthz`. In production, administrator login authenticates the operating system's root account through PAM. For development, seed a separate administrator with `scripts/seed_admin.go`:

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

Set `VITE_API_PROXY` to the backend destination, which defaults to `http://localhost:8080`:

```bash
VITE_API_PROXY=http://localhost:8080 npm run dev
```

### Repository structure

```
cmd/server/       Go entry point (main)
internal/         Backend packages (domains, wordpress, dns, redis, firewall, github, backups, ...)
frontend/src/     React interface (pages/, components/, lib/)
migrations/       SQL schema migrations applied at startup
scripts/          Build-time tools (build-assets.sh, seed_admin.go)
assets/           Prebuilt release artifacts used by the installer
install.sh        One-line bootstrap that downloads the repository and runs servika-install.sh
```

> The prebuilt binaries and `frontend-dist.tar.gz` in `assets/` allow `curl | bash` installation without compiling. When publishing changes, rebuild the Go assets with `scripts/build-assets.sh` and regenerate the frontend archive from the `npm run build` output.

## Contributing and license

- Contributions through issues and pull requests are welcome.
- License: **MIT**. See [LICENSE](LICENSE). You may use, modify, distribute, and include it in your own products.
