# Security

## Firewall (nftables)

Servika manages an nftables firewall with the table `servika_fw`. Rules are stored at `/etc/nftables/servika_fw.nft`.

### Viewing Rules

Go to **Firewall** in the admin sidebar. Rules are displayed with:
- IP address or range
- Port (or `*` for all)
- Action (allow/deny)
- Description

### Adding Rules

Click **Add Rule** and provide:

| Field       | Description                                                  |
|-------------|--------------------------------------------------------------|
| IP          | Single IP (e.g. `1.2.3.4`) or CIDR range (e.g. `10.0.0.0/8`) |
| Port        | Port number or `*` for all ports                             |
| Type        | Allow or Deny                                                |
| Description | Optional label                                               |

### Templates

Quick-apply common firewall configurations:

| Template          | Effect                        |
|-------------------|-------------------------------|
| Allow HTTP/HTTPS  | Opens ports 80 and 443 to all |
| Block Brute Force | Blocks common attack IPs      |
| Custom            | Define your own template      |

### Enabling / Disabling Rules

Toggle individual rules on or off without deleting them. Disabled rules are commented out in the nftables configuration.

### Firewall Reapply

The firewall is reapplied at panel startup. If the nftables table is missing or corrupted, it is recreated from the database.

## Antivirus (ClamAV)

Each domain can run on-demand ClamAV scans.

### Starting a Scan

On the domain detail page, go to the **Antivirus** tab and click **Start Scan**. Scans run asynchronously — the panel returns a scan ID for tracking.

### Scan Status

Check progress with the scan ID. The status shows:
- Files scanned
- Threats found
- Scan duration
- Completion status

### Handling Threats

When threats are found, you can:

| Action     | Description                                     |
|------------|-------------------------------------------------|
| Quarantine | Move infected files to a quarantine directory   |
| Delete     | Permanently remove infected files               |
| Ignore     | Mark as false positive and skip in future scans |

### Updating Signatures

Update ClamAV virus definitions from the panel with **Update Signatures** (`freshclam`).

## Password-Protected Directories

Add HTTP Basic Authentication to any directory within a domain.

### Adding Protection

On the domain detail page, go to the **Directory Protection** tab and click **Add**. Provide:

| Field    | Description                                    |
|----------|------------------------------------------------|
| Path     | Directory relative to web root (e.g. `/admin`) |
| Username | HTTP auth username                             |
| Password | HTTP auth password                             |

The panel creates an `.htpasswd` file and updates the nginx vhost to include the `auth_basic` directives.

### Managing Protection

- **List** — View all protected directories for a domain
- **Delete** — Remove protection from a directory

## 2FA (Two-Factor Authentication)

Administrators can enable TOTP-based 2FA:

1. Go to **Profile > 2FA Setup**
2. Scan the QR code with an authenticator app
3. Enter the verification code to confirm
4. 2FA is now required at every login

To disable 2FA, go to **Profile > Disable 2FA** and confirm with your current password.

## Security Best Practices

- Enable 2FA for all admin accounts
- Change the default root password
- Keep the panel updated (`servika-update`)
- Review firewall rules regularly
- Enable automatic ClamAV signature updates
- Use SFTP instead of FTP for remote backup destinations
- Restrict SSH access to specific keys
