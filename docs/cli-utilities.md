# CLI Utilities

Servika installs these utilities under `/usr/local/bin`. All must be run as **root**.

## servika-update

Safely update the panel from GitHub.

```bash
servika-update              # Update to the latest release
servika-update --dry-run    # Show what would change without applying
servika-update --force      # Reapply even when the same version
servika-update --branch X   # Update from a different branch
```

**Update flow:**
1. Creates a complete MariaDB database dump (aborts on failure)
2. Downloads and verifies new artifacts
3. Replaces binary, frontend, migrations, ops tools, and systemd units
4. Restarts the panel service
5. Verifies `/healthz` responds
6. On failure: restores the previous binary and database dump

To bootstrap the updater on an older installation:

```bash
curl -fsSL https://raw.githubusercontent.com/servika/servika/main/assets/ops/servika-update \
  -o /usr/local/bin/servika-update && chmod +x /usr/local/bin/servika-update
```

---

## servika-db-backup

Back up the panel database (`panel`) with integrity checks.

```bash
servika-db-backup
```

**Details:**
- Dumps all databases
- gzip-compresses the output
- Verifies gzip integrity and minimum size
- Stores at `/var/backups/servika/db/` (mode `0700`, files `0600`)
- Retains 14 days
- Timer: `servika-db-backup.timer` (daily at 03:30 UTC, randomized ±5 min)

**Restore a backup:**

```bash
systemctl stop servika
gunzip -c /var/backups/servika/db/panel-YYYY-MM-DD-HHMMSS.sql.gz | mysql
systemctl start servika
```

---

## servika-optimize

Retune MariaDB, nginx, and PHP for the server's available resources.

```bash
servika-optimize
```

Adjusts:
- MariaDB: `innodb_buffer_pool_size`, `max_connections`, query cache
- nginx: `worker_processes`, `worker_connections`, buffer sizes
- PHP: `opcache.memory_consumption`, `opcache.max_accelerated_files`

Run after upgrading server hardware (RAM, CPU) or when performance needs tuning.

---

## servika-redis-setup

Install or repair the Valkey (Redis) infrastructure.

```bash
servika-redis-setup
```

**What it does:**
- Installs Valkey if not present
- Enables Unix socket support
- Configures ACL for per-tenant isolation
- Starts and enables the service

Idempotent — safe to run on existing installations.

---

## servika-wp-redis

Connect or disconnect Redis object cache for a domain's WordPress installation.

```bash
servika-wp-redis <domain>          # Toggle
servika-wp-redis <domain> --on     # Connect
servika-wp-redis <domain> --off    # Disconnect
```

Requirements:
- `servika-redis-setup` must have been run first
- The domain must have Redis enabled in the panel
- A WordPress installation must exist on the domain

---

## servika-ftp-setup

Install or repair the Pure-FTPd MySQL backend.

```bash
servika-ftp-setup
```

**What it does:**
- Installs Pure-FTPd with MySQL support
- Configures the `ftp_accounts` table connection
- Sets up passive port range
- Starts and enables the service

---

## servika-backup-all

Run scheduled backups for all domains that have backup schedules configured.

```bash
servika-backup-all
```

**Cron timer:** `servika-backup-all.timer` (daily at 03:00 UTC).

This processes each domain's backup schedule in sequence. Domains without a schedule are skipped. Backups are stored locally and optionally transferred to remote destinations.

---

## servika-jail

Create a per-user chroot SSH jail, similar to a cPanel jailed shell.

```bash
servika-jail <system_user>
```

**What it does:**
- Sets up a minimal chroot environment in `/home/jails/<user>/`
- Binds the user's home directory
- Copies required binaries and libraries (bash, ls, cp, mv, rm, cat, grep, nano, scp, sftp, git, rsync, tar, gzip, unzip, curl, wget, php, wp, composer, mysql, mysqldump)
- Includes `/etc` files: passwd, group, resolv.conf, hosts, nsswitch.conf, localtime, ssl/certs, pam.d, security
- Replicates `/lib64` and `/usr/lib64` with required shared libraries
- Updates the user's shell in `/etc/passwd`

The jail configuration file is at `/etc/security/servika-jail/50-servika-jail.conf`.

This is automatically configured when SSH access is enabled for a domain in the panel.

---

## servika-repair

Repair permissions, SELinux contexts, and file ownership.

```bash
servika-repair
```

**What it repairs:**
- `/opt/servika/` ownership and SELinux labels
- `/home/c_*` POSIX ACL grants
- nginx configuration file permissions
- PHP-FPM socket permissions
- `/var/log/` log file ownership
- Systemd unit file contexts

Idempotent — safe to run on healthy installations. Run after manual file changes or if permissions drift over time.

---

## Health Check

All CLI tools exit with code `0` on success and non-zero on failure. Scripts are written in POSIX-compatible shell (checked with `shellcheck`).
