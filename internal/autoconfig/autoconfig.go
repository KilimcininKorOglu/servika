// Package autoconfig answers the two endpoints mail clients probe before asking
// a person for server names and ports.
//
// Thunderbird fetches /.well-known/autoconfig/mail/config-v1.1.xml and Outlook
// POSTs to /autodiscover/autodiscover.xml. Both are served from the domain's OWN
// vhost rather than an autoconfig.<domain> or autodiscover.<domain> host, for
// the same reason webmail is: a subdomain needs an extra A record and an extra
// name on the certificate for every domain, and a client that reaches either one
// over a certificate that does not cover it shows a warning at exactly the
// moment it is about to be handed a password. The domain's existing certificate
// already covers this path.
//
// The trade-off is the older Thunderbird releases that only tried
// autoconfig.<domain>. They fall back to guessing, which reaches the same
// servers because the discovery A records exist.
//
// Neither endpoint can require authentication: a mail client has no panel
// session. They therefore return only what the MX record already reveals, which
// is that this host handles mail for the domain, plus the ports and encryption a
// client would otherwise be told by hand.
package autoconfig

import (
	"database/sql"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"servika/internal/httpx"
	"servika/internal/provisioner"
)

// Handlers serves both endpoints.
type Handlers struct{ DB *sql.DB }

// The ports and encryption below are what this server actually runs, taken from
// assets/mail/dovecot (protocols = imap lmtp, no POP3), assets/mail/postfix
// (submission on 587 with smtpd_tls_security_level=encrypt), and the firewall
// openings in servika-mail-setup (25, 587, 143). Announcing anything else
// produces an account that configures itself and then cannot connect.
const (
	imapPort       = 143
	submissionPort = 587
	// Dovecot's userdb matches on the full address (`WHERE m.email='%u'`), so a
	// bare local part cannot log in.
	usernameIsFullAddress = "%EMAILADDRESS%"
)

// maxAutodiscoverBody bounds the POST Outlook sends. The real request is well
// under a kilobyte; anything larger is not a mail client.
const maxAutodiscoverBody = 8 << 10

var errNoMailHost = errors.New("no mail hostname with a valid certificate")

// Thunderbird answers GET /.well-known/autoconfig/mail/config-v1.1.xml.
//
// The email address Thunderbird appends as a query parameter is ignored: the
// response uses %EMAILADDRESS%, a placeholder the client substitutes itself, so
// no client-supplied text is echoed back at all.
func (h *Handlers) Thunderbird(w http.ResponseWriter, r *http.Request) {
	domain, mailHost, ok := h.resolve(w, r)
	if !ok {
		return
	}

	config := clientConfig{
		Version: "1.1",
		Provider: emailProvider{
			ID:               domain,
			Domain:           domain,
			DisplayName:      domain,
			DisplayShortName: domain,
			Incoming: []serverConfig{{
				Type:           "imap",
				Hostname:       mailHost,
				Port:           imapPort,
				SocketType:     "STARTTLS",
				Username:       usernameIsFullAddress,
				Authentication: "password-cleartext",
			}},
			Outgoing: []serverConfig{{
				Type:           "smtp",
				Hostname:       mailHost,
				Port:           submissionPort,
				SocketType:     "STARTTLS",
				Username:       usernameIsFullAddress,
				Authentication: "password-cleartext",
			}},
		},
	}
	writeXML(w, config)
}

// Outlook answers POST /autodiscover/autodiscover.xml.
//
// Outlook has no username placeholder, so the address it posts has to come back
// in LoginName. It is parsed and checked against the domain being asked about
// before it is used, and the response is produced by the XML encoder rather than
// by pasting strings together, so a crafted address cannot break out of the
// element it belongs in.
func (h *Handlers) Outlook(w http.ResponseWriter, r *http.Request) {
	domain, mailHost, ok := h.resolve(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxAutodiscoverBody))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "request body could not be read")
		return
	}
	address, err := requestedAddress(body)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "the request did not carry a usable email address")
		return
	}
	// The address decides which mailbox the client will try to log in as. If its
	// domain is not the one this vhost serves, answering would hand out settings
	// for somebody else's domain under this domain's name.
	if !strings.EqualFold(addressDomain(address), domain) {
		httpx.WriteError(w, http.StatusNotFound, "this host does not serve that address")
		return
	}

	response := autodiscoverResponse{
		XMLName: xml.Name{Local: "Autodiscover"},
		Xmlns:   autodiscoverResponseNS,
		Response: autodiscoverBody{
			Xmlns: autodiscoverPayloadNS,
			Account: autodiscoverAccount{
				AccountType: "email",
				Action:      "settings",
				Protocols: []autodiscoverProtocol{
					{
						Type: "IMAP", Server: mailHost, Port: imapPort,
						SSL: "on", SPA: "off", AuthRequired: "on", LoginName: address,
					},
					{
						Type: "SMTP", Server: mailHost, Port: submissionPort,
						SSL: "on", SPA: "off", AuthRequired: "on", LoginName: address,
					},
				},
			},
		},
	}
	writeXML(w, response)
}

