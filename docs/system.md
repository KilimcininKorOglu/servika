# System & Monitoring

## Server Dashboard

The **Dashboard** page shows real-time server metrics:

| Metric                 | Source           |
|------------------------|------------------|
| CPU usage (%)          | `/proc/stat`     |
| Load average (1/5/15m) | `/proc/loadavg`  |
| Memory usage           | `/proc/meminfo`  |
| Swap usage             | `/proc/meminfo`  |
| Disk usage (root)      | `syscall.Statfs` |
| Network I/O            | `/proc/net/dev`  |
| Uptime                 | `/proc/uptime`   |

## Load History

Go to **Dashboard > Load History** for historical charts (1-168 hours, ~500 data points). Data is sampled every minute by `StartLoadSampler` and retained for 7 days.

## Processes

View running processes at **System > Processes**. This shows the server's process list sorted by CPU usage.

## Server Logs

View server-level logs at **Admin > System Logs**:

| Log           | Path                                  |
|---------------|---------------------------------------|
| Panel         | servika service journal               |
| nginx error   | `/var/log/nginx/error.log`            |
| MariaDB error | `/var/log/mariadb/mariadb.log`        |
| PHP-FPM       | System journal for php*-php-fpm units |
| BIND          | System journal for named unit         |
| Messages      | `/var/log/messages`                   |

Logs can be read inline or tailed live with automatic refresh.

## Service Management

Go to **System > Services** to view and control system services:

| Service     | Unit                            |
|-------------|---------------------------------|
| Panel       | `servika`                       |
| nginx       | `nginx`                         |
| MariaDB     | `mariadb`                       |
| FTP         | `pure-ftpd-mysql` / `pure-ftpd` |
| DNS         | `named`                         |
| PHP 7.4 FPM | `php74-php-fpm`                 |
| PHP 8.2 FPM | `php82-php-fpm`                 |
| PHP 8.3 FPM | `php83-php-fpm`                 |
| PHP 8.4 FPM | `php84-php-fpm`                 |
| Cron        | `crond`                         |
| SSH         | `sshd`                          |

Actions: **Start**, **Stop**, **Restart**. Only predefined allowlist units are accepted. Service output is never exposed in API responses.

## Panel Updates

Go to **Tools & Settings > Panel Update** or use the CLI:

```bash
servika-update              # Update to latest release
servika-update --dry-run    # Preview changes
servika-update --force      # Reapply same version
servika-update --branch X   # Use a different branch
```

### Update Process

1. Create and verify a full MariaDB database dump
2. Download new release artifacts
3. Replace binary, frontend, migrations, ops tools, and systemd units
4. Restart the panel
5. Verify `/healthz` responds successfully
6. Write release marker on success

### Rollback

If the health check fails after an update:
- The previous binary is restored
- The pre-update database dump is restored

### Release Hashes

The updater computes hashes from artifact digests rather than `sha256sum` lines containing random extraction paths. It detects changes in frontend-dist and migrations independently of the binary.

## Package Management

Go to **Tools & Settings > Packages** (admin only) to manage system packages via DNF:

| Action    | Description                 |
|-----------|-----------------------------|
| Search    | Search available packages   |
| Installed | List installed packages     |
| Info      | Show package details        |
| Install   | Install a package           |
| Remove    | Remove a package            |
| Update    | Update all packages         |
| Status    | Check for available updates |

All package operations run as root through `dnf`.

## Domain Health

Each domain has a health check at **Domain > Health**. The panel tests:

- nginx vhost configuration validity
- PHP-FPM socket reachability
- Document root existence
- HTTP response status

## Performance Monitoring

Per-domain performance metrics at **Domain > Performance**:

- PHP-FPM pool status (active/idle workers, slow requests)
- Disk usage vs plan limit
- Monthly bandwidth vs plan limit
- Database query statistics
- Redis cache hit ratio (if enabled)
