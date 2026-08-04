package provisioner

import (
	"bytes"
	"strings"
	"testing"
)

func renderCanonical(t *testing.T, opts VhostOpts) string {
	t.Helper()
	var buf bytes.Buffer
	if err := vhostTmpl.Execute(&buf, opts); err != nil {
		t.Fatalf("main vhost render: %v", err)
	}
	if opts.RedirectFromHost() != "" {
		if err := wwwRedirectTmpl.Execute(&buf, opts); err != nil {
			t.Fatalf("canonical redirect render: %v", err)
		}
	}
	return buf.String()
}

func TestServerNamesKeepsBothHostsWhenNoRedirectIsSet(t *testing.T) {
	for _, mode := range []string{"", "off", "unknown"} {
		opts := VhostOpts{DomainName: "example.com", WWWRedirect: mode}
		if got := opts.ServerNames(); got != "example.com www.example.com" {
			t.Errorf("mode %q: ServerNames() = %q, want both hostnames", mode, got)
		}
		if from := opts.RedirectFromHost(); from != "" {
			t.Errorf("mode %q: RedirectFromHost() = %q, want no redirect block", mode, from)
		}
	}
}

func TestCanonicalRedirectMovesOneHostOutOfTheMainVhost(t *testing.T) {
	tests := []struct {
		mode, wantServerNames, wantFrom, wantTo string
	}{
		{mode: "to_www", wantServerNames: "www.example.com", wantFrom: "example.com", wantTo: "www.example.com"},
		{mode: "to_apex", wantServerNames: "example.com", wantFrom: "www.example.com", wantTo: "example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			opts := VhostOpts{DomainName: "example.com", WWWRedirect: tc.mode}
			if got := opts.ServerNames(); got != tc.wantServerNames {
				t.Errorf("ServerNames() = %q, want %q", got, tc.wantServerNames)
			}
			if got := opts.RedirectFromHost(); got != tc.wantFrom {
				t.Errorf("RedirectFromHost() = %q, want %q", got, tc.wantFrom)
			}
			if got := opts.RedirectToHost(); got != tc.wantTo {
				t.Errorf("RedirectToHost() = %q, want %q", got, tc.wantTo)
			}

			config := renderCanonical(t, opts)
			if !strings.Contains(config, "server_name "+tc.wantFrom+";") {
				t.Error("the redirected hostname has no server block, so it would fall through to nginx's default vhost")
			}
			if !strings.Contains(config, "return 301 http://"+tc.wantTo+"$request_uri;") {
				t.Errorf("the redirect does not point at %s", tc.wantTo)
			}
		})
	}
}

// Without a certificate the redirect must stay on http, or every visitor lands on
// a TLS handshake the server cannot complete.
func TestCanonicalRedirectSchemeFollowsTheCertificate(t *testing.T) {
	plain := renderCanonical(t, VhostOpts{DomainName: "example.com", WWWRedirect: "to_www"})
	if strings.Contains(plain, "return 301 https://") {
		t.Error("a vhost with no certificate redirects to https")
	}

	secure := renderCanonical(t, VhostOpts{
		DomainName:  "example.com",
		WWWRedirect: "to_www",
		CertPath:    "/etc/pki/servika/example.com/cert.pem",
		KeyPath:     "/etc/pki/servika/example.com/key.pem",
	})
	if !strings.Contains(secure, "return 301 https://www.example.com$request_uri;") {
		t.Error("a vhost with a certificate does not redirect to https")
	}
	if !strings.Contains(secure, "listen 443 ssl;") {
		t.Error("the redirect block does not listen on 443, so https on the redirected host fails to connect")
	}
}

// Renewal has to be able to answer HTTP-01 for the redirected hostname, or the
// certificate loses that name at the next issuance.
func TestCanonicalRedirectKeepsTheACMEChallengeReachable(t *testing.T) {
	config := renderCanonical(t, VhostOpts{DomainName: "example.com", WWWRedirect: "to_apex"})
	block := config[strings.Index(config, "server_name www.example.com;"):]
	if !strings.Contains(block, "location /.well-known/acme-challenge/") {
		t.Error("the redirect block swallows the ACME challenge path")
	}
}

// A domain whose own name is already a www host has no second hostname to move.
func TestCanonicalRedirectIgnoresAWWWDomain(t *testing.T) {
	opts := VhostOpts{DomainName: "www.example.com", WWWRedirect: "to_www"}
	if got := opts.ServerNames(); got != "www.example.com" {
		t.Errorf("ServerNames() = %q, want the domain unchanged", got)
	}
	if from := opts.RedirectFromHost(); from != "" {
		t.Errorf("RedirectFromHost() = %q, want no redirect block", from)
	}
}
