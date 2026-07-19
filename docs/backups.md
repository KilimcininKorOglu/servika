# Backups

Servika supports local and remote backups with scheduling, retention, and restore.

## Backup Contents

Each domain backup includes:

- All files in the domain's home directory
- MariaDB databases owned by the domain
- Cron jobs
- Domain metadata (PHP version, SSL config, DNS records)

## Creating a Backup

On the domain detail page, go to the **Backups** tab and click **Create Backup**. The backup runs immediately and stores the archive locally.

## Backup Schedule

Configure an automatic schedule per domain:

| Setting   | Options                                          |
|-----------|--------------------------------------------------|
| Frequency | Daily, Weekly, Monthly                           |
| Retention | Number of backups to keep (oldest deleted first) |
| Time      | Hour of day (UTC)                                |

The schedule is managed by the panel's cron system and runs as the domain's system user.

## Remote Destinations

Backups can be sent to a remote server via SFTP or FTP.

### Add a Destination

| Field    | Description                  |
|----------|------------------------------|
| Type     | SFTP or FTP                  |
| Host     | Remote server hostname or IP |
| Port     | 22 (SFTP) or 21 (FTP)        |
| Username | Remote user                  |
| Password | Remote password              |
| Path     | Remote directory path        |

Use **Test Destination** to verify connectivity before enabling automatic transfers.

## Restoring a Backup

Select a backup and click **Restore**. The panel:

1. Stops the domain's PHP-FPM pool
2. Extracts the archive to a temporary location
3. Restores files to the domain's home directory
4. Restores databases (if included)
5. Restarts PHP-FPM

Restoring does not overwrite files that have changed since the backup without confirmation.

## Downloading a Backup

Backup archives can be downloaded through the panel. The download link is authenticated and temporary.

## Admin Backup Summary

Admins can view backup status across all domains at **Backups > Summary**. This shows:

- Total backups across all domains
- Per-domain backup counts and sizes
- Last backup time
- Schedules with next run times

## Manual Tick

Admins can trigger the backup scheduler immediately at **Admin > Backups > Tick** (`POST /admin/backups/tick`).

## Panel Database Backups

Separate from domain backups, the panel's own database is backed up by `servika-db-backup`:

```bash
servika-db-backup              # Manual backup
systemctl status servika-db-backup.timer  # Timer status
```

Backups are stored at `/var/backups/servika/db/` with 14-day retention. Each dump is gzip-compressed and verified for integrity before the final filename is assigned.
