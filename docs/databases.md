# Databases

Each domain can create and manage MariaDB databases through the panel. Databases are prefixed with the domain's system user namespace for isolation.

## Creating a Database

On the domain detail page, go to the **Databases** tab and click **Add Database**.

### Auto Mode (recommended)

Toggle **Auto** and the panel generates a database name and user automatically:

- Database name: `<system_user>_db<id>`
- Username: `<system_user>_db<id>`
- Password: 24-character random, shown once

### Custom Mode

Choose a custom database suffix. It must match `[a-z0-9_]{1,32}`. The domain's system user prefix is prepended automatically.

### User Options

| Mode              | Behavior                                                                  |
|-------------------|---------------------------------------------------------------------------|
| **New user**      | Creates a new database user with a generated or custom password           |
| **Existing user** | Grants the new database to an existing database user from the same domain |

The **Existing user** option allows one database user to own multiple databases, matching the cPanel/Plesk model.

## Viewing Databases

The database list shows:

- Database name
- Username
- Host (MariaDB server)
- phpMyAdmin quick-link

Click the phpMyAdmin link to open the database directly (authenticated automatically through the panel's SSO token system).

## Changing Passwords

Admins can reset a database user's password from the panel. The new password is shown once.

## Deleting a Database

Admins can delete a database. The panel checks whether the database user owns other databases:

| User owns...       | Action                                           |
|--------------------|--------------------------------------------------|
| Only this database | Database and user are both dropped               |
| Multiple databases | Only this database is dropped; user is preserved |

This prevents accidentally breaking other databases that share the same user.

## phpMyAdmin

phpMyAdmin is available at `/<domain>/pma/` (proxied through nginx). The panel manages phpMyAdmin SSO:

- A per-session signon token is generated
- Tokens expire after use
- The phpMyAdmin host and socket are auto-configured at startup

## Resource Limits

Database queries are governed by the domain's service plan:

| Limit                | Description                                         |
|----------------------|-----------------------------------------------------|
| Max connections      | Per-user `MAX_USER_CONNECTIONS`                     |
| Max queries per hour | `MAX_QUERIES_PER_HOUR`                              |
| Max updates per hour | `MAX_UPDATES_PER_HOUR`                              |
| Slow query timeout   | Queries exceeding the threshold are logged          |
| Slow query kill      | Queries exceeding a higher threshold are terminated |

Limits are enforced at the MariaDB level via `GRANT` options and are reapplied during startup healing.
