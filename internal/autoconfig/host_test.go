package autoconfig

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"database/sql/driver"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var errDBUnavailable = errors.New("connection refused")

type failingConnector struct{}

func (failingConnector) Open(string) (driver.Conn, error) { return nil, errDBUnavailable }

func init() { sql.Register("autoconfig_failing_db", failingConnector{}) }

func failingDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("autoconfig_failing_db", "")
	if err != nil {
		t.Fatalf("open the failing database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// writeMailCertificate installs a certificate covering the given names where
// provisioner.MailCertificateStatus looks for it.
func writeMailCertificate(t *testing.T, root, domain string, notAfter time.Time, hosts ...string) {
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
	body := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, "mail.crt"), body, 0o600); err != nil {
		t.Fatalf("write the certificate: %v", err)
	}
}

// The whole point of measuring is that mail.<domain> is announced only when a
// certificate really covers it. The database is deliberately unreachable here:
// reaching the certificate branch must not depend on it, and if the check were
// removed the panel fallback would need a database this test does not provide.
func TestAnnouncesTheMailHostWhenTheCertificateCoversIt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SERVIKA_CERT_ROOT", root)
	writeMailCertificate(t, root, "example.com", time.Now().Add(30*24*time.Hour),
		"mail.example.com", "smtp.example.com", "imap.example.com")

	h := &Handlers{DB: failingDB(t)}
	host, err := h.announceableHost(httptest.NewRequest("GET", "/", nil), "example.com")
	if err != nil {
		t.Fatalf("announceableHost: %v", err)
	}
	if host != "mail.example.com" {
		t.Errorf("announceableHost = %q, want mail.example.com", host)
	}
}

// An expired certificate covers nothing a client will accept, so it must not
// keep the hostname being announced.
func TestExpiredCertificateDoesNotKeepAnnouncingTheMailHost(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SERVIKA_CERT_ROOT", root)
	writeMailCertificate(t, root, "example.com", time.Now().Add(-time.Hour), "mail.example.com")

	h := &Handlers{DB: failingDB(t)}
	if host, err := h.announceableHost(httptest.NewRequest("GET", "/", nil), "example.com"); err == nil {
		t.Errorf("announceableHost returned %q for an expired certificate", host)
	}
}

// With no certificate the panel fallback needs the database. When that read
// fails the answer must be an error, never a guessed hostname: announcing a name
// nothing covers walks the client into a certificate warning while it is about
// to send a password.
func TestNoHostIsInventedWhenNothingCanBeMeasured(t *testing.T) {
	t.Setenv("SERVIKA_CERT_ROOT", t.TempDir())

	h := &Handlers{DB: failingDB(t)}
	host, err := h.announceableHost(httptest.NewRequest("GET", "/", nil), "example.com")
	if err == nil {
		t.Fatalf("announceableHost returned %q with no certificate and no database", host)
	}
	if host != "" {
		t.Errorf("announceableHost returned %q alongside an error", host)
	}
}

// The endpoints answer for the domain in the Host header, so a request that
// names a domain without mail service here has to be refused rather than served
// with another domain's settings.
func TestUnknownHostIsRefused(t *testing.T) {
	h := &Handlers{DB: failingDB(t)}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/.well-known/autoconfig/mail/config-v1.1.xml", nil)
	request.Host = ""

	h.Thunderbird(recorder, request)
	if recorder.Code != 404 {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
}
