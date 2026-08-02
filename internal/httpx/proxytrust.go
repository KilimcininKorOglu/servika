package httpx

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"sync"
)

// ProxySecretPath is the file shared with nginx. The provisioner heal writes it
// only AFTER it has injected `proxy_set_header X-Servika-Proxy "<secret>"` into
// the panel vhost and `nginx -t` has passed. So if the file exists, nginx is
// guaranteed to send the header too (otherwise ClientIP falls back to the old
// loopback-trust behaviour and nobody is locked out).
const ProxySecretPath = "/etc/servika/proxy.secret" // #nosec G101 -- filesystem path, not a credential

var (
	proxyOnce sync.Once
	proxyVal  string
)

// ProxySecret returns the persistent secret (when present), else "". It only
// READS, never generates — production + nginx synchronisation is the provisioner
// heal's job (for the fail-safe ordering described on ProxySecretPath).
func ProxySecret() string {
	proxyOnce.Do(func() {
		if b, err := os.ReadFile(ProxySecretPath); err == nil {
			if t := strings.TrimSpace(string(b)); len(t) >= 32 {
				proxyVal = t
			}
		}
	})
	return proxyVal
}

// NewProxySecret generates a 256-bit random hex secret (used by the provisioner heal).
func NewProxySecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

// hasProxySecretHeader reports whether the request really came from nginx (it
// carries the shared secret) using a constant-time comparison.
func hasProxySecretHeader(r *http.Request, secret string) bool {
	got := strings.TrimSpace(r.Header.Get("X-Servika-Proxy"))
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}
