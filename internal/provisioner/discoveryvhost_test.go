package provisioner

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeSANCertificate puts a certificate naming exactly hosts on disk and
// returns its paths.
func writeSANCertificate(t *testing.T, hosts ...string) (certPath, keyPath string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: hosts[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		DNSNames:     hosts,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create a certificate: %v", err)
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write the certificate: %v", err)
	}
	if err := os.WriteFile(keyPath,
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatalf("write the key: %v", err)
	}
	return certPath, keyPath
}

func discoveryOpts(t *testing.T, hosts ...string) VhostOpts {
	t.Helper()
	certPath, keyPath := writeSANCertificate(t, hosts...)
	return VhostOpts{
		DomainName: "example.com",
		WebRoot:    "/home/example/public_html",
		PHPSocket:  "/run/php-fpm/example.sock",
		Backend:    "php-fpm",
		CertPath:   certPath,
		KeyPath:    keyPath,
	}
}

// The block appears only when the certificate names both hosts.
//
// Answering a name the certificate omits is worse than not answering it: the
// client hits a name mismatch on the very connection it makes to learn where to
// send a mailbox password, while an unanswered name just moves it on to the next
// lookup in its own order.
func TestDiscoveryVhostNeedsTheCertificateToNameBothHosts(t *testing.T) {
	cases := []struct {
		name  string
		hosts []string
		want  bool
	}{
		{"both names covered", []string{"example.com", "autoconfig.example.com", "autodiscover.example.com"}, true},
		{"only autoconfig covered", []string{"example.com", "autoconfig.example.com"}, false},
		{"neither name covered", []string{"example.com", "www.example.com"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := discoveryOpts(t, tc.hosts...)
			if got := opts.discoveryEligible(); got != tc.want {
				t.Errorf("discoveryEligible() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A domain without TLS, and a suspended one, never get the block.
func TestDiscoveryVhostIsRefusedWithoutTLSOrWhileSuspended(t *testing.T) {
	covered := []string{"example.com", "autoconfig.example.com", "autodiscover.example.com"}

	plain := discoveryOpts(t, covered...)
	plain.CertPath, plain.KeyPath = "", ""
	if plain.discoveryEligible() {
		t.Error("a vhost without TLS was given the auto-configuration block")
	}

	suspended := discoveryOpts(t, covered...)
	suspended.Suspended = true
	if suspended.discoveryEligible() {
		t.Error("a suspended domain was given the auto-configuration block")
	}
}

func TestDiscoveryVhostServesOnlyTheAutoConfigurationPaths(t *testing.T) {
	opts := discoveryOpts(t, "example.com", "autoconfig.example.com", "autodiscover.example.com")
	var rendered bytes.Buffer
	if err := discoveryVhostTmpl.Execute(&rendered, opts); err != nil {
		t.Fatalf("render the auto-configuration vhost: %v", err)
	}
	body := rendered.String()

	if !strings.Contains(body, "server_name autoconfig.example.com autodiscover.example.com;") {
		t.Errorf("the vhost does not name both hosts:\n%s", body)
	}
	if !strings.Contains(body, autoconfigMarker) {
		t.Error("the vhost does not answer the Thunderbird path")
	}
	// Without this the customer's site would be published under two hostnames it
	// was never given, on a certificate issued for auto-configuration alone.
	if !strings.Contains(body, "location / { return 404; }") {
		t.Error("the vhost falls through to the site instead of answering 404")
	}
	// Plain HTTP would hand a client the address to send a password to on the
	// word of anyone on the path.
	if strings.Contains(body, "listen 80;") {
		t.Error("the vhost listens on plain HTTP")
	}
}

// The block is appended to a live vhost file, so a syntax error in it takes the
// whole host down at the next reload.
func TestDiscoveryVhostIsValidNginxSyntax(t *testing.T) {
	opts := discoveryOpts(t, "example.com", "autoconfig.example.com", "autodiscover.example.com")
	var rendered bytes.Buffer
	if err := discoveryVhostTmpl.Execute(&rendered, opts); err != nil {
		t.Fatalf("render the auto-configuration vhost: %v", err)
	}
	prefix := t.TempDir()
	// The ssl directives need a real pair, which the rendered block already
	// points at; only the listen port is lowered so the test needs no privilege.
	body := "events {}\nhttp {\n" +
		strings.ReplaceAll(rendered.String(), "listen 443 ssl;", "listen 8443 ssl;") + "}\n"
	body = strings.ReplaceAll(body, "listen [::]:443 ssl;", "")
	// The log paths belong to a live host; point them at the sandbox so the check
	// measures the block's syntax rather than the test machine's filesystem.
	body = strings.ReplaceAll(body, "/var/log/nginx/", prefix+"/")
	checkNginxSyntax(t, prefix, body, "the auto-configuration vhost is not valid nginx syntax")
}

// The repair recognises its own work through this marker; a rename that left it
// behind would turn the repair into a silent no-op.
func TestDiscoveryRepairMarkerMatchesTheRenderedVhost(t *testing.T) {
	if !strings.Contains(discoveryVhostNginx, discoveryMarker) {
		t.Errorf("%q does not appear in the auto-configuration vhost", discoveryMarker)
	}
}
