package subdomain

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"servika/internal/domains"
)

// writeCertificate writes a PEM certificate whose issuer is issuerName and whose
// subject is subjectName. Equal names are what a self-signed certificate has,
// and different ones are what a CA leaves behind.
func writeCertificate(t *testing.T, path, issuerName, subjectName string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: subjectName},
		Issuer:       pkix.Name{CommonName: issuerName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	// Self-signing against a template whose Issuer differs from its Subject
	// produces exactly the shape the reader has to tell apart, without needing a
	// second key pair: x509.CreateCertificate copies the PARENT's subject into
	// the issuer field, so the parent here is the template itself with the issuer
	// name as its subject.
	parent := &x509.Certificate{Subject: pkix.Name{CommonName: issuerName}}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create the certificate: %v", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write the certificate: %v", err)
	}
}

// The panel records nothing about how a subdomain's certificate was obtained, so
// the answer comes from the file. Only the self-signed case is decidable there,
// and it is the one that matters: it is the fail-safe a browser refuses.
func TestOnlyASelfSignedCertificateIsNamedAsOne(t *testing.T) {
	directory := t.TempDir()

	selfSigned := filepath.Join(directory, "self.crt")
	writeCertificate(t, selfSigned, "sub.example.com", "sub.example.com")
	if got := certificateSource(selfSigned); got != domains.SSLSourceSelfSigned {
		t.Errorf("self-signed source = %q, want %q", got, domains.SSLSourceSelfSigned)
	}

	// A CA-issued certificate returns EMPTY rather than a guessed issuer name:
	// the panel cannot verify which authority it came from, and the interface
	// reads an empty source as trusted with an unrecorded origin.
	issued := filepath.Join(directory, "issued.crt")
	writeCertificate(t, issued, "Example Authority R3", "sub.example.com")
	if got := certificateSource(issued); got != "" {
		t.Errorf("CA-issued source = %q, want empty", got)
	}
}

// A file that cannot be read or parsed is not evidence of anything. Answering
// "self-signed" would put a browser warning on a site that is serving fine.
func TestAnUnreadableCertificateIsNotCalledSelfSigned(t *testing.T) {
	directory := t.TempDir()

	garbage := filepath.Join(directory, "garbage.crt")
	if err := os.WriteFile(garbage, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	brokenPEM := filepath.Join(directory, "broken.crt")
	if err := os.WriteFile(brokenPEM,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{0x30, 0x00}}), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		filepath.Join(directory, "missing.crt"),
		garbage,
		brokenPEM,
	} {
		if got := certificateSource(path); got != "" {
			t.Errorf("certificateSource(%q) = %q, want empty", filepath.Base(path), got)
		}
	}
}
