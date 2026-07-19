# DNS Management

Servika includes a full BIND DNS server. Each domain can have its own DNS zone with records managed through the panel.

## DNS Zone

Navigate to a domain detail page, then the **DNS** tab.

### Supported Record Types

| Type  | Description     | Example value                       |
|-------|-----------------|-------------------------------------|
| A     | IPv4 address    | `192.168.1.1`                       |
| AAAA  | IPv6 address    | `2001:db8::1`                       |
| CNAME | Canonical name  | `www.example.com.`                  |
| MX    | Mail exchange   | `mail.example.com.` (with priority) |
| TXT   | Text record     | `v=spf1 mx ~all`                    |
| NS    | Name server     | `ns1.example.com.`                  |
| SRV   | Service record  | With priority, weight, port         |
| PTR   | Reverse pointer | `1.0.0.127.in-addr.arpa.`           |

### Adding Records

Click **Add Record**, choose the type, fill in name, value, TTL, and priority (for MX/SRV). Click Save.

### Editing and Deleting

Click a record row to edit it inline. Delete individual records or use **Bulk Delete** to remove multiple records at once. Use **Bulk Status** to enable/disable multiple records.

### Applying the Default Template

The **Apply Template** button seeds the zone with a standard set of records (A, MX, SPF, NS) based on the central DNS template. This is idempotent — existing identical records are skipped.

## DNS Template (Admin)

Admins can customize the default DNS template at **Tools & Settings > DNS Template**. The template defines which records are created when a new domain is provisioned or when **Apply Template** is clicked.

Template records support placeholders:

| Placeholder  | Replaced with             |
|--------------|---------------------------|
| `{DOMAIN}`   | The domain name           |
| `{IP}`       | The server's public IPv4  |
| `{SELECTOR}` | DKIM selector             |
| `{DKIM}`     | DKIM public key TXT value |

### SOA Defaults

The template also defines SOA record defaults: primary NS, hostmaster, refresh, retry, expire, minimum, and TTL values.

### DKIM

When DKIM is enabled in the template, a 2048-bit RSA key pair is generated per domain. The public key is published as a TXT record, and the private key is stored in the database. If OpenDKIM is installed, keys are automatically synced to `/etc/opendkim/keys/`.

## SOA Record

View and edit the Start of Authority record for each domain. The SOA defines zone-wide parameters:

| Field      | Typical value             | Description                         |
|------------|---------------------------|-------------------------------------|
| Primary NS | `ns1.example.com.`        | Primary name server                 |
| Hostmaster | `hostmaster.example.com.` | Admin email (dot instead of @)      |
| Refresh    | `3600`                    | Secondary server refresh interval   |
| Retry      | `900`                     | Retry interval after failed refresh |
| Expire     | `1209600`                 | Zone expiry if unreachable          |
| Minimum    | `3600`                    | Negative caching TTL                |
| TTL        | `3600`                    | Default record TTL                  |

## DNSSEC

DNSSEC can be enabled per domain. When enabled, BIND uses its built-in `default` DNSSEC policy with inline signing. Keys are stored in `/var/named/dynamic/`.

Enable or disable DNSSEC from the DNS tab of the domain detail page.

## Zone File Generation

Zone files are written to `/var/named/<domain>.zone` and validated with `named-checkzone` before deployment. The generated include file at `/etc/named/servika-zones.conf` is also validated with `named-checkconf`.

Zone changes trigger a BIND reload via `rndc reload`. If `rndc` is unavailable, the panel falls back to `systemctl reload named`, then `systemctl restart named` as a last resort.
