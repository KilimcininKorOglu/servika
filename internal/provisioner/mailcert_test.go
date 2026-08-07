package provisioner

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// writeCertificate places a self-signed certificate for hosts under a domain's
// directory in the certificate root, exactly as an issuance would.
func writeCertificate(t *testing.T, root, domain string, notAfter time.Time, hosts ...string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: hosts[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     hosts,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create a certificate: %v", err)
	}
	dir := filepath.Join(root, domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create the certificate directory: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	for name, body := range map[string][]byte{
		mailCertFile:  certPEM,
		mailKeyFile:   keyPEM,
		mailChainFile: append(append([]byte{}, keyPEM...), certPEM...),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func certRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SERVIKA_CERT_ROOT", root)
	return root
}

// mail. is the MX target; the other two are the names people type into a mail
// client out of habit, and a client checks the certificate against whichever it
// was given. pop. is absent on purpose: no POP3 service runs, and an
// unreachable name in the order fails the whole certificate.
func TestMailHostNamesCoverTheNamesClientsUse(t *testing.T) {
	got := MailHostNames("Example.COM ")
	want := []string{"mail.example.com", "smtp.example.com", "imap.example.com"}
	if !slices.Equal(got, want) {
		t.Errorf("MailHostNames = %v, want %v", got, want)
	}
	for _, host := range got {
		if strings.HasPrefix(host, "pop.") {
			t.Errorf("the mail certificate covers %s, but no POP3 service runs", host)
		}
	}
}

// The certificate itself is the source of truth for the SNI map, so the names
// have to be read back out of it rather than remembered somewhere else.
func TestInstalledCertificatesReportTheNamesTheyCover(t *testing.T) {
	root := certRoot(t)
	writeCertificate(t, root, "example.com", time.Now().Add(60*24*time.Hour),
		"mail.example.com", "smtp.example.com")

	covered := InstalledMailCertificates()
	if !slices.Equal(covered["example.com"], []string{"mail.example.com", "smtp.example.com"}) {
		t.Errorf("covered = %v, want the certificate's own DNS names", covered)
	}
}

// Serving an expired certificate is worse than serving the shared one: the
// client refuses instead of warning. It must drop out of the map by itself.
func TestExpiredCertificatesAreLeftOutOfTheMap(t *testing.T) {
	root := certRoot(t)
	writeCertificate(t, root, "stale.test", time.Now().Add(-24*time.Hour), "mail.stale.test")
	writeCertificate(t, root, "fresh.test", time.Now().Add(24*time.Hour), "mail.fresh.test")

	covered := InstalledMailCertificates()
	if _, present := covered["stale.test"]; present {
		t.Error("an expired certificate was included in the SNI map")
	}
	if _, present := covered["fresh.test"]; !present {
		t.Error("a valid certificate was left out of the SNI map")
	}
}

// A domain with a web certificate but no mail certificate must not appear: the
// map would then point at a file that does not exist and the reload would fail.
func TestDomainsWithoutAMailCertificateAreNotListed(t *testing.T) {
	root := certRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "web-only.test"), 0o755); err != nil {
		t.Fatalf("create the directory: %v", err)
	}
	if len(InstalledMailCertificates()) != 0 {
		t.Error("a domain with no mail chain was listed")
	}
}

// Postfix needs the private key FIRST and then the chain; the other way round it
// rejects the entry and the domain silently falls back to the server-wide
// certificate, which is the problem the whole feature exists to solve.
func TestMailChainStartsWithThePrivateKey(t *testing.T) {
	chain := mailChainBytes([]byte("KEY-PEM"), []byte("CERT-PEM"))
	if string(chain) != "KEY-PEM\nCERT-PEM" {
		t.Errorf("chain = %q, want the key first with a separating newline", chain)
	}
}

// The chain holds a private key, so anything readable beyond root leaks it.
// Ownership is enforced too, which only a root process can verify.
func TestMailChainIsWrittenPrivate(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("writeMailChain chowns the file to root, which requires root")
	}
	root := certRoot(t)
	writeCertificate(t, root, "example.com", time.Now().Add(time.Hour), "mail.example.com")
	certPath, keyPath, chainPath := MailCertificatePaths("example.com")
	if err := writeMailChain(chainPath, certPath, keyPath); err != nil {
		t.Fatalf("writeMailChain: %v", err)
	}
	info, err := os.Stat(chainPath)
	if err != nil {
		t.Fatalf("stat the chain: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("chain mode = %v, want 0600", info.Mode().Perm())
	}
}

// The status the panel shows must come from the file, so a certificate deleted
// or expired on disk cannot keep being reported as installed.
func TestMailCertificateStatusFollowsTheFile(t *testing.T) {
	root := certRoot(t)
	if hosts, _ := MailCertificateStatus("example.com"); hosts != nil {
		t.Errorf("status = %v with nothing installed, want none", hosts)
	}
	writeCertificate(t, root, "example.com", time.Now().Add(48*time.Hour), "mail.example.com")
	hosts, expiry := MailCertificateStatus("example.com")
	if len(hosts) != 1 || hosts[0] != "mail.example.com" || expiry == "" {
		t.Errorf("status = (%v, %q), want the installed certificate", hosts, expiry)
	}
}

// A hostname is only worth announcing when the mail stack will actually present
// a matching certificate for it, and the SNI map is built from the installed
// mail chains alone. Anything else is served the server-wide default, which is a
// mismatch warning at the client.
func TestMailSNICoversOnlyNamesTheInstalledChainsCarry(t *testing.T) {
	root := certRoot(t)
	writeCertificate(t, root, "example.com", time.Now().Add(48*time.Hour),
		"mail.example.com", "imap.example.com")

	for _, host := range []string{"mail.example.com", "IMAP.Example.COM ", "imap.example.com"} {
		if !MailSNICovers(host) {
			t.Errorf("MailSNICovers(%q) = false, want true", host)
		}
	}
	// The other direction: a name no chain carries must be refused, or the check
	// would pass everything and announce a hostname the client warns about.
	for _, host := range []string{"", "smtp.example.com", "panel.hoster.test", "example.com"} {
		if MailSNICovers(host) {
			t.Errorf("MailSNICovers(%q) = true, want false", host)
		}
	}
}

// A certificate without its chain file never reaches the SNI map, so it must not
// make a name look covered either.
func TestMailSNIIgnoresACertificateWithNoChain(t *testing.T) {
	root := certRoot(t)
	writeCertificate(t, root, "example.com", time.Now().Add(48*time.Hour), "mail.example.com")
	if MailSNICovers("mail.example.com") != true {
		t.Fatalf("the fixture did not install a usable chain")
	}
	_, _, chainPath := MailCertificatePaths("example.com")
	if err := os.Remove(chainPath); err != nil {
		t.Fatalf("remove the chain: %v", err)
	}
	if MailSNICovers("mail.example.com") {
		t.Error("a certificate with no chain was reported as covered")
	}
}
