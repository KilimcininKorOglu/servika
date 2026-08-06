package provisioner

import (
	"strings"
	"testing"
)

// alwaysCovers and neverCovers stand in for CertificateCoversHost so the
// decision can be exercised without a certificate on disk.
func alwaysCovers(string, string, string) bool { return true }
func neverCovers(string, string, string) bool  { return false }

func certifiedOpts(mode string) VhostOpts {
	return VhostOpts{
		DomainName:  "example.com",
		WWWRedirect: mode,
		CertPath:    "/etc/pki/servika/example.com/cert.pem",
		KeyPath:     "/etc/pki/servika/example.com/key.pem",
	}
}

// The template turns the 301 into an https one as soon as a certificate exists,
// so a redirect to a host the certificate does not name sends every visitor into
// a certificate error before they reach the site. Both hostnames serving
// directly is a lost preference; a certificate error is an outage.
func TestACanonicalRedirectTheCertificateDoesNotCoverIsDropped(t *testing.T) {
	for _, mode := range []string{"to_www", "to_apex"} {
		t.Run(mode, func(t *testing.T) {
			kept := withCertifiableCanonicalRedirect(certifiedOpts(mode), neverCovers)
			if kept.WWWRedirect != "" {
				t.Fatalf("WWWRedirect = %q, want it dropped", kept.WWWRedirect)
			}
			if kept.RedirectToHost() != "" || kept.RedirectFromHost() != "" {
				t.Error("the redirect hosts survived, so the block would still render")
			}
			// Both hostnames must go back onto the main vhost, or the one that
			// was moved out has no server block at all.
			if got := kept.ServerNames(); got != "example.com www.example.com" {
				t.Errorf("ServerNames() = %q, want both hostnames", got)
			}
		})
	}
}

// The whole point of the setting is to work when it can, so a certificate that
// does name the target must be left alone.
func TestACoveredCanonicalRedirectSurvives(t *testing.T) {
	for _, mode := range []string{"to_www", "to_apex"} {
		t.Run(mode, func(t *testing.T) {
			kept := withCertifiableCanonicalRedirect(certifiedOpts(mode), alwaysCovers)
			if kept.WWWRedirect != mode {
				t.Fatalf("WWWRedirect = %q, want %q", kept.WWWRedirect, mode)
			}
			config := renderCanonical(t, kept)
			if !strings.Contains(config, "return 301 https://"+kept.RedirectToHost()+"$request_uri;") {
				t.Error("the covered redirect was not rendered")
			}
		})
	}
}

// A site with no certificate redirects over http, so no certificate is involved
// and the check must not run. Dropping the redirect here would break a working
// HTTP-only configuration for no reason.
func TestAnHTTPOnlySiteKeepsItsCanonicalRedirect(t *testing.T) {
	plain := VhostOpts{DomainName: "example.com", WWWRedirect: "to_www"}
	kept := withCertifiableCanonicalRedirect(plain, neverCovers)
	if kept.WWWRedirect != "to_www" {
		t.Fatalf("WWWRedirect = %q, want it kept on an HTTP-only site", kept.WWWRedirect)
	}
	config := renderCanonical(t, kept)
	if !strings.Contains(config, "return 301 http://www.example.com$request_uri;") {
		t.Error("the http redirect was not rendered")
	}
}

// The real check, against a real certificate, so the decision is not only proven
// against a stand-in that could disagree with CertificateCoversHost.
func TestTheRealCertificateCheckDecidesTheSameWay(t *testing.T) {
	t.Run("a certificate naming only the apex loses the www redirect", func(t *testing.T) {
		certPath, keyPath := writeSANCertificate(t, "example.com")
		opts := VhostOpts{
			DomainName: "example.com", WWWRedirect: "to_www",
			CertPath: certPath, KeyPath: keyPath,
		}
		if kept := withCertifiableCanonicalRedirect(opts, CertificateCoversHost); kept.WWWRedirect != "" {
			t.Error("a certificate that does not name www still carried the redirect to it")
		}
	})

	t.Run("a certificate naming both keeps it", func(t *testing.T) {
		certPath, keyPath := writeSANCertificate(t, "example.com", "www.example.com")
		opts := VhostOpts{
			DomainName: "example.com", WWWRedirect: "to_www",
			CertPath: certPath, KeyPath: keyPath,
		}
		if kept := withCertifiableCanonicalRedirect(opts, CertificateCoversHost); kept.WWWRedirect != "to_www" {
			t.Error("a certificate naming both hostnames lost the redirect")
		}
	})
}

// The shapes that keep both hostnames on one server_name clear WWWRedirect
// before this runs, so there must be nothing left for it to decide about.
func TestTheCheckIsANoOpOnceTheRedirectIsAlreadyCleared(t *testing.T) {
	opts := certifiedOpts("")
	if kept := withCertifiableCanonicalRedirect(opts, neverCovers); kept.WWWRedirect != "" {
		t.Errorf("WWWRedirect = %q, want it to stay empty", kept.WWWRedirect)
	}
}
