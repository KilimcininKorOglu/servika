package subdomain

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"strings"

	"servika/internal/domains"
)

// sslState reports whether a subdomain is actually serving HTTPS, and what kind
// of certificate it is serving.
//
// Both halves of "actually" are needed. Certificate files can sit in ~/ssl
// without the vhost having been switched, and a vhost cannot serve HTTPS without
// them, so reporting on either alone would announce a padlock the browser will
// not draw.
//
// The source is read from the CERTIFICATE rather than from a column, because
// there is no column: nothing records how a subdomain's certificate was
// obtained. Reading the file also answers correctly for certificates that
// predate this code and cannot drift from what is installed, which is the
// failure mode a stored value has.
func sslState(systemUser, subdomainName, fqdn string) (active bool, source string) {
	certPath, keyPath := certificatePaths(systemUser, fqdn)
	if !fileExists(certPath) || !fileExists(keyPath) {
		return false, ""
	}
	// #nosec G703 -- path is built from a validated identifier (systemUser ^c_[A-Za-z0-9_]+$) and a validated FQDN, not from raw tenant input.
	config, err := os.ReadFile(confPath(systemUser, subdomainName))
	if err != nil || !strings.Contains(string(config), "listen 443 ssl;") {
		return false, ""
	}
	return true, certificateSource(certPath)
}

// certificateSource names what kind of certificate certPath holds.
//
// Only the self-signed case is decidable from the file, and it is the only one
// that matters: it is the fail-safe a browser refuses, so it has to be told
// apart from a certificate a browser accepts. A CA-issued certificate returns an
// EMPTY source, which the interface reads as "trusted, origin unknown" rather
// than guessing an issuer the panel cannot verify.
//
// An unreadable or unparseable file is also empty. It is not evidence the
// certificate is self-signed, and claiming so would raise a false alarm on a
// site that is serving perfectly well.
func certificateSource(certPath string) string {
	// #nosec G703 G304 -- path is built from a validated identifier and a validated FQDN, not from raw tenant input.
	raw, err := os.ReadFile(certPath)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return ""
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	if certificate.Issuer.String() == certificate.Subject.String() {
		return domains.SSLSourceSelfSigned
	}
	return ""
}
