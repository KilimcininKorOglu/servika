package subdomain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

var managedUserPattern = regexp.MustCompile(`^c_[a-z0-9_]{1,26}$`)

func sslDirectory(systemUser string) string {
	return filepath.Join("/home", systemUser, "ssl")
}

func certificatePaths(systemUser, fqdn string) (string, string) {
	directory := sslDirectory(systemUser)
	return filepath.Join(directory, fqdn+".crt"), filepath.Join(directory, fqdn+".key")
}

func (h *Handlers) subInfo(r *http.Request, domainID int64, parentDomain string) (string, string, string, bool) {
	subdomainID, err := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
	if err != nil || subdomainID <= 0 {
		return "", "", "", false
	}
	var name, fqdn, phpVersion string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT subdomain, fqdn, COALESCE(php_version,'8.3') FROM subdomains WHERE id=? AND domain_id=?`,
		subdomainID, domainID).Scan(&name, &fqdn, &phpVersion); err != nil {
		return "", "", "", false
	}
	if !subdomainPattern.MatchString(name) || fqdn != name+"."+parentDomain || provisioner.ValidateDomain(fqdn) != nil {
		return "", "", "", false
	}
	return name, fqdn, phpVersion, true
}

func validSSLType(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "self-signed", true
	}
	return value, value == "self-signed" || value == "letsencrypt"
}

// SSLStatus reports whether a subdomain has certificate files and an HTTPS vhost.
func (h *Handlers) SSLStatus(w http.ResponseWriter, r *http.Request) {
	domainID, systemUser, parentDomain, _, _, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if !managedUserPattern.MatchString(systemUser) {
		httpx.WriteError(w, http.StatusInternalServerError, "invalid domain configuration")
		return
	}
	name, fqdn, _, ok := h.subInfo(r, domainID, parentDomain)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	certPath, keyPath := certificatePaths(systemUser, fqdn)
	config, _ := os.ReadFile(confPath(systemUser, name))
	active := fileExists(certPath) && fileExists(keyPath) && strings.Contains(string(config), "listen 443 ssl;")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"active": active})
}

// SSLIssue installs a self-signed or Let's Encrypt certificate for a subdomain.
func (h *Handlers) SSLIssue(w http.ResponseWriter, r *http.Request) {
	domainID, systemUser, parentDomain, _, demo, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "not available for demo subscriptions")
		return
	}
	if !managedUserPattern.MatchString(systemUser) {
		httpx.WriteError(w, http.StatusInternalServerError, "invalid domain configuration")
		return
	}
	name, fqdn, phpVersion, ok := h.subInfo(r, domainID, parentDomain)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	var request struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	certificateType, valid := validSSLType(request.Type)
	if !valid {
		httpx.WriteError(w, http.StatusBadRequest, "type must be self-signed or letsencrypt")
		return
	}
	socket, err := provisioner.PHPSocketFor(systemUser, phpVersion)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "pHP version is not installed on the server")
		return
	}
	certPath, keyPath := certificatePaths(systemUser, fqdn)
	if err := os.MkdirAll(sslDirectory(systemUser), 0o750); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not prepare certificate directory")
		return
	}

	if certificateType == "letsencrypt" {
		err = issueLetsEncrypt(fqdn, certPath, keyPath)
	} else {
		err = issueSelfSigned(fqdn, certPath, keyPath)
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "sSL installation failed")
		return
	}
	_ = os.Chmod(keyPath, 0o640)
	_ = exec.Command("chown", "-R", systemUser+":"+systemUser, sslDirectory(systemUser)).Run()
	_ = exec.Command("restorecon", "-R", sslDirectory(systemUser)).Run()

	config := vhostSSL(fqdn, docrootOf(systemUser, fqdn), socket, certPath, keyPath)
	if err := applyVhost(confPath(systemUser, name), config); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "sSL installation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "fqdn": fqdn, "type": certificateType})
}

// SSLRemove removes a subdomain certificate and restores its HTTP-only vhost.
func (h *Handlers) SSLRemove(w http.ResponseWriter, r *http.Request) {
	domainID, systemUser, parentDomain, _, demo, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "not available for demo subscriptions")
		return
	}
	if !managedUserPattern.MatchString(systemUser) {
		httpx.WriteError(w, http.StatusInternalServerError, "invalid domain configuration")
		return
	}
	name, fqdn, phpVersion, ok := h.subInfo(r, domainID, parentDomain)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	socket, err := provisioner.PHPSocketFor(systemUser, phpVersion)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "pHP version is not installed on the server")
		return
	}
	if err := applyVhost(confPath(systemUser, name), vhost(fqdn, docrootOf(systemUser, fqdn), socket)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not disable SSL")
		return
	}
	certPath, keyPath := certificatePaths(systemUser, fqdn)
	_ = os.Remove(certPath)
	_ = os.Remove(keyPath)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func issueSelfSigned(fqdn, certPath, keyPath string) error {
	return exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "365",
		"-keyout", keyPath, "-out", certPath, "-subj", "/CN="+fqdn, "-addext", "subjectAltName=DNS:"+fqdn).Run()
}

func issueLetsEncrypt(fqdn, certPath, keyPath string) error {
	if err := os.MkdirAll("/var/www/_acme", 0o755); err != nil {
		return err
	}
	_ = exec.Command("restorecon", "-R", "/var/www/_acme").Run()
	if err := exec.Command("/root/.acme.sh/acme.sh", "--issue", "--webroot", "/var/www/_acme",
		"-d", fqdn, "--keylength", "ec-256").Run(); err != nil {
		return err
	}
	return exec.Command("/root/.acme.sh/acme.sh", "--install-cert", "-d", fqdn, "--ecc",
		"--key-file", keyPath, "--fullchain-file", certPath).Run()
}

func applyVhost(path, config string) error {
	previous, readErr := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		return err
	}
	_ = exec.Command("restorecon", path).Run()
	rollback := func() {
		if readErr == nil {
			_ = os.WriteFile(path, previous, 0o644)
		} else {
			_ = os.Remove(path)
		}
	}
	if err := exec.Command("nginx", "-t").Run(); err != nil {
		rollback()
		return err
	}
	if err := exec.Command("systemctl", "reload", "nginx").Run(); err != nil {
		rollback()
		_ = exec.Command("systemctl", "reload", "nginx").Run()
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func vhostSSL(fqdn, docroot, socket, certPath, keyPath string) string {
	return fmt.Sprintf(`server {
    listen 80;
    listen [::]:80;
    server_name %[1]s;
    location /.well-known/acme-challenge/ { root /var/www/_acme; try_files $uri =404; }
    location / { return 301 https://$host$request_uri; }
}
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name %[1]s;

    ssl_certificate     %[4]s;
    ssl_certificate_key %[5]s;
    ssl_protocols TLSv1.2 TLSv1.3;

    root %[2]s;
    index index.php index.html index.htm;
    access_log /var/log/nginx/%[1]s.access.log;
    error_log  /var/log/nginx/%[1]s.error.log warn;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    location / { try_files $uri $uri/ /index.php?$query_string; }
    location ~ \.php$ {
        try_files $uri =404;
        fastcgi_split_path_info ^(.+\.php)(/.+)$;
        fastcgi_pass unix:%[3]s;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param HTTPS on;
        fastcgi_read_timeout 60s;
    }
    location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2?|svg|webp|avif|pdf|zip|gz)$ {
        expires 30d;
        access_log off;
    }
    location ~ /\.(?!well-known) { deny all; }
}
`, fqdn, docroot, socket, certPath, keyPath)
}
