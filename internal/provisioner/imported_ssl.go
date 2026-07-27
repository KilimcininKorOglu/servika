package provisioner

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrImportedSSLInvalid signals that the source certificate/key pair cannot be
// used safely. The import may leave the account without SSL in that case.
var ErrImportedSSLInvalid = errors.New("invalid source SSL certificate")

// InstallImportedSSL installs a verified PEM certificate chain and private key
// into the system certificate directory. The caller updates the DB first and
// then calls RerenderVhost, so concurrent vhost re-renders cannot overwrite SSL.
func InstallImportedSSL(domainName string, certPEM, keyPEM []byte) (string, string, time.Time, error) {
	if err := ValidateDomain(domainName); err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: %v", ErrImportedSSLInvalid, err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: certificate and private key do not match", ErrImportedSSLInvalid)
	}
	if len(pair.Certificate) == 0 {
		return "", "", time.Time{}, fmt.Errorf("%w: no certificate found", ErrImportedSSLInvalid)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: certificate could not be parsed", ErrImportedSSLInvalid)
	}
	now := time.Now()
	if leaf.NotBefore.After(now.Add(5 * time.Minute)) {
		return "", "", time.Time{}, fmt.Errorf("%w: certificate is not yet valid", ErrImportedSSLInvalid)
	}
	if !leaf.NotAfter.After(now) {
		return "", "", time.Time{}, fmt.Errorf("%w: certificate has expired", ErrImportedSSLInvalid)
	}
	if err := leaf.VerifyHostname(domainName); err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: certificate does not cover %s", ErrImportedSSLInvalid, domainName)
	}
	if len(leaf.ExtKeyUsage) > 0 && !hasServerAuth(leaf.ExtKeyUsage) {
		return "", "", time.Time{}, fmt.Errorf("%w: certificate is not valid for server authentication", ErrImportedSSLInvalid)
	}

	sslDir := certSystemDir(domainName)
	if err := os.MkdirAll(sslDir, 0o755); err != nil {
		return "", "", time.Time{}, err
	}
	certPath := filepath.Join(sslDir, domainName+".crt")
	keyPath := filepath.Join(sslDir, domainName+".key")
	if err := writeImportedPEM(certPath, certPEM, 0o644); err != nil {
		return "", "", time.Time{}, err
	}
	if err := writeImportedPEM(keyPath, keyPEM, 0o600); err != nil {
		_ = os.Remove(certPath)
		return "", "", time.Time{}, err
	}
	if err := applyCertificatePermissions(sslDir, certPath, keyPath); err != nil {
		return "", "", time.Time{}, err
	}
	return certPath, keyPath, leaf.NotAfter, nil
}

func hasServerAuth(usages []x509.ExtKeyUsage) bool {
	for _, usage := range usages {
		if usage == x509.ExtKeyUsageAny || usage == x509.ExtKeyUsageServerAuth {
			return true
		}
	}
	return false
}

// writeImportedPEM writes a PEM file atomically (temp file, fsync, rename) with
// the requested mode so a partial write is never observed at the target path.
func writeImportedPEM(target string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".servika-import-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, target)
}
