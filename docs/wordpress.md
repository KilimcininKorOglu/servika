# WordPress

Servika provides one-click WordPress installation and a full WP-CLI toolkit for managing plugins, themes, users, and repairs.

## Installing WordPress

On the domain detail page, go to the **WordPress** tab and click **Install**. Provide:

| Field          | Description                                              |
|----------------|----------------------------------------------------------|
| Site title     | The WordPress site name                                  |
| Admin username | WP admin user                                            |
| Admin password | WP admin password (auto-generated or custom)             |
| Admin email    | Admin email address                                      |
| Database name  | Auto-selected or choose an existing database             |
| Path           | Subdirectory (e.g. `/wp`), leave empty for document root |

Installation uses WP-CLI and takes a few seconds. The site is immediately available at the domain URL.

## WordPress Toolkit

Click a WordPress installation to access the toolkit:

### Status

View installation details: WP version, PHP version, database name, active plugins/themes count, and maintenance mode status.

### Plugins

- **List** — View all installed plugins with status (active/inactive) and available updates
- **Activate / Deactivate** — Toggle individual plugins
- **Update** — Update individual plugins or all at once
- **Delete** — Remove a plugin

### Themes

- **List** — View installed themes with active status
- **Activate** — Switch to a different theme
- **Update** — Update a theme
- **Delete** — Remove an inactive theme

### Users

- **List** — View all WordPress users with roles
- **Reset Password** — Set a new password for any user

### Repair

Run WordPress repair operations:

| Operation                   | Description                          |
|-----------------------------|--------------------------------------|
| `wp core verify-checksums`  | Verify WordPress core file integrity |
| `wp rewrite flush`          | Flush permalink rules                |
| `wp cache flush`            | Clear object cache                   |
| `wp transient delete --all` | Delete all transients                |
| `wp db repair`              | Repair database tables               |
| `wp db optimize`            | Optimize database tables             |

### Tools

Run WP-CLI commands: `wp plugin auto-updates`, `wp theme auto-updates`, `wp site switch-language`, and more.

### Maintenance Mode

Toggle maintenance mode on/off. When enabled, visitors see a "temporarily unavailable" page. The maintenance state is file-backed under `wp-content`.

## Updating WordPress

Two update paths:

1. **WordPress Core** — Click **Update** on the WordPress installation row
2. **Plugins/Themes** — Use the toolkit to update individually

Updates use WP-CLI and run with the domain's system user for correct file ownership.

## Listing All WordPress Sites (Admin)

Admins can view all WordPress installations across all domains at **Tools & Settings > WordPress** (`/wordpress/all`). This shows the domain name, WP version, and PHP version for each installation.

## Redis Integration

When Redis object cache is enabled for a domain (see [Redis](redis.md)), WordPress sites on that domain can connect to it with a single click. The panel installs and configures the Redis object cache plugin automatically.

## Deleting WordPress

Removing a WordPress installation deletes the WordPress files and the associated database (if the database is not shared with other WordPress installs). The action is irreversible — back up first.
