# Servika Documentation

Welcome to the Servika hosting control panel documentation. These guides cover every feature, CLI utility, and workflow available to administrators and end-users.

## Guides

| Guide                                           | Description                                                                      |
|-------------------------------------------------|----------------------------------------------------------------------------------|
| [Installation](installation.md)                 | Server requirements, one-line install, post-install setup                        |
| [Domains](domains.md)                           | Create, suspend, delete domains; PHP version selection; subdomains; web backends |
| [DNS](dns.md)                                   | DNS zone management, record types, templates, DNSSEC                             |
| [WordPress](wordpress.md)                       | One-click install, WP-CLI toolkit, plugin/theme/user management, repair          |
| [File Manager](files.md)                        | Browse, upload, edit, archive, extract, search, permissions                      |
| [Databases](databases.md)                       | Create, delete databases; auto vs custom naming; shared users; phpMyAdmin        |
| [SSL / TLS](ssl.md)                             | Let's Encrypt issuance, renewal, self-signed fallback, subdomain SSL             |
| [Backups](backups.md)                           | Local and remote (SFTP/FTP) backups, scheduling, restore                         |
| [Security](security.md)                         | nftables firewall, antivirus (ClamAV), password-protected directories            |
| [Git & GitHub Deployment](git-deployment.md)    | Git clone/pull, GitHub repo connect, webhook auto-deploy                         |
| [PHP Management](php.md)                        | Install/remove PHP versions, extensions, IonCube, per-domain settings            |
| [Redis Object Cache](redis.md)                  | Enable/disable per-domain Redis, WordPress auto-integration                      |
| [Service Plans & Customers](plans-customers.md) | Create plans with resource limits, customer accounts, bulk operations            |
| [System & Monitoring](system.md)                | Server usage, load history, process list, service management, package updates    |
| [CLI Utilities](cli-utilities.md)               | All `/usr/local/bin` tools: update, backup, optimize, jail, repair, FTP, Redis   |

## Quick Reference

### Ports

| Port | Service                          |
|------|----------------------------------|
| 80   | nginx (customer HTTP)            |
| 443  | nginx (customer HTTPS)           |
| 8443 | Panel (admin HTTPS, self-signed) |
| 3306 | MariaDB (localhost only)         |

### Paths

| Path                           | Purpose                   |
|--------------------------------|---------------------------|
| `/opt/servika/`                | Panel installation root   |
| `/home/c_<user>/`              | Per-domain customer home  |
| `/etc/nginx/conf.d/`           | nginx vhost configuration |
| `/etc/pki/servika/`            | TLS certificates          |
| `/etc/nftables/servika_fw.nft` | Firewall rules            |
| `/var/named/`                  | BIND DNS zone files       |
| `/var/backups/servika/db/`     | Panel database backups    |

### Service Units

```bash
systemctl status servika     # Panel itself
systemctl status nginx       # Web server
systemctl status mariadb     # Database
systemctl status named       # DNS (BIND)
systemctl status pure-ftpd   # FTP
```
