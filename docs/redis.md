# Redis Object Cache

Servika runs Valkey (Redis-compatible) as an object cache with per-tenant isolation and automatic WordPress integration.

## Enabling Redis for a Domain

On the domain detail page, go to the **Redis** tab.

### Status

Shows whether Redis is enabled and the connection details:

- Socket path: `/var/run/redis/<domain>.sock`
- Database index: domain-specific
- Memory used / max memory

### Enable Redis

Click **Enable Redis**. The panel:

1. Allocates a dedicated database index for the domain
2. Creates a Unix socket at `/var/run/redis/<system_user>.sock`
3. Sets a per-domain password
4. Sets a `maxmemory` limit from the domain's service plan

The socket is owned by `valkey:valkey` with ACL access for the domain's system user.

### Disable Redis

Click **Disable Redis**. The panel:

1. Flushes the domain's database index
2. Removes the socket
3. Deactivates the WordPress Redis plugin if connected

## WordPress Integration

When Redis is enabled for a domain:

1. Go to the domain's **WordPress** tab
2. Select the WordPress installation
3. Click **Redis > Connect**

The panel installs and activates a Redis object cache plugin, configures it with the domain's Redis socket and password, and flushes the cache.

To disconnect, click **Redis > Disconnect** which deactivates the plugin and removes the configuration.

## Redis Setup (Server-Level)

The `servika-redis-setup` CLI tool installs and configures Valkey at the server level:

```bash
servika-redis-setup
```

This tool:
- Installs Valkey if not present
- Configures Unix socket support
- Sets up ACL for tenant isolation
- Starts and enables the service

Run this once after installation. It is idempotent — safe to run on existing installations for repair.

## WordPress Redis (Per-Domain CLI)

The `servika-wp-redis` tool manages Redis for a specific domain's WordPress from the command line:

```bash
servika-wp-redis <domain>          # Toggle Redis cache
servika-wp-redis <domain> --on     # Connect
servika-wp-redis <domain> --off    # Disconnect
```

## Memory Limits

The `maxmemory` limit for each domain's Redis database is set from the domain's service plan. The panel enforces this limit via Valkey's `CONFIG SET maxmemory` command.

## Monitoring

Redis is monitored through the domain's **Performance** tab, which shows:
- Cache hit ratio
- Memory usage vs limit
- Connected clients
- Keyspace size
