# SSL / TLS

Servika manages TLS certificates through Let's Encrypt with automatic renewal and self-signed fallback.

## Issuing a Certificate

On the domain detail page, go to the **SSL** tab and click **Issue Certificate**.

Requirements:
- The domain must resolve to the server's IP (DNS A record must be set)
- Port 80 must be reachable from the internet (Let's Encrypt HTTP-01 challenge)

The panel uses acme.sh with these safety measures:

| Measure                | Description                                                                                                                                                       |
|------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Reuse-before-issue** | If a valid certificate exists (not expiring within 30 days), it is reused instead of requesting a new one                                                         |
| **Fail-safe**          | If issuance fails (including Let's Encrypt 429 rate limits), the existing certificate or a fresh self-signed one is used — port 443 is never dropped to HTTP-only |
| **No --force**         | acme.sh is never called with `--force`, preventing unnecessary issuance that triggers rate limits                                                                 |

## Certificate Storage

Certificates are stored under `/etc/pki/servika/<domain>/` with root ownership:

- Certificate: mode `0644`
- Private key: mode `0600`
- Full chain: `fullchain.pem`

## Renewal

acme.sh handles automatic renewal via its built-in cron job. The panel does not need to trigger renewals — acme.sh checks expiry and renews when necessary.

## Disabling SSL

Removing SSL reverts the domain to HTTP-only on port 80. The certificate files are not deleted from disk, so re-issuing later is faster.

## Subdomain SSL

Each subdomain can have its own SSL certificate. Navigate to **Domains > Subdomains**, find the subdomain row, and use the SSL actions.

## SSL Status

The SSL status endpoint shows:
- Whether SSL is enabled
- Certificate expiry date
- Issuer (Let's Encrypt or self-signed)
- Certificate and key paths

## Startup Repair

At startup, the panel runs `HealSSLVhost443OnStartup` to:
- Find all domains with `ssl_enabled = 1`
- Validate each domain's certificate and key exist
- Re-render missing or broken 443 vhost blocks

This automatically repairs SSL configuration after a panel update or nginx configuration change.

## Manual Certificate Replacement

To replace a certificate manually, place the certificate files in `/etc/pki/servika/<domain>/` and click **Issue** in the panel. The reuse-before-issue check detects the valid certificate and deploys it without contacting Let's Encrypt.
