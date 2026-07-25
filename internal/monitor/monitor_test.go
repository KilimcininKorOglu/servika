package monitor

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// issueCert creates a certificate for cn/dnsNames signed by the given issuer.
// A nil issuer produces a self-signed certificate.
func issueCert(t *testing.T, cn string, dnsNames []string, isCA bool, issuer *x509.Certificate, issuerKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		DNSNames:              dnsNames,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	signer, signerKey := tmpl, key
	if issuer != nil {
		signer, signerKey = issuer, issuerKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert, key
}

func TestVerifyLeaf(t *testing.T) {
	caCert, caKey := issueCert(t, "Test CA", nil, true, nil, nil)
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	leaf, _ := issueCert(t, "example.com", []string{"example.com"}, false, caCert, caKey)
	selfSigned, _ := issueCert(t, "example.com", []string{"example.com"}, false, nil, nil)

	t.Run("trusted chain and host", func(t *testing.T) {
		ok, msg := verifyLeaf(leaf, nil, "example.com", roots)
		if !ok || msg != "" {
			t.Fatalf("verifyLeaf(trusted) = (%t, %q), want (true, \"\")", ok, msg)
		}
	})

	t.Run("wrong hostname", func(t *testing.T) {
		ok, msg := verifyLeaf(leaf, nil, "other.com", roots)
		if ok || msg == "" {
			t.Fatalf("verifyLeaf(wrong host) = (%t, %q), want (false, non-empty)", ok, msg)
		}
	})

	t.Run("self-signed untrusted", func(t *testing.T) {
		ok, msg := verifyLeaf(selfSigned, nil, "example.com", roots)
		if ok || msg == "" {
			t.Fatalf("verifyLeaf(self-signed) = (%t, %q), want (false, non-empty)", ok, msg)
		}
	})
}