// resolve turns the requested host into the domain this vhost serves and the
// mail hostname it is safe to announce, or writes the response and reports
// false.
func (h *Handlers) resolve(w http.ResponseWriter, r *http.Request) (domain, mailHost string, ok bool) {
	requested := normalizeHost(r.Host)
	if requested == "" {
		httpx.WriteError(w, http.StatusNotFound, "unknown host")
		return "", "", false
	}
	domain, hosted, err := h.hostedDomainFor(r, requested)
	if err != nil {
		// #nosec G706 -- requested came through normalizeHost, which accepts only the hostname alphabet, so it cannot carry CR/LF or control characters; err is a database driver error.
		log.Printf("autoconfig lookup for %s: %v", requested, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "mail settings are temporarily unavailable")
		return "", "", false
	}
	if !hosted {
		httpx.WriteError(w, http.StatusNotFound, "this host does not serve mail")
		return "", "", false
	}
	mailHost, err = h.announceableHost(r, domain)
	if err != nil {
		// Not an error the client can act on, and not one worth a 500: there is
		// simply no name that would survive the client's certificate check yet.
		httpx.WriteError(w, http.StatusNotFound, "no mail hostname is ready to be announced")
		return "", "", false
	}
	return domain, mailHost, true
}

// hostsMail reports whether the domain has active mail service here. A domain
// that merely exists in the panel is not enough: answering for it would send a
// client at a mailbox that cannot be created.
// hostedDomainFor maps the requested host onto the domain whose mail settings
// answer for it.
//
// Thunderbird asks autoconfig.<domain> and Outlook asks autodiscover.<domain>,
// so those two hosts stand for the domain underneath them. The literal host is
// tried FIRST: a customer may legitimately host a domain that is itself called
// autoconfig.example.com, and that domain's own settings must win over the
// settings of example.com.
func (h *Handlers) hostedDomainFor(r *http.Request, requested string) (domain string, hosted bool, err error) {
	found, err := h.hostsMail(r, requested)
	if err != nil {
		return "", false, err
	}
	if found {
		return requested, true, nil
	}
	for _, prefix := range []string{"autoconfig.", "autodiscover."} {
		if !strings.HasPrefix(requested, prefix) {
			continue
		}
		parent := strings.TrimPrefix(requested, prefix)
		if parent == "" {
			continue
		}
		found, err := h.hostsMail(r, parent)
		if err != nil {
			return "", false, err
		}
		if found {
			return parent, true, nil
		}
	}
	return "", false, nil
}

func (h *Handlers) hostsMail(r *http.Request, domain string) (bool, error) {
	var count int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM mail_domains WHERE domain_name = ? AND status = 'active'`, domain).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// announceableHost returns the hostname to put in the settings.
//
// It is MEASURED, never assumed. mail.<domain> is announced only when the
// domain's own mail certificate covers it; otherwise the panel's custom domain
// is used, and only while its certificate is active. Announcing a name no
// certificate covers would walk the client straight into a warning on its first
// connection, which is the failure this whole endpoint exists to avoid.
func (h *Handlers) announceableHost(r *http.Request, domain string) (string, error) {
	want := "mail." + domain
	covered, _ := provisioner.MailCertificateStatus(domain)
	for _, name := range covered {
		if strings.EqualFold(name, want) {
			return want, nil
		}
	}

	var panelDomain sql.NullString
	var sslStatus string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT custom_domain, ssl_status FROM panel_settings WHERE id = 1`).Scan(&panelDomain, &sslStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if host := strings.TrimSpace(panelDomain.String); host != "" && sslStatus == "active" {
		return strings.ToLower(host), nil
	}
	return "", errNoMailHost
}

// normalizeHost turns the Host header into the domain the vhost serves: no port,
// lower case, and without the www. label, which is the same site.
//
// The result is then checked against the hostname alphabet rather than merely
// having a few dangerous characters removed. The Host header is client-supplied
// and this value goes into a query, a log line and the response body, so the
// only safe rule is an allowlist: anything that is not a hostname is not a
// hostname, and "" makes every caller answer 404.
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if idx := strings.LastIndex(host, ":"); idx != -1 && !strings.Contains(host[idx:], "]") {
		host = host[:idx]
	}
	host = strings.Trim(host, ".")
	host = strings.TrimPrefix(host, "www.")
	if !isHostname(host) {
		return ""
	}
	return host
}

// isHostname reports whether the value is a plausible DNS name: labels of
// letters, digits and hyphens separated by dots, and nothing else. It is
// deliberately stricter than DNS allows, because everything this panel hosts is
// a name ValidateDomain already accepted.
func isHostname(host string) bool {
	if host == "" || len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			default:
				return false
			}
		}
	}
	return true
}

func addressDomain(address string) string {
	if idx := strings.LastIndex(address, "@"); idx != -1 {
		return strings.ToLower(address[idx+1:])
	}
	return ""
}

func writeXML(w http.ResponseWriter, payload any) {
	body, err := xml.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Printf("autoconfig: encode the response: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "mail settings could not be produced")
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	// The answer depends only on the host and is fine to cache briefly, but not
	// long enough to outlive a certificate being issued, which changes which
	// hostname is announced.
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return
	}
	if _, err := w.Write(body); err != nil {
		log.Printf("autoconfig: write the response: %v", err)
	}
}
