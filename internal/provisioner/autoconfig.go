package provisioner

// Mail client auto-configuration on the customer's OWN domain.
//
// Thunderbird probes /.well-known/autoconfig/mail/config-v1.1.xml and Outlook
// POSTs /autodiscover/autodiscover.xml, and both try the mail domain itself
// before any autoconfig./autodiscover. subdomain. Serving them here means the
// domain's existing certificate already covers the request, so no extra A record
// and no extra certificate name is needed per domain. The panel decides what to
// answer, including whether the domain hosts mail at all.
//
// Like the webmail block, nothing here defines add_header, so both locations
// inherit the domain's own server-level security headers instead of silently
// dropping them.
const autoconfigNginx = `
    # ---- Mail client auto-configuration (answered by the panel) ----
    location = /.well-known/autoconfig/mail/config-v1.1.xml {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 15s;
    }

    # Outlook capitalises the path as /AutoDiscover/AutoDiscover.xml, so the
    # match is case-insensitive. proxy_pass may not carry a URI inside a regex
    # location, hence the rewrite to the canonical spelling the panel routes on.
    location ~* ^/autodiscover/autodiscover\.xml$ {
        rewrite ^ /autodiscover/autodiscover.xml break;
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 15s;
    }
`

// autoconfigBlock returns the block for a vhost.
//
// It is rendered only on the TLS vhost, by the same reasoning as webmail: these
// endpoints tell a client where to send a mailbox password, and a client that
// read them over plain HTTP would have taken that instruction from anyone on the
// path. Outlook refuses an unencrypted answer outright.
func autoconfigBlock() string { return autoconfigNginx }
