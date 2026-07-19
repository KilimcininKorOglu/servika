# Domain Management

## Creating a Domain

Go to **Domains > Add Domain**. Fill in:

| Field        | Description                                   |
|--------------|-----------------------------------------------|
| Domain name  | e.g. `example.com`                            |
| PHP version  | Choose from installed versions                |
| Service plan | Resource limits (Starter selected by default) |
| Notes        | Optional admin notes                          |

Each domain creates:
- A Linux system user (`c_<id>`)
- Home directory at `/home/c_<user>/`
- nginx vhost (HTTP + HTTPS)
- PHP-FPM pool on a per-domain socket
- DNS zone (if DNS management is enabled)

## Domain Settings

Click a domain row to open its detail page, then use the tabs:

### General

- **Suspend / Resume** — Suspend takes the site offline (returns 503). Admin-only.
- **Delete** — Removes the domain, its files, databases, and user. Irreversible.
- **Change Owner** — Bulk-transfer domains to another customer. Admin-only.

### PHP

Set the PHP version per domain. The change takes effect immediately by rewriting the nginx vhost to point to the new FPM socket.

### Web Backend

Choose the HTTP backend for the domain:

| Backend           | Use case                                      |
|-------------------|-----------------------------------------------|
| PHP-FPM (socket)  | Standard PHP sites (WordPress, Laravel, etc.) |
| Static files only | No PHP processing                             |
| Reverse proxy     | Proxy to a custom port (Node.js, Go, Python)  |

### SSH Access

Enable SSH access for a domain user. When enabled:
- A chroot jail is created (`servika-jail`)
- The user can `scp`/`sftp` files
- The user cannot see other domains' files

SSH key authentication is supported. Upload a public key in the SSH tab.

### FTP

Each domain gets an FTP account. Use the **Set Password** action to change the FTP password. The FTP host and username are shown in the domain detail card.

Connect with any standard FTP client:

```
Host: <domain FTP host>
User: <domain FTP user>
Port: 21
```

## Subdomains

Navigate to a domain detail page, then the **Subdomains** tab.

### Add a Subdomain

Enter the subdomain prefix (e.g. `blog` for `blog.example.com`). Each subdomain:
- Shares the parent domain's PHP-FPM pool
- Gets its own document root at `<parent_root>/<prefix>`
- Can have its own SSL certificate

### SSL for Subdomains

Each subdomain can issue its own Let's Encrypt certificate. Navigate to the subdomain row and use the SSL actions.

## Bulk Operations

Admin-only operations available from the domains list:

| Operation          | Description                                |
|--------------------|--------------------------------------------|
| Bulk Change Owner  | Reassign multiple domains to a customer    |
| Bulk Change Status | Suspend or resume multiple domains at once |

## Calculating Disk Usage

The panel can recalculate disk usage for a domain. This runs `du` on the domain's home directory and updates the stored size. Trigger it from the domain detail page or let the periodic sampler run automatically.
