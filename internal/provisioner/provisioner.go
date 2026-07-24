// Package provisioner manages Linux users, nginx vhosts, multi-version PHP-FPM, and SSL/TLS for domains.
package provisioner

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"

	"servika/internal/config"
)

var (
	domainNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,251}\.[a-z]{2,24}$`)
	tenantUserPattern = regexp.MustCompile(`^c_[a-z0-9_]+$`)
	slugSan           = regexp.MustCompile(`[^a-z0-9]+`)
	packageDB         *sql.DB
)

const (
	cacheZoneName          = "servikacache"
	maxCertificateFileSize = 1 << 20
	cacheLogFormatName     = "servika_cache_status"
)

func cacheZoneDir() string { return config.NginxCacheDir() }

func cacheZoneConf() string { return config.NginxCacheConf() }

func cacheZoneTempConf() string { return config.NginxCacheTempConf() }

func certSystemBaseDir() string { return config.CertRoot() }

func cacheZoneBody() string {
	return `# Managed automatically by Servika. DO NOT EDIT.
# Vhosts use "fastcgi_cache servikacache"; this file provides the matching zone definition.
fastcgi_cache_path ` + cacheZoneDir() + ` levels=1:2 keys_zone=` + cacheZoneName + `:100m max_size=1g inactive=60m use_temp_path=off;
`
}

func cacheLogFormatConf() string { return config.NginxCacheLogConf() }

func cacheLogFormatBody() string {
	return `# Managed automatically by Servika. DO NOT EDIT.
# Provides a minimal log format that records only the upstream cache status for FastCGI cache hit-rate metrics.
log_format ` + cacheLogFormatName + ` '$upstream_cache_status';
`
}

// PublicHTML returns the default tenant document root.
func PublicHTML(systemUser string) string {
	return filepath.Join("/home", systemUser, "public_html")
}

// SafeWebRootSubdirectory validates a document-root path relative to public_html.
func SafeWebRootSubdirectory(subdirectory string) (string, error) {
	rel := strings.Trim(strings.TrimSpace(subdirectory), "/")
	if rel == "" || rel == "." {
		return "", nil
	}
	if strings.Contains(rel, "..") || !regexp.MustCompile(`^[A-Za-z0-9._/-]+$`).MatchString(rel) {
		return "", fmt.Errorf("invalid web root")
	}
	return rel, nil
}

// AbsoluteWebRoot resolves a public_html-relative document root and rejects symlink escapes.
func AbsoluteWebRoot(systemUser, subdirectory string) (string, error) {
	if !tenantUserPattern.MatchString(systemUser) {
		return "", fmt.Errorf("invalid system user")
	}
	rel, err := SafeWebRootSubdirectory(subdirectory)
	if err != nil {
		return "", err
	}
	base := PublicHTML(systemUser)
	abs := base
	if rel != "" {
		abs = filepath.Clean(filepath.Join(base, rel))
	}
	if abs != base && !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("web root cannot leave public_html")
	}
	check := abs
	for check == base || strings.HasPrefix(check, base+string(os.PathSeparator)) {
		if real, err := filepath.EvalSymlinks(check); err == nil {
			if real != base && !strings.HasPrefix(real, base+string(os.PathSeparator)) {
				return "", fmt.Errorf("web root cannot leave public_html through a symlink")
			}
			break
		}
		if check == base {
			break
		}
		check = filepath.Dir(check)
	}
	return abs, nil
}

// WebRootSubdirectory returns the public_html-relative subdirectory for a stored web root.
func WebRootSubdirectory(systemUser, webRoot string) string {
	base := PublicHTML(systemUser)
	clean := filepath.Clean(strings.TrimSpace(webRoot))
	if clean == "." || clean == "" || clean == base {
		return ""
	}
	if rel, ok := strings.CutPrefix(clean, base+string(os.PathSeparator)); ok {
		return rel
	}
	return ""
}

// SafeWebRoot returns a safe absolute document root, falling back to public_html.
func SafeWebRoot(systemUser, webRoot string) string {
	sub := WebRootSubdirectory(systemUser, webRoot)
	abs, err := AbsoluteWebRoot(systemUser, sub)
	if err != nil {
		return PublicHTML(systemUser)
	}
	return abs
}

func AddonWebRoot(systemUser, domainName string) string {
	return filepath.Join("/home", systemUser, "domains", strings.ToLower(strings.TrimSpace(domainName)))
}

func addonVhostConfigPath(systemUser, domainName string) string {
	safeDomain := strings.ToLower(strings.TrimSpace(domainName))
	safeDomain = slugSan.ReplaceAllString(safeDomain, "_")
	safeDomain = strings.Trim(safeDomain, "_")
	return "/etc/nginx/conf.d/addon_" + systemUser + "_" + safeDomain + ".conf"
}

func safeAddonWebRoot(systemUser, domainName, webRoot string) string {
	base := filepath.Join("/home", systemUser, "domains")
	clean := filepath.Clean(strings.TrimSpace(webRoot))
	if clean != "" && clean != "." && strings.HasPrefix(clean, base+string(os.PathSeparator)) {
		return clean
	}
	return AddonWebRoot(systemUser, domainName)
}

func currentWebRoot(systemUser string) string {
	if packageDB == nil {
		return PublicHTML(systemUser)
	}
	var webRoot string
	if err := packageDB.QueryRow(`SELECT COALESCE(web_root,'') FROM domains WHERE system_user=? AND parent_domain_id IS NULL LIMIT 1`, systemUser).Scan(&webRoot); err != nil {
		return PublicHTML(systemUser)
	}
	return webRoot
}

var cacheZoneDefinitionPattern = regexp.MustCompile(`keys_zone\s*=\s*` + regexp.QuoteMeta(cacheZoneName) + `\s*:`)

// Init configures database-backed state and repairs managed server configuration.
func Init(db *sql.DB) {
	packageDB = db
	// Chicken-egg fix: guarantee per-user ACL (setfacl) and the RAR extractor (bsdtar) are
	// installed BEFORE HealHomePerms and the file manager RAR extraction rely on them. This
	// keeps per-user ACL isolation and RAR extraction ready on the very first update + restart.
	ensureArchiveTools()
	healCacheZoneOnStartup()
	healPanelVhostHeadersOnStartup()
	healPanelLoginRateLimitOnStartup()
	healPanelIndexNoCacheOnStartup()
	ensurePMAStartup()
	healVhostsOnStartup()
	HealHomePerms()
	ensureFPMSELinuxFcontext()
	ensureHTTPDHomeBooleans()
	HealSSLCertPathsOnStartup()
	HealSSLVhost443OnStartup()
	EnsureTenantFPMOnStartup()
	HealWAFOnStartup() // WAF: validate ModSecurity module status + refresh per-domain modsec confs for WAF-enabled domains
}

func healCacheZoneOnStartup() {
	changed, err := ensureCacheZone()
	if err != nil {
		log.Printf("servikacache repair: could not write zone configuration: %v", err)
		return
	}
	if !changed {
		return
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		log.Printf("servikacache repair: nginx configuration remains invalid, reload skipped: %s", strings.TrimSpace(string(out)))
		return
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		log.Printf("servikacache repair: nginx reload failed: %s", strings.TrimSpace(string(out)))
		return
	}
	log.Printf("servikacache repair: zone configuration restored and nginx reloaded")
}

func ensureCacheZone() (bool, error) {
	changed := false
	zoneDir := cacheZoneDir()
	zoneConf := cacheZoneConf()
	tempConf := cacheZoneTempConf()
	zoneBody := cacheZoneBody()
	if err := os.MkdirAll(zoneDir, 0700); err != nil {
		return false, fmt.Errorf("create cache directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(zoneDir), 0o755); err != nil {
		return false, fmt.Errorf("set cache parent permissions: %w", err)
	}
	if uid, gid, err := uidGid("nginx"); err == nil {
		if err := os.Chown(zoneDir, uid, gid); err != nil {
			return false, fmt.Errorf("set cache directory ownership: %w", err)
		}
	}
	_, _ = exec.Command("restorecon", "-R", zoneDir).CombinedOutput()

	if _, err := os.Stat(tempConf); err == nil {
		if err := os.Remove(tempConf); err != nil {
			return false, fmt.Errorf("remove temporary cache zone configuration: %w", err)
		}
		changed = true
	}

	if cacheZoneDefinedElsewhere() {
		if _, err := os.Stat(zoneConf); err == nil {
			if err := os.Remove(zoneConf); err != nil {
				return false, fmt.Errorf("remove duplicate managed cache zone configuration: %w", err)
			}
			changed = true
		}
		return changed, nil
	}

	if current, err := os.ReadFile(zoneConf); err == nil && string(current) == zoneBody {
		// Zone body unchanged; still ensure the log format file exists.
		return ensureCacheLogFormat(), nil
	}
	if err := os.WriteFile(zoneConf, []byte(zoneBody), 0644); err != nil {
		return false, fmt.Errorf("write cache zone configuration: %w", err)
	}
	_, _ = exec.Command("restorecon", zoneConf).CombinedOutput()
	return ensureCacheLogFormat() || true, nil
}

// ensureCacheLogFormat writes the log_format definition file for cache status
// so that nginx vhosts can reference the servika_cache_status format. The file
// is prefixed 00- to load before any domain vhost in alphabetical glob order.
func ensureCacheLogFormat() bool {
	logFormatConf := cacheLogFormatConf()
	logFormatBody := cacheLogFormatBody()
	if current, err := os.ReadFile(logFormatConf); err == nil && string(current) == logFormatBody {
		return false
	}
	if err := os.WriteFile(logFormatConf, []byte(logFormatBody), 0644); err != nil {
		log.Printf("servikacache repair: could not write cache log format: %v", err)
		return false
	}
	_, _ = exec.Command("restorecon", logFormatConf).CombinedOutput()
	return true
}

// purgeFastCGICache removes all nginx FastCGI cache entries so that
// cache TTL and enable/disable changes take effect immediately on
// the next request instead of serving stale cached content.
func purgeFastCGICache(systemUser string) {
	dir := cacheZoneDir()
	if _, err := os.Stat(dir); err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var purged int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Cache hierarchy: <dir>/<one-char>/<two-char>/<cache-file>
		oneCharDir := filepath.Join(dir, entry.Name())
		oneLevel, _ := os.ReadDir(oneCharDir)
		for _, one := range oneLevel {
			if !one.IsDir() {
				continue
			}
			twoCharDir := filepath.Join(oneCharDir, one.Name())
			twoLevel, _ := os.ReadDir(twoCharDir)
			for _, two := range twoLevel {
				if err := os.Remove(filepath.Join(twoCharDir, two.Name())); err == nil {
					purged++
				}
			}
		}
	}
	if purged > 0 {
		log.Printf("fastcgi cache: purged %d entries (%s)", purged, systemUser)
	}
}

func cacheZoneDefinedElsewhere() bool {
	files := []string{"/etc/nginx/nginx.conf"}
	if extra, err := filepath.Glob("/etc/nginx/conf.d/*.conf"); err == nil {
		files = append(files, extra...)
	}
	for _, filename := range files {
		if filename == cacheZoneConf() {
			continue
		}
		body, err := os.ReadFile(filename)
		if err == nil && cacheZoneDefinitionPattern.Match(body) {
			return true
		}
	}
	return false
}

type phpConfig struct {
	PoolDir string
	SockDir string
	Service string
	FPMBin  string
}

var phpMap = map[string]phpConfig{
	"7.4": {PoolDir: "/etc/opt/remi/php74/php-fpm.d", SockDir: "/var/opt/remi/php74/run/php-fpm", Service: "php74-php-fpm", FPMBin: "/opt/remi/php74/root/usr/sbin/php-fpm"},
	"8.0": {PoolDir: "/etc/opt/remi/php80/php-fpm.d", SockDir: "/var/opt/remi/php80/run/php-fpm", Service: "php80-php-fpm", FPMBin: "/opt/remi/php80/root/usr/sbin/php-fpm"},
	"8.1": {PoolDir: "/etc/opt/remi/php81/php-fpm.d", SockDir: "/var/opt/remi/php81/run/php-fpm", Service: "php81-php-fpm", FPMBin: "/opt/remi/php81/root/usr/sbin/php-fpm"},
	"8.2": {PoolDir: "/etc/opt/remi/php82/php-fpm.d", SockDir: "/var/opt/remi/php82/run/php-fpm", Service: "php82-php-fpm", FPMBin: "/opt/remi/php82/root/usr/sbin/php-fpm"},
	"8.3": {PoolDir: "/etc/php-fpm.d", SockDir: "/run/php-fpm", Service: "php-fpm", FPMBin: "/usr/sbin/php-fpm"},
	"8.4": {PoolDir: "/etc/opt/remi/php84/php-fpm.d", SockDir: "/var/opt/remi/php84/run/php-fpm", Service: "php84-php-fpm", FPMBin: "/opt/remi/php84/root/usr/sbin/php-fpm"},
	"8.5": {PoolDir: "/etc/opt/remi/php85/php-fpm.d", SockDir: "/var/opt/remi/php85/run/php-fpm", Service: "php85-php-fpm", FPMBin: "/opt/remi/php85/root/usr/sbin/php-fpm"},
	"8.6": {PoolDir: "/etc/opt/remi/php86/php-fpm.d", SockDir: "/var/opt/remi/php86/run/php-fpm", Service: "php86-php-fpm", FPMBin: "/opt/remi/php86/root/usr/sbin/php-fpm"},
}

func ValidateDomain(d string) error {
	d = strings.ToLower(strings.TrimSpace(d))
	if d == "" {
		return fmt.Errorf("domain name is required")
	}
	if len(d) > 253 {
		return fmt.Errorf("domain name is too long")
	}
	if !domainNamePattern.MatchString(d) {
		return fmt.Errorf("invalid domain name format (example: example.com)")
	}
	return nil
}

func certSystemDir(domainName string) string {
	return filepath.Join(certSystemBaseDir(), domainName)
}

func prepareCertificateDir(domainName string) (string, error) {
	if err := ValidateDomain(domainName); err != nil {
		return "", err
	}
	sslDir := certSystemDir(strings.ToLower(strings.TrimSpace(domainName)))
	if err := os.MkdirAll(sslDir, 0755); err != nil {
		return "", fmt.Errorf("create certificate directory: %w", err)
	}
	if err := os.Chown(sslDir, 0, 0); err != nil {
		return "", fmt.Errorf("set certificate directory ownership: %w", err)
	}
	if err := os.Chmod(sslDir, 0755); err != nil {
		return "", fmt.Errorf("set certificate directory permissions: %w", err)
	}
	_, _ = tenantCommand("restorecon", "-R", sslDir).CombinedOutput()
	return sslDir, nil
}

func applyCertificatePermissions(sslDir, certPath, keyPath string) error {
	for _, item := range []struct {
		path string
		mode os.FileMode
	}{
		{path: certPath, mode: 0644},
		{path: keyPath, mode: 0600},
	} {
		if err := os.Chown(item.path, 0, 0); err != nil {
			return fmt.Errorf("set certificate ownership: %w", err)
		}
		if err := os.Chmod(item.path, item.mode); err != nil {
			return fmt.Errorf("set certificate permissions: %w", err)
		}
	}
	_, _ = tenantCommand("restorecon", "-R", sslDir).CombinedOutput()
	return nil
}

func readTenantCertificate(path string, expectedUID int) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open tenant certificate: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("open tenant certificate: invalid file descriptor")
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect tenant certificate: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("tenant certificate is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != expectedUID {
		return nil, fmt.Errorf("tenant certificate owner does not match the tenant")
	}
	if info.Size() <= 0 || info.Size() > maxCertificateFileSize {
		return nil, fmt.Errorf("tenant certificate size is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCertificateFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read tenant certificate: %w", err)
	}
	if len(data) > maxCertificateFileSize {
		return nil, fmt.Errorf("tenant certificate exceeds the size limit")
	}
	return data, nil
}

func writeSystemCertificate(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".servika-certificate-*")
	if err != nil {
		return fmt.Errorf("create temporary certificate: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary certificate permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary certificate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary certificate: %w", err)
	}
	if err := temporary.Chown(0, 0); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary certificate ownership: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary certificate: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install certificate: %w", err)
	}
	return nil
}

func copyTenantCertificate(source, destination string, expectedUID int, mode os.FileMode) error {
	data, err := readTenantCertificate(source, expectedUID)
	if err != nil {
		return err
	}
	return writeSystemCertificate(destination, data, mode)
}

func removeHomeCertificate(systemUser, domainName string) {
	if !tenantUserPattern.MatchString(systemUser) || ValidateDomain(domainName) != nil {
		return
	}
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	sslDir := filepath.Join("/home", systemUser, "ssl")
	_ = os.Remove(filepath.Join(sslDir, domainName+".crt"))
	_ = os.Remove(filepath.Join(sslDir, domainName+".key"))
}

// HealSSLCertPathsOnStartup migrates active certificates from tenant homes into root-owned system storage.
func HealSSLCertPathsOnStartup() {
	if packageDB == nil {
		return
	}
	rows, err := packageDB.Query(`SELECT id, domain_name, system_user, COALESCE(php_version,'8.3'), cert_path, key_path
		FROM domains
		WHERE ssl_enabled=1 AND (cert_path LIKE '/home/%' OR key_path LIKE '/home/%')`)
	if err != nil {
		log.Printf("SSL certificate path healing: query failed: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()

	migrated := 0
	for rows.Next() {
		var id int64
		var domainName, systemUser, phpVersion, oldCertPath, oldKeyPath string
		if err := rows.Scan(&id, &domainName, &systemUser, &phpVersion, &oldCertPath, &oldKeyPath); err != nil {
			log.Printf("SSL certificate path healing: row scan failed: %v", err)
			continue
		}
		if ValidateDomain(domainName) != nil || !tenantUserPattern.MatchString(systemUser) {
			log.Printf("SSL certificate path healing: refused invalid domain or tenant for domain ID %d", id)
			continue
		}
		domainName = strings.ToLower(strings.TrimSpace(domainName))
		expectedCertPath := filepath.Join("/home", systemUser, "ssl", domainName+".crt")
		expectedKeyPath := filepath.Join("/home", systemUser, "ssl", domainName+".key")
		if filepath.Clean(oldCertPath) != expectedCertPath || filepath.Clean(oldKeyPath) != expectedKeyPath {
			log.Printf("SSL certificate path healing: refused unexpected tenant paths for %s", domainName)
			continue
		}
		uid, _, err := uidGid(systemUser)
		if err != nil {
			log.Printf("SSL certificate path healing: resolve owner for %s: %v", domainName, err)
			continue
		}
		sslDir, err := prepareCertificateDir(domainName)
		if err != nil {
			log.Printf("SSL certificate path healing: prepare directory for %s: %v", domainName, err)
			continue
		}
		newCertPath := filepath.Join(sslDir, domainName+".crt")
		newKeyPath := filepath.Join(sslDir, domainName+".key")
		if err := copyTenantCertificate(oldCertPath, newCertPath, uid, 0644); err != nil {
			log.Printf("SSL certificate path healing: migrate certificate for %s: %v", domainName, err)
			continue
		}
		if err := copyTenantCertificate(oldKeyPath, newKeyPath, uid, 0600); err != nil {
			log.Printf("SSL certificate path healing: migrate private key for %s: %v", domainName, err)
			continue
		}
		_, _ = tenantCommand("restorecon", "-R", sslDir).CombinedOutput()

		socket, err := PHPSocketFor(systemUser, phpVersion)
		if err != nil {
			log.Printf("SSL certificate path healing: resolve PHP socket for %s: %v", domainName, err)
			continue
		}
		if err := applyVhostForDomain(packageDB, id, socket, phpVersion, &newCertPath, &newKeyPath); err != nil {
			log.Printf("SSL certificate path healing: render vhost for %s: %v", domainName, err)
			continue
		}
		if _, err := packageDB.Exec(`UPDATE domains SET cert_path=?, key_path=? WHERE id=?`, newCertPath, newKeyPath, id); err != nil {
			log.Printf("SSL certificate path healing: update database for %s: %v", domainName, err)
			continue
		}
		removeHomeCertificate(systemUser, domainName)
		migrated++
	}
	if err := rows.Err(); err != nil {
		log.Printf("SSL certificate path healing: row iteration failed: %v", err)
	}
	if migrated > 0 {
		log.Printf("SSL certificate path healing: migrated %d certificate sets", migrated)
	}
}

func SlugFromDomain(d string) string {
	s := strings.ToLower(strings.TrimSpace(d))
	s = slugSan.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 26 {
		s = s[:26]
	}
	return "c_" + s
}

func normalizePHP(v string) string {
	v = strings.TrimSpace(v)
	if _, ok := phpMap[v]; !ok {
		return "8.3"
	}
	return v
}

// vhostTmpl covers vhosts both with and without SSL.
var vhostTmpl = template.Must(template.New("v").Parse(`{{- if .SSL -}}
# {{.DomainName}} — port 80 remains open for the HTTP-01 challenge; all other traffic redirects to 443
server {
    listen 80;
    listen [::]:80;
    server_name {{.ServerNames}};

    location /.well-known/acme-challenge/ {
        root /var/www/_acme;
        auth_basic off;
        try_files $uri =404;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name {{.ServerNames}};

    ssl_certificate     {{.CertPath}};
    ssl_certificate_key {{.KeyPath}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;

    root {{.WebRoot}};
    index index.php index.html index.htm;
    disable_symlinks if_not_owner;

    # ---- Security headers (managed by the panel) ----
{{.SecHeaders}}
{{.ModSec}}{{.IPRules}}{{.DenyBlocks}}{{.HotlinkLocation}}

    access_log /var/log/nginx/{{.DomainName}}.access.log;
    error_log  /var/log/nginx/{{.DomainName}}.error.log warn;

{{if eq .Backend "apache"}}    # ---- Backend: Apache (127.0.0.1:10080 proxy) ----
    location / {
        proxy_pass http://127.0.0.1:10080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Host $host;
        proxy_read_timeout 60s;
    }
{{else if eq .Backend "static"}}    # ---- Backend: Static files (no PHP), PHP source-exposure guard ----
    location ~* \.(php|phtml|php3|php4|php5|phps)(/|$) { return 404; }
    location / { try_files $uri $uri/ =404; }
{{else}}    # ---- Backend: nginx + PHP-FPM (default) ----
    location / { try_files $uri $uri/ /index.php?$query_string; }

{{if .FastCgiCache}}    set $skip_cache 0;
    if ($request_method = POST) { set $skip_cache 1; }
    if ($query_string != "") { set $skip_cache 1; }
    if ($request_uri ~* "/wp-admin/|/wp-login.php|/cart/|/checkout/|/my-account/|preview=true|sitemap.*\.xml") { set $skip_cache 1; }
    if ($http_cookie ~* "comment_author|wordpress_[a-f0-9]+|wp-postpass|wordpress_no_cache|wordpress_logged_in") { set $skip_cache 1; }
{{end}}    location ~ \.php$ {
        try_files $uri =404;
        fastcgi_split_path_info ^(.+\.php)(/.+)$;
        fastcgi_pass unix:{{.PHPSocket}};
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param PATH_INFO $fastcgi_path_info;
        fastcgi_param HTTPS on;
        fastcgi_read_timeout 60s;
        # Repeat headers because this location may define add_header below.
{{.SecHeaders}}{{if .FastCgiCache}}        fastcgi_cache servikacache;
        fastcgi_cache_valid 200 301 302 {{.FastCgiCacheMinutes}}m;
        fastcgi_cache_valid 404 1m;
        fastcgi_cache_bypass $skip_cache;
        fastcgi_no_cache $skip_cache;
        fastcgi_cache_use_stale error timeout invalid_header updating http_500 http_503;
        fastcgi_cache_background_update on;
        fastcgi_cache_lock on;
        add_header X-Cache-Status $upstream_cache_status always;
        access_log /var/log/nginx/{{.DomainName}}.cache.log servika_cache_status buffer=32k flush=5m;
{{end}}    }
{{end}}
{{if .BrowserCache}}    # ---- Browser cache (static files and legitimate archive downloads) ----
    # ZIP and GZIP downloads are allowed; sensitive .sql.gz files are denied by the earlier location.
    location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2?|svg|webp|avif|mp4|webm|pdf|zip|gz)$ {
        expires {{.BrowserCacheDays}}d;
        access_log off;
        add_header Cache-Control "public" always;
        # Repeat headers because this location defines add_header.
{{.SecHeaders}}    }
{{end}}

    location ~ /\.(?!well-known) { deny all; }

{{if .ExtraDirectives}}    # ---- Additional directives (user-provided) ----
    {{.ExtraDirectives}}
{{end}}    # Servika managed (SSL: {{.SSLSource}}) — {{.DomainName}}
}
{{- else -}}
server {
    listen 80;
    listen [::]:80;
    server_name {{.ServerNames}};

    root {{.WebRoot}};
    index index.php index.html index.htm;
    disable_symlinks if_not_owner;

    access_log /var/log/nginx/{{.DomainName}}.access.log;
    error_log  /var/log/nginx/{{.DomainName}}.error.log warn;

    # ---- Security headers (managed by the panel) ----
{{.SecHeaders}}
    location /.well-known/acme-challenge/ {
        root /var/www/_acme;
        auth_basic off;
        try_files $uri =404;
    }

{{.IPRules}}{{.DenyBlocks}}{{.HotlinkLocation}}

{{if eq .Backend "apache"}}    # ---- Backend: Apache (127.0.0.1:10080 proxy) ----
    location / {
        proxy_pass http://127.0.0.1:10080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto http;
        proxy_set_header X-Forwarded-Host $host;
        proxy_read_timeout 60s;
    }
{{else if eq .Backend "static"}}    # ---- Backend: Static files (no PHP), PHP source-exposure guard ----
    location ~* \.(php|phtml|php3|php4|php5|phps)(/|$) { return 404; }
    location / { try_files $uri $uri/ =404; }
{{else}}    # ---- Backend: nginx + PHP-FPM (default) ----
    location / { try_files $uri $uri/ /index.php?$query_string; }

{{if .FastCgiCache}}    set $skip_cache 0;
    if ($request_method = POST) { set $skip_cache 1; }
    if ($query_string != "") { set $skip_cache 1; }
    if ($request_uri ~* "/wp-admin/|/wp-login.php|/cart/|/checkout/|/my-account/|preview=true|sitemap.*\.xml") { set $skip_cache 1; }
    if ($http_cookie ~* "comment_author|wordpress_[a-f0-9]+|wp-postpass|wordpress_no_cache|wordpress_logged_in") { set $skip_cache 1; }
{{end}}    location ~ \.php$ {
        try_files $uri =404;
        fastcgi_split_path_info ^(.+\.php)(/.+)$;
        fastcgi_pass unix:{{.PHPSocket}};
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param PATH_INFO $fastcgi_path_info;
        fastcgi_read_timeout 60s;
        # Repeat headers because this location may define add_header below.
{{.SecHeaders}}{{if .FastCgiCache}}        fastcgi_cache servikacache;
        fastcgi_cache_valid 200 301 302 {{.FastCgiCacheMinutes}}m;
        fastcgi_cache_valid 404 1m;
        fastcgi_cache_bypass $skip_cache;
        fastcgi_no_cache $skip_cache;
        fastcgi_cache_use_stale error timeout invalid_header updating http_500 http_503;
        fastcgi_cache_background_update on;
        fastcgi_cache_lock on;
        add_header X-Cache-Status $upstream_cache_status always;
        access_log /var/log/nginx/{{.DomainName}}.cache.log servika_cache_status buffer=32k flush=5m;
{{end}}    }
{{end}}
{{if .BrowserCache}}    # ---- Browser cache (static files and legitimate archive downloads) ----
    # ZIP and GZIP downloads are allowed; sensitive .sql.gz files are denied by the earlier location.
    location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2?|svg|webp|avif|mp4|webm|pdf|zip|gz)$ {
        expires {{.BrowserCacheDays}}d;
        access_log off;
        add_header Cache-Control "public" always;
        # Repeat headers because this location defines add_header.
{{.SecHeaders}}    }
{{end}}

    location ~ /\.(?!well-known) { deny all; }

{{if .ExtraDirectives}}    # ---- Additional directives (user-provided) ----
    {{.ExtraDirectives}}
{{end}}    # Servika managed — {{.DomainName}} (HTTP only, PHP {{.PHPVersion}})
}
{{- end -}}
`))

const denyBlocksNginx = `    # ---- Deny CGI and interpreter scripts ----
    location ~* \.(cgi|pl|py|sh|rb|lua|fcgi)$ { deny all; }
    # ---- Deny backup, dump, and sensitive files ----
    # Legitimate archives and compressed sitemaps remain downloadable.
    location ~* \.(sql|sql\.gz|bak|old|orig|save|swp|swo|dump|inc|log|php\.bak|php~|php\.save)$ { deny all; }
`

func buildSecurityHeaders(opts VhostOpts) string {
	var headers strings.Builder
	if opts.HdrXContentType {
		headers.WriteString("    add_header X-Content-Type-Options \"nosniff\" always;\n")
	}
	headers.WriteString("    add_header X-Frame-Options \"SAMEORIGIN\" always;\n")
	if opts.HdrXXSS {
		headers.WriteString("    add_header X-XSS-Protection \"1; mode=block\" always;\n")
	}
	if opts.HdrReferrer {
		headers.WriteString("    add_header Referrer-Policy \"strict-origin-when-cross-origin\" always;\n")
	}
	if opts.HdrPermissions {
		headers.WriteString("    add_header Permissions-Policy \"geolocation=(), microphone=(), camera=(), interest-cohort=()\" always;\n")
	}
	headers.WriteString("    add_header Content-Security-Policy-Report-Only \"default-src 'self' https: http: data: blob: 'unsafe-inline' 'unsafe-eval'; frame-ancestors 'self';\" always;\n")
	if opts.SSL() && opts.HdrCSPUpgrade {
		headers.WriteString("    add_header Content-Security-Policy \"upgrade-insecure-requests\" always;\n")
	}
	if opts.SSL() && opts.HdrHSTS {
		includeSubdomains := ""
		if opts.HSTSSubdomains {
			includeSubdomains = "; includeSubDomains"
		}
		preload := ""
		if opts.HSTSPreload {
			preload = "; preload"
		}
		fmt.Fprintf(&headers, "    add_header Strict-Transport-Security \"max-age=%d%s%s\" always;\n", opts.HSTSMaxAge, includeSubdomains, preload)
	}
	return headers.String()
}

var suspendedVhostTmpl = template.Must(template.New("suspended").Parse(`# {{.DomainName}} suspended by Servika
server {
    listen 80;
    listen [::]:80;
    server_name {{.ServerNames}};

    location /.well-known/acme-challenge/ {
        root /var/www/_acme;
        auth_basic off;
        try_files $uri =404;
    }

    access_log /var/log/nginx/{{.DomainName}}.access.log;
    error_log /var/log/nginx/{{.DomainName}}.error.log warn;

    location / { return 503; }
    error_page 503 /_suspended.html;
    location = /_suspended.html {
        internal;
        default_type text/html;
        return 503 '<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Account Suspended</title><style>body{font-family:system-ui,sans-serif;background:#f8fafc;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}.card{max-width:520px;background:#fff;border:1px solid #e2e8f0;border-radius:16px;padding:48px;text-align:center}h1{font-size:22px;color:#0f172a;margin:0 0 8px}p{color:#64748b;line-height:1.6}</style></head><body><div class="card"><h1>Account Suspended</h1><p>This website has been temporarily suspended. Please contact your service provider.</p></div></body></html>';
    }
}
{{if .SSL}}
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name {{.ServerNames}};

    ssl_certificate {{.CertPath}};
    ssl_certificate_key {{.KeyPath}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    access_log /var/log/nginx/{{.DomainName}}.access.log;
    error_log /var/log/nginx/{{.DomainName}}.error.log warn;

    location / { return 503; }
    error_page 503 /_suspended.html;
    location = /_suspended.html {
        internal;
        default_type text/html;
        return 503 '<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Account Suspended</title><style>body{font-family:system-ui,sans-serif;background:#f8fafc;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}.card{max-width:520px;background:#fff;border:1px solid #e2e8f0;border-radius:16px;padding:48px;text-align:center}h1{font-size:22px;color:#0f172a;margin:0 0 8px}p{color:#64748b;line-height:1.6}</style></head><body><div class="card"><h1>Account Suspended</h1><p>This website has been temporarily suspended. Please contact your service provider.</p></div></body></html>';
    }
}
{{end}}`))

var redirectVhostTmpl = template.Must(template.New("redirect").Parse(`# {{.DomainName}} redirect managed by Servika
server {
    listen 80;
    listen [::]:80;
    server_name {{.ServerNames}};

    location /.well-known/acme-challenge/ {
        root /var/www/_acme;
        auth_basic off;
        try_files $uri =404;
    }

    access_log /var/log/nginx/{{.DomainName}}.access.log;
    error_log /var/log/nginx/{{.DomainName}}.error.log warn;

    location / {
        return {{.RedirectCode}} {{.RedirectTarget}}$request_uri;
    }
}
{{if .SSL}}
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name {{.ServerNames}};

    ssl_certificate {{.CertPath}};
    ssl_certificate_key {{.KeyPath}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    access_log /var/log/nginx/{{.DomainName}}.access.log;
    error_log /var/log/nginx/{{.DomainName}}.error.log warn;

    location / {
        return {{.RedirectCode}} {{.RedirectTarget}}$request_uri;
    }
}
{{end}}`))

var phpPoolTmpl = template.Must(template.New("p").Parse(`[{{.User}}]
user = {{.User}}
group = {{.User}}
listen = {{.Socket}}
listen.owner = nginx
listen.group = nginx
listen.mode = 0660
pm = ondemand
pm.max_children = 8
pm.process_idle_timeout = 30s
pm.max_requests = 500
; Security settings use php_admin_value so tenant code cannot override them.
php_admin_value[open_basedir] = /home/{{.User}}/:/tmp/
php_admin_value[disable_functions] = exec,passthru,shell_exec,system,proc_open,popen,proc_close,proc_get_status,proc_terminate,proc_nice,pcntl_exec,dl,symlink,link,posix_kill,posix_mkfifo,posix_setpgid,posix_setsid,posix_setuid,posix_setgid
php_admin_value[upload_tmp_dir] = /home/{{.User}}/tmp
php_admin_value[sys_temp_dir] = /home/{{.User}}/tmp
php_admin_value[session.save_path] = /home/{{.User}}/tmp
catch_workers_output = yes
`))

// VhostOpts contains the optional SSL and server settings used to render a vhost.
type VhostOpts struct {
	DomainName string
	WebRoot    string
	PHPSocket  string
	PHPVersion string
	CertPath   string
	KeyPath    string
	SSLSource  string // "self-signed" | "letsencrypt" | ""
	Suspended  bool
	ConfigPath string

	// nginx security header toggles, enabled by default.
	HdrXContentType bool
	HdrXXSS         bool
	HdrReferrer     bool
	HdrPermissions  bool
	HdrCSPUpgrade   bool
	HdrHSTS         bool
	HSTSMaxAge      int
	HSTSSubdomains  bool
	HSTSPreload     bool

	// Performance caching.
	FastCgiCache        bool
	FastCgiCacheMinutes int
	BrowserCache        bool
	BrowserCacheDays    int

	// User-provided extra directives.
	ExtraDirectives string

	// Full raw custom vhost, enabled only for administrator-managed domains.
	CustomVhostContent string

	// Whole-domain redirect, enabled when no suspension or custom vhost is active.
	RedirectTarget string
	RedirectCode   int

	// Web server backend: "php-fpm" by default, "apache", or "static".
	Backend string

	// Render-time security blocks that are not persisted in the database.
	SecHeaders      string
	DenyBlocks      string
	ModSec          string // WAF (ModSecurity) server-context directive block; empty when WAF is off or module absent
	IPRules         string // IP allow/deny directives; empty when access control is off
	HotlinkLocation string // valid_referers image location; empty when hotlink protection is off
}

func (o VhostOpts) SSL() bool {
	return o.CertPath != "" && o.KeyPath != ""
}

// ServerNames returns the nginx server_name list for the domain.
func (o VhostOpts) ServerNames() string {
	return strings.Join(wwwHostNames(o.DomainName), " ")
}

// wwwHostNames returns the canonical certificate and vhost hostnames for a domain.
func wwwHostNames(domain string) []string {
	if strings.HasPrefix(strings.ToLower(domain), "www.") {
		return []string{domain}
	}
	return []string{domain, "www." + domain}
}

type Result struct {
	SystemUser string
	WebRoot    string
	FTPHost    string
	PHPVersion string
	PHPSocket  string
}

func phpPoolPath(systemUser, phpVersion string) (string, string, string) {
	version := normalizePHP(phpVersion)
	config := phpMap[version]
	return filepath.Join(config.PoolDir, systemUser+".conf"),
		filepath.Join(config.SockDir, systemUser+".sock"),
		config.Service
}

func writePoolValidated(systemUser, phpVersion string) (socket, service string, err error) {
	version := normalizePHP(phpVersion)
	config := phpMap[version]
	poolPath, socket, service := phpPoolPath(systemUser, version)

	if err := os.MkdirAll(filepath.Dir(poolPath), 0755); err != nil {
		return "", "", fmt.Errorf("create PHP pool directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0755); err != nil {
		return "", "", fmt.Errorf("create PHP socket directory: %w", err)
	}

	var pool bytes.Buffer
	if err := phpPoolTmpl.Execute(&pool, map[string]string{"User": systemUser, "Socket": socket}); err != nil {
		return "", "", fmt.Errorf("render PHP pool: %w", err)
	}

	previousPool, readErr := os.ReadFile(poolPath)
	hadPreviousPool := readErr == nil
	if err := os.WriteFile(poolPath, pool.Bytes(), 0644); err != nil {
		return "", "", fmt.Errorf("write PHP pool: %w", err)
	}
	if config.FPMBin != "" {
		if output, err := exec.Command(config.FPMBin, "-t").CombinedOutput(); err != nil {
			if hadPreviousPool {
				_ = os.WriteFile(poolPath, previousPool, 0644)
			} else {
				_ = os.Remove(poolPath)
			}
			return "", "", fmt.Errorf("php-fpm -t (%s) failed, pool restored: %s: %w", version, strings.TrimSpace(string(output)), err)
		}
	}
	if output, err := exec.Command("systemctl", "reload-or-restart", service).CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("php-fpm (%s) reload: %s: %w", service, strings.TrimSpace(string(output)), err)
	}
	return socket, service, nil
}

// renderAndReload writes the vhost, validates nginx, and reloads it for both SSL modes.
// For the "apache" backend, it also writes the per-domain Apache vhost and reloads httpd.
// When switching away from Apache, it removes the obsolete Apache vhost.
func renderAndReload(opts VhostOpts, systemUser string) error {
	// Use PHP-FPM as the default backend.
	if opts.Backend == "" {
		opts.Backend = "php-fpm"
	}
	// Preserve the isolated socket across every vhost rewrite, including SSL changes.
	if TenantFPMActive(systemUser) {
		opts.PHPSocket = tenantSocket(systemUser)
	}
	if !opts.Suspended && packageDB != nil {
		var suspended int
		_ = packageDB.QueryRow(
			`SELECT COALESCE(suspended,0) FROM domains WHERE system_user=? AND parent_domain_id IS NULL LIMIT 1`, systemUser).
			Scan(&suspended)
		opts.Suspended = suspended == 1
	}

	opts.SecHeaders = buildSecurityHeaders(opts)
	opts.DenyBlocks = denyBlocksNginx
	// WAF (ModSecurity) directive: computed from effective settings on every render.
	// When suspended the suspended template (no ModSec field) is rendered — computation is skipped.
	// buildModSec returns "" when WAF is off or the module is absent (doesn't break the vhost);
	// when active it also refreshes the per-domain modsec conf — single source, self-healing.
	if !opts.Suspended {
		opts.ModSec = buildModSec(systemUser)
		opts.IPRules = buildIPRules(opts.DomainName)
		opts.HotlinkLocation = buildHotlink(opts.DomainName)
	}

	if !opts.Suspended && opts.CustomVhostContent == "" && packageDB != nil {
		var target string
		var code int
		if err := packageDB.QueryRow(
			`SELECT target_url, status_code FROM domain_redirects WHERE domain_id=(SELECT id FROM domains WHERE domain_name=? LIMIT 1)`, opts.DomainName).
			Scan(&target, &code); err == nil && strings.TrimSpace(target) != "" {
			opts.RedirectTarget = target
			opts.RedirectCode = code
		}
	}

	tmpl := vhostTmpl
	if opts.Suspended {
		tmpl = suspendedVhostTmpl
	} else if opts.RedirectTarget != "" {
		tmpl = redirectVhostTmpl
	}
	var buf bytes.Buffer
	if opts.CustomVhostContent != "" && !opts.Suspended {
		buf.WriteString(strings.TrimSpace(opts.CustomVhostContent))
		buf.WriteByte('\n')
	} else if err := tmpl.Execute(&buf, opts); err != nil {
		return fmt.Errorf("template render: %w", err)
	}
	cfgPath := opts.ConfigPath
	if cfgPath == "" {
		cfgPath = "/etc/nginx/conf.d/dom_" + systemUser + ".conf"
	}
	previousConfig, readErr := os.ReadFile(cfgPath)
	hadPreviousConfig := readErr == nil
	if err := os.WriteFile(cfgPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write vhost: %w", err)
	}
	if _, err := ensureCacheZone(); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		if hadPreviousConfig {
			_ = os.WriteFile(cfgPath, previousConfig, 0644)
		} else {
			_ = os.Remove(cfgPath)
		}
		return fmt.Errorf("nginx -t failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// Purge stale FastCGI cache entries for this domain so that cache TTL and
	// enable/disable changes take effect immediately instead of serving old content.
	purgeFastCGICache(systemUser)

	// Manage the Apache backend idempotently by writing or removing its vhost.
	if opts.Backend == "apache" && !opts.Suspended {
		if err := writeApacheVhost(opts, systemUser); err != nil {
			return err
		}
	} else {
		if err := deleteApacheVhostIfExists(systemUser); err != nil {
			return err
		}
	}
	return nil
}

func Provision(domainName, phpVersion string) (*Result, error) {
	if err := ValidateDomain(domainName); err != nil {
		return nil, err
	}
	phpVersion = normalizePHP(phpVersion)
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	systemUser := SlugFromDomain(domainName)
	home := "/home/" + systemUser

	if !userExists(systemUser) {
		out, err := exec.Command("useradd", "-m", "-d", home, "-s", "/usr/sbin/nologin", systemUser).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "already exists") {
			return nil, fmt.Errorf("useradd: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	dirs := []string{"public_html", "logs", "tmp", "ssl", ".cron"}
	for _, d := range dirs {
		_ = os.MkdirAll(filepath.Join(home, d), 0750)
	}

	uid, gid, err := uidGid(systemUser)
	if err == nil {
		_ = filepath.Walk(home, func(p string, _ os.FileInfo, _ error) error {
			_ = os.Chown(p, uid, gid)
			return nil
		})
	}

	_ = filepath.Walk(filepath.Join(home, "public_html"), func(p string, info os.FileInfo, _ error) error {
		if info == nil {
			return nil
		}
		if info.IsDir() {
			_ = os.Chmod(p, 0750)
		} else {
			_ = os.Chmod(p, 0644)
		}
		return nil
	})
	if err == nil {
		hardenHomePerms(home, systemUser, uid, gid)
	}

	indexPath := filepath.Join(home, "public_html", "index.html")
	_ = os.WriteFile(indexPath, []byte(welcomeHTML(domainName)), 0644)
	if err == nil {
		_ = os.Chown(indexPath, uid, gid)
	}

	_, _ = exec.Command("restorecon", "-R", home).CombinedOutput()

	// Write, validate, and activate the tenant PHP-FPM pool.
	socket, _, err := writePoolValidated(systemUser, phpVersion)
	if err != nil {
		return nil, err
	}

	// Create the initial vhost without SSL.
	if err := renderAndReload(VhostOpts{
		DomainName: domainName,
		WebRoot:    PublicHTML(systemUser),
		PHPSocket:  socket,
		PHPVersion: phpVersion,
	}, systemUser); err != nil {
		return nil, err
	}

	return &Result{
		SystemUser: systemUser,
		WebRoot:    PublicHTML(systemUser),
		FTPHost:    domainName, // The handler stores h.IPv4, the server IP, in the ftp_host database column.
		PHPVersion: phpVersion,
		PHPSocket:  socket,
	}, nil
}

func DeprovisionAddonDomain(domainName, systemUser, webRoot string, parked bool) error {
	if domainName != "" && ValidateDomain(domainName) == nil {
		_ = os.Remove(addonVhostConfigPath(systemUser, domainName))
		_ = os.RemoveAll(certSystemDir(strings.ToLower(strings.TrimSpace(domainName))))
	}
	if !parked {
		docroot := safeAddonWebRoot(systemUser, domainName, webRoot)
		base := filepath.Join("/home", systemUser, "domains")
		if strings.HasPrefix(docroot, base+string(os.PathSeparator)) && filepath.Clean(docroot) != filepath.Clean(base) {
			_ = os.RemoveAll(docroot)
		}
	}
	_, _ = exec.Command("systemctl", "reload", "nginx").CombinedOutput()
	purgeFastCGICache(systemUser)
	return nil
}

func Deprovision(domainName, systemUser string) error {
	cfgPath := "/etc/nginx/conf.d/dom_" + systemUser + ".conf"
	_ = os.Remove(cfgPath)
	subdomainVhosts, _ := filepath.Glob("/etc/nginx/conf.d/sub_" + systemUser + "_*.conf")
	for _, vhostPath := range subdomainVhosts {
		_ = os.Remove(vhostPath)
	}
	TeardownTenantFPM(systemUser)
	if domainName != "" && ValidateDomain(domainName) == nil {
		_ = os.RemoveAll(certSystemDir(strings.ToLower(strings.TrimSpace(domainName))))
	}
	// Clean up per-domain WAF modsec confs (prevent orphans).
	if reWafSK.MatchString(systemUser) {
		_ = os.Remove(filepath.Join(wafDomainsDir, systemUser+".conf"))
		_ = os.Remove(filepath.Join(wafDomainsDir, systemUser+".custom.conf"))
	}
	for _, config := range phpMap {
		p := filepath.Join(config.PoolDir, systemUser+".conf")
		if _, err := os.Stat(p); err == nil {
			_ = os.Remove(p)
			_, _ = exec.Command("systemctl", "reload-or-restart", config.Service).CombinedOutput()
		}
	}
	_, _ = exec.Command("systemctl", "reload", "nginx").CombinedOutput()
	purgeFastCGICache(systemUser)

	if !strings.HasPrefix(systemUser, "c_") {
		return fmt.Errorf("security: refusing to delete a user without the c_ prefix")
	}
	if userExists(systemUser) {
		_, _ = exec.Command("userdel", "-r", systemUser).CombinedOutput()
	}
	return nil
}

func SetPHPVersion(domainName, systemUser, newVersion, certPath, keyPath, sslSource, backend, webRoot string) (string, error) {
	newVersion = normalizePHP(newVersion)
	for _, config := range phpMap {
		p := filepath.Join(config.PoolDir, systemUser+".conf")
		if _, err := os.Stat(p); err == nil {
			_ = os.Remove(p)
			_, _ = exec.Command("systemctl", "reload-or-restart", config.Service).CombinedOutput()
		}
	}

	socket, _, err := writePoolValidated(systemUser, newVersion)
	if err != nil {
		return "", err
	}

	if err := renderAndReload(VhostOpts{
		DomainName: domainName,
		WebRoot:    SafeWebRoot(systemUser, webRoot),
		PHPSocket:  socket,
		PHPVersion: newVersion,
		CertPath:   certPath,
		KeyPath:    keyPath,
		SSLSource:  sslSource,
		Backend:    backend,
	}, systemUser); err != nil {
		return "", err
	}
	return socket, nil
}

// EnableSelfSigned generates a self-signed certificate with OpenSSL and re-renders the vhost with SSL.
func EnableSelfSigned(domainName, systemUser, phpVersion, backend string) (certPath, keyPath string, err error) {
	if err := ValidateDomain(domainName); err != nil {
		return "", "", err
	}
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	phpVersion = normalizePHP(phpVersion)
	certPath, keyPath, err = generateSelfSigned(domainName)
	if err != nil {
		return "", "", err
	}
	if err := writeSSLVhost(domainName, systemUser, phpVersion, backend, certPath, keyPath, "self-signed"); err != nil {
		return "", "", err
	}
	removeHomeCertificate(systemUser, domainName)
	return certPath, keyPath, nil
}

// EnableLetsEncrypt obtains a certificate with acme.sh and re-renders the vhost with SSL.
//
// Rate-limit resilience (teardown fix — see ssl_heal.go):
//  1. REUSE-BEFORE-ISSUE: when a valid certificate (notAfter > now+30d, covers
//     the required hostnames, matching key) exists in the acme store or /etc/pki,
//     deploy it and SKIP issuance.
//     This never triggers a re-issue with the same SAN set (LE 429 rate-limit).
//  2. FAIL-SAFE: when issuance fails (including 429), sslFailSafe keeps 443 alive with the
//     existing/self-signed certificate. The vhost is never dropped to HTTP-only.
func EnableLetsEncrypt(domainName, systemUser, phpVersion, backend string) (certPath, keyPath string, real bool, err error) {
	if err := ValidateDomain(domainName); err != nil {
		return "", "", false, err
	}
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	phpVersion = normalizePHP(phpVersion)

	sslDir, err := prepareCertificateDir(domainName)
	if err != nil {
		return "", "", false, err
	}
	certPath = filepath.Join(sslDir, domainName+".crt")
	keyPath = filepath.Join(sslDir, domainName+".key")

	// (1) Reuse-before-issue: skip a fresh issuance only when a valid real CA certificate exists.
	if src, srcKey := reusableLetsEncryptCertificate(domainName, 30); src != "" {
		if cp, kp, e := installToPKI(domainName, src, srcKey); e == nil {
			if e := writeSSLVhost(domainName, systemUser, phpVersion, backend, cp, kp, "letsencrypt"); e != nil {
				return "", "", false, e
			}
			removeHomeCertificate(systemUser, domainName)
			log.Printf("ssl reuse: %s valid letsencrypt certificate found; fresh LE issuance skipped (rate-limit protection)", domainName)
			return cp, kp, true, nil
		}
	}

	// (2) Real issuance/renewal (only reached when <30 days remain or no cert exists).
	_ = os.MkdirAll("/var/www/_acme", 0755)
	_, _ = tenantCommand("restorecon", "-R", "/var/www/_acme").CombinedOutput()

	// --force removed: acme.sh does not re-issue when it already has a valid cert
	// (rate-limit protection). It still renews inside the renewal window.
	args := []string{
		"--issue",
		"--webroot", "/var/www/_acme",
	}
	for _, host := range wwwHostNames(domainName) {
		args = append(args, "-d", host)
	}
	args = append(args, "--keylength", "2048")
	if out, e := acmeCommand(args...).CombinedOutput(); e != nil {
		// FAIL-SAFE (no teardown): keep 443 alive with the existing/self-signed cert.
		return sslFailSafe(domainName, systemUser, phpVersion, backend, "acme issue: "+strings.TrimSpace(string(out)))
	}

	// Install the certificate into the target paths with acme.sh install-cert.
	insArgs := []string{
		"--install-cert",
		"-d", domainName,
		"--cert-file", certPath,
		"--key-file", keyPath,
		"--fullchain-file", certPath,
		"--reloadcmd", "systemctl reload nginx",
	}
	if out, e := acmeCommand(insArgs...).CombinedOutput(); e != nil {
		return sslFailSafe(domainName, systemUser, phpVersion, backend, "acme install-cert: "+strings.TrimSpace(string(out)))
	}
	if err := applyCertificatePermissions(sslDir, certPath, keyPath); err != nil {
		return "", "", false, err
	}
	if e := writeSSLVhost(domainName, systemUser, phpVersion, backend, certPath, keyPath, "letsencrypt"); e != nil {
		return "", "", false, e
	}
	removeHomeCertificate(systemUser, domainName)
	return certPath, keyPath, true, nil
}

// DisableSSL re-renders the vhost without SSL while retaining certificate files for reuse.
func DisableSSL(domainName, systemUser, phpVersion, backend string) error {
	phpVersion = normalizePHP(phpVersion)
	_, socket, _ := phpPoolPath(systemUser, phpVersion)
	return renderAndReload(VhostOpts{
		DomainName: domainName,
		WebRoot:    SafeWebRoot(systemUser, currentWebRoot(systemUser)),
		PHPSocket:  socket,
		PHPVersion: phpVersion,
		Backend:    backend,
	}, systemUser)
}

func userExists(username string) bool {
	_, err := user.Lookup(username)
	return err == nil
}

func uidGid(username string) (int, int, error) {
	account, err := user.Lookup(username)
	if err != nil {
		return 0, 0, err
	}
	uid, _ := strconv.Atoi(account.Uid)
	gid, _ := strconv.Atoi(account.Gid)
	return uid, gid, nil
}

// ensureArchiveToolsOnce runs the archive-tool heal at most once per process (no repeated dnf).
var ensureArchiveToolsOnce sync.Once

// ensureArchiveTools guarantees that per-user ACL (setfacl, acl package) and the RAR extractor
// (bsdtar, libarchive) are installed on the host, at panel startup, BEFORE HealHomePerms and the
// file manager RAR extraction rely on them.
//
// Why this is needed (chicken-egg): servika-update updates itself first; the step that installs
// `dnf install acl bsdtar` exists only in the new update script, so it does not run on the first
// update. Without the tools, hardenHomePerms falls back to the fail-safe group=nginx model
// (per-user ACL only arrives on the second update) and .rar archives cannot be opened. This heal
// installs the tools from the panel's own startup, so per-user ACL isolation and RAR extraction
// are ready even on the first update + restart.
//
// Idempotent and once per process (sync.Once). When a tool is already on PATH, dnf is NOT called.
// When dnf is unavailable (different distribution or minimal environment), the heal is skipped
// silently so the existing fail-safe branches (group=nginx, RAR unar/unrar fallback) stay in
// effect. Each install is logged.
func ensureArchiveTools() {
	ensureArchiveToolsOnce.Do(func() {
		if _, err := exec.LookPath("dnf"); err != nil {
			return
		}
		if _, err := exec.LookPath("setfacl"); err != nil {
			if out, err := exec.Command("dnf", "install", "-y", "acl").CombinedOutput(); err != nil {
				log.Printf("archive-tool heal: 'acl' install failed (fail-safe group=nginx in effect): %s", strings.TrimSpace(string(out)))
			} else {
				log.Printf("archive-tool heal: 'acl' (setfacl) installed; per-user ACL isolation active on first update")
			}
		}
		if _, err := exec.LookPath("bsdtar"); err != nil {
			if out, err := exec.Command("dnf", "install", "-y", "bsdtar").CombinedOutput(); err != nil {
				log.Printf("archive-tool heal: 'bsdtar' install failed (RAR may fall back to unar/unrar): %s", strings.TrimSpace(string(out)))
			} else {
				log.Printf("archive-tool heal: 'bsdtar' (libarchive) installed; RAR extraction ready on first update")
			}
		}
	})
}

const homeACLSentinel = "/var/lib/servika/.home_acl_v1_done"

func aclAvailable() bool {
	_, err := exec.LookPath("setfacl")
	return err == nil
}

func applyLegacyHomePerms(home string, uid, nginxGID int) {
	publicHTML := filepath.Join(home, "public_html")
	_ = os.Chown(home, uid, nginxGID)
	_ = os.Chmod(home, 0710)
	_ = os.Chown(publicHTML, uid, nginxGID)
	_ = os.Chmod(publicHTML, 0750)
}

func hardenHomePerms(home, systemUser string, uid, gid int) bool {
	publicHTML := filepath.Join(home, "public_html")
	if !managedPublicHTML(publicHTML, systemUser) {
		log.Printf("tenant home permissions: rejected unmanaged path %s", publicHTML)
		return false
	}
	if aclAvailable() {
		_ = os.Chown(home, uid, gid)
		_ = os.Chmod(home, 0710)
		_ = os.Chown(publicHTML, uid, gid)
		_ = os.Chmod(publicHTML, 0750)
		if output, err := tenantCommand("setfacl", "-m", "u:nginx:--x", home).CombinedOutput(); err != nil {
			log.Printf("tenant home permissions: home ACL failed for %s: %s", systemUser, strings.TrimSpace(string(output)))
			return false
		}
		if output, err := tenantCommand("setfacl", "-m", "u:nginx:rX", publicHTML).CombinedOutput(); err != nil {
			log.Printf("tenant home permissions: document root ACL failed for %s: %s", systemUser, strings.TrimSpace(string(output)))
			return false
		}
		if output, err := tenantCommand("setfacl", "-d", "-m", "u:nginx:rX", publicHTML).CombinedOutput(); err != nil {
			log.Printf("tenant home permissions: default ACL failed for %s: %s", systemUser, strings.TrimSpace(string(output)))
			return false
		}
		return true
	}

	if _, nginxGID, err := uidGid("nginx"); err == nil {
		applyLegacyHomePerms(home, uid, nginxGID)
		return false
	}
	log.Printf("tenant home permissions: ACL tools and nginx account unavailable for %s", systemUser)
	_ = os.Chmod(home, 0711)
	_ = os.Chmod(publicHTML, 0755)
	return false
}

func managedPublicHTML(path, systemUser string) bool {
	if !tenantUserPattern.MatchString(systemUser) {
		return false
	}
	expected := filepath.Join("/home", systemUser, "public_html")
	if filepath.Clean(path) != expected {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func hardenHomePermsRecursive(publicHTML, systemUser string) bool {
	if !managedPublicHTML(publicHTML, systemUser) || !aclAvailable() {
		return false
	}
	output, err := tenantCommand("setfacl", "-R", "-P", "-m", "u:nginx:rX", publicHTML).CombinedOutput()
	if err != nil {
		log.Printf("tenant home permissions: recursive ACL failed for %s: %s", systemUser, strings.TrimSpace(string(output)))
		return false
	}
	return true
}

// HealHomePerms applies tenant-isolating ownership and permissions to existing managed homes.
func HealHomePerms() {
	if packageDB == nil {
		return
	}
	rows, err := packageDB.Query(`SELECT DISTINCT system_user FROM domains`)
	if err != nil {
		log.Printf("heal tenant home permissions: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()

	_, sentinelErr := os.Stat(homeACLSentinel)
	migrateExisting := os.IsNotExist(sentinelErr)
	updated := 0
	migrationSucceeded := aclAvailable()
	for rows.Next() {
		var systemUser string
		if err := rows.Scan(&systemUser); err != nil || !tenantUserPattern.MatchString(systemUser) {
			continue
		}
		home := filepath.Join("/home", systemUser)
		info, err := os.Lstat(home)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		uid, gid, err := uidGid(systemUser)
		if err != nil {
			continue
		}
		if !hardenHomePerms(home, systemUser, uid, gid) {
			migrationSucceeded = false
		}
		if migrateExisting && !hardenHomePermsRecursive(filepath.Join(home, "public_html"), systemUser) {
			migrationSucceeded = false
		}
		updated++
	}
	if err := rows.Err(); err != nil {
		log.Printf("heal tenant home permissions rows: %v", err)
		migrationSucceeded = false
	}
	if updated > 0 {
		log.Printf("healed permissions for %d tenant homes", updated)
	}
	if migrateExisting && migrationSucceeded {
		if err := os.MkdirAll(filepath.Dir(homeACLSentinel), 0755); err != nil {
			log.Printf("heal tenant home permissions: could not create sentinel directory: %v", err)
			return
		}
		if err := os.WriteFile(homeACLSentinel, []byte("done\n"), 0644); err != nil {
			log.Printf("heal tenant home permissions: could not write sentinel: %v", err)
		}
	}
}

func tenantCommand(name string, args ...string) *exec.Cmd {
	command := exec.Command(name, args...)
	command.Env = []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C",
		"LC_ALL=C",
	}
	return command
}

func acmeCommand(args ...string) *exec.Cmd {
	command := tenantCommand(config.ACMEBin(), args...)
	command.Env = append(command.Env, "HOME="+config.ACMEHome())
	return command
}

// SuspendUserRuntime disables or restores cron execution and terminates managed tenant processes.
func SuspendUserRuntime(systemUser string, suspended bool) {
	if !tenantUserPattern.MatchString(systemUser) {
		return
	}
	const suspendedCronDir = "/var/lib/servika/cron-suspended"
	cronSpool := filepath.Join("/var/spool/cron", systemUser)
	storedCron := filepath.Join(suspendedCronDir, systemUser)

	if suspended {
		if _, err := os.Stat(cronSpool); err == nil {
			if err := os.MkdirAll(suspendedCronDir, 0700); err != nil {
				log.Printf("suspend tenant runtime: create cron store for %s: %v", systemUser, err)
			} else if err := os.Rename(cronSpool, storedCron); err != nil {
				log.Printf("suspend tenant runtime: disable crontab for %s: %v", systemUser, err)
			}
		}
		_, _ = tenantCommand("pkill", "-KILL", "-u", systemUser).CombinedOutput()
		return
	}

	if _, err := os.Stat(storedCron); err == nil {
		if err := os.MkdirAll("/var/spool/cron", 0700); err != nil {
			log.Printf("resume tenant runtime: create cron spool for %s: %v", systemUser, err)
			return
		}
		if err := os.Rename(storedCron, cronSpool); err != nil {
			log.Printf("resume tenant runtime: restore crontab for %s: %v", systemUser, err)
			return
		}
		_ = os.Chmod(cronSpool, 0600)
		_, _ = tenantCommand("restorecon", cronSpool).CombinedOutput()
	}
}

func welcomeHTML(domain string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:Inter,system-ui,sans-serif;background:linear-gradient(135deg,#f8fafc,#fff7ed);min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px}
  .card{max-width:560px;background:#fff;border:1px solid #e2e8f0;border-radius:16px;padding:48px;text-align:center;box-shadow:0 10px 25px rgba(0,0,0,0.05)}
  .logo{width:48px;height:48px;background:#ea580c;border-radius:10px;margin:0 auto 20px;display:flex;align-items:center;justify-content:center;color:#fff;font-weight:700}
  h1{font-size:24px;color:#0f172a;margin-bottom:8px}
  p{color:#64748b;line-height:1.6;margin-bottom:8px}
  .muted{font-size:13px;color:#94a3b8;margin-top:24px}
  code{background:#f1f5f9;padding:2px 6px;border-radius:4px;font-size:13px;color:#475569}
</style>
</head>
<body>
<div class="card">
  <div class="logo">G</div>
  <h1>%s</h1>
  <p>The website was created successfully.</p>
  <p>Use FTP or the file manager to upload content.</p>
  <p class="muted">Web root: <code>public_html/</code> · PHP enabled · Managed by Servika</p>
</div>
</body>
</html>`, domain, domain)
}

// ApplyVhostForDomain re-renders an nginx vhost for a domain ID.
// It runs after PHP version or socket changes and loads SSL settings from the database.
func ApplyVhostForDomain(db *sql.DB, domainID int64, socket, phpVersion string) error {
	return applyVhostForDomain(db, domainID, socket, phpVersion, nil, nil)
}

func applyVhostForDomain(db *sql.DB, domainID int64, socket, phpVersion string, certPathOverride, keyPathOverride *string) error {
	var domainName, systemUser, certPath, keyPath, sslSource, backend, webRoot, customVhostContent string
	var suspended, customVhostEnabled int
	var parentDomainID sql.NullInt64
	if err := db.QueryRow(
		`SELECT domain_name, system_user, COALESCE(cert_path,''), COALESCE(key_path,''), COALESCE(ssl_source,''),
		        COALESCE(web_backend,'php-fpm'), COALESCE(web_root,''), COALESCE(suspended,0),
		        COALESCE(custom_vhost_enabled,0), COALESCE(custom_vhost_content,''), parent_domain_id
		 FROM domains WHERE id=?`, domainID).
		Scan(&domainName, &systemUser, &certPath, &keyPath, &sslSource, &backend, &webRoot, &suspended,
			&customVhostEnabled, &customVhostContent, &parentDomainID); err != nil {
		return fmt.Errorf("read domain details: %w", err)
	}
	if certPathOverride != nil && keyPathOverride != nil {
		certPath = *certPathOverride
		keyPath = *keyPathOverride
	}
	if TenantFPMActive(systemUser) {
		socket = tenantSocket(systemUser)
	}

	webRoot = SafeWebRoot(systemUser, webRoot)
	configPath := "/etc/nginx/conf.d/dom_" + systemUser + ".conf"
	if parentDomainID.Valid {
		configPath = addonVhostConfigPath(systemUser, domainName)
		webRoot = safeAddonWebRoot(systemUser, domainName, webRoot)
	}

	// Default nginx settings to enabled when no row exists.
	opts := VhostOpts{
		ConfigPath:      configPath,
		DomainName:      domainName,
		WebRoot:         webRoot,
		PHPSocket:       socket,
		PHPVersion:      phpVersion,
		CertPath:        certPath,
		KeyPath:         keyPath,
		SSLSource:       sslSource,
		Backend:         backend,
		Suspended:       suspended == 1,
		HdrXContentType: true, HdrXXSS: true, HdrReferrer: true,
		HdrPermissions: true, HdrCSPUpgrade: true, HdrHSTS: true,
		HSTSMaxAge: 31536000, HSTSSubdomains: true, HSTSPreload: false,
	}
	if customVhostEnabled == 1 {
		opts.CustomVhostContent = customVhostContent
	}
	// Disable FastCGI cache and enable a 30-day browser cache by default.
	opts.FastCgiCache = false
	opts.FastCgiCacheMinutes = 60
	opts.BrowserCache = true
	opts.BrowserCacheDays = 30

	var b1, b2, b3, b4, b5, b6, b7, b8, bFC, bBC int
	var maxAge, fastCgiCacheMinutes, browserCacheDays int
	var extraDirectives string
	err := db.QueryRow(
		`SELECT hdr_x_content_type, hdr_x_xss, hdr_referrer, hdr_permissions,
		        hdr_csp_upgrade, hdr_hsts, hsts_max_age, hsts_subdomains, hsts_preload, extra_directives,
		        fastcgi_cache, fastcgi_cache_minutes, browser_cache, browser_cache_days
		 FROM nginx_settings WHERE domain_id=?`, domainID).
		Scan(&b1, &b2, &b3, &b4, &b5, &b6, &maxAge, &b7, &b8, &extraDirectives,
			&bFC, &fastCgiCacheMinutes, &bBC, &browserCacheDays)
	if err == nil {
		opts.HdrXContentType = b1 == 1
		opts.HdrXXSS = b2 == 1
		opts.HdrReferrer = b3 == 1
		opts.HdrPermissions = b4 == 1
		opts.HdrCSPUpgrade = b5 == 1
		opts.HdrHSTS = b6 == 1
		opts.HSTSMaxAge = maxAge
		opts.HSTSSubdomains = b7 == 1
		opts.HSTSPreload = b8 == 1
		opts.ExtraDirectives = extraDirectives
		opts.FastCgiCache = bFC == 1
		opts.FastCgiCacheMinutes = fastCgiCacheMinutes
		opts.BrowserCache = bBC == 1
		opts.BrowserCacheDays = browserCacheDays
	}
	// Add protected-directory .htpasswd blocks regardless of whether nginx_settings has a row.
	if pb := buildProtectedBlocks(db, domainID, socket); pb != "" {
		if opts.ExtraDirectives != "" {
			opts.ExtraDirectives += "\n"
		}
		opts.ExtraDirectives += pb
	}
	return renderAndReload(opts, systemUser)
}

// RerenderVhost resolves a domain's PHP socket and re-renders its vhost.
func RerenderVhost(db *sql.DB, domainID int64) error {
	var systemUser, phpVersion string
	if err := db.QueryRow(
		`SELECT system_user, php_version FROM domains WHERE id=?`, domainID).
		Scan(&systemUser, &phpVersion); err != nil {
		return err
	}
	socket, err := PHPSocketFor(systemUser, phpVersion)
	if err != nil {
		socket = "/run/php-fpm/" + systemUser + ".sock"
	}
	return ApplyVhostForDomain(db, domainID, socket, phpVersion)
}

// PHPSocketFor returns the active tenant or shared socket path.
func PHPSocketFor(systemUser, phpVersion string) (string, error) {
	if TenantFPMActive(systemUser) {
		return tenantSocket(systemUser), nil
	}
	return sharedSocketPath(systemUser, phpVersion)
}

func sharedSocketPath(systemUser, phpVersion string) (string, error) {
	phpVersion = normalizePHP(phpVersion)
	// AppStream 8.3
	if phpVersion == "8.3" {
		return "/run/php-fpm/" + systemUser + ".sock", nil
	}
	// Remi pattern: 5.6 -> 56, 7.4 -> 74, 8.2 -> 82
	versionCode := strings.Replace(phpVersion, ".", "", 1)
	if len(versionCode) >= 2 {
		socketDir := "/var/opt/remi/php" + versionCode + "/run/php-fpm"
		// Verify that the service is installed.
		if _, err := os.Stat("/opt/remi/php" + versionCode + "/root/usr/sbin/php-fpm"); err == nil {
			return socketDir + "/" + systemUser + ".sock", nil
		}
	}
	return "", fmt.Errorf("unsupported or uninstalled version: %s", phpVersion)
}

// buildProtectedBlocks generates nginx auth_basic location blocks from protected_directories.
// Each protected path receives an outer prefix location and a nested .php location that prevents PHP source disclosure.
func buildProtectedBlocks(db *sql.DB, domainID int64, socket string) string {
	rows, err := db.Query(`SELECT DISTINCT path, htpasswd_file FROM protected_directories WHERE domain_id=? ORDER BY path`, domainID)
	if err != nil {
		return ""
	}
	defer func() { _ = rows.Close() }()
	var b strings.Builder
	for rows.Next() {
		var path, file string
		if err := rows.Scan(&path, &file); err != nil {
			continue
		}
		if path == "/" {
			// The root path cannot use a separate "location /" because it duplicates the required
			// existing prefix and nginx rejects it. Define auth_basic at the server level instead,
			// allowing all locations, including PHP, to inherit it. The acme-challenge location
			// remains exempt through "auth_basic off", so Let's Encrypt issuance and renewal work.
			fmt.Fprintf(&b, `    auth_basic "Authentication Required";
    auth_basic_user_file %s;
`, file)
			continue
		}
		fmt.Fprintf(&b, `    location ^~ %s {
        auth_basic "Authentication Required";
        auth_basic_user_file %s;
        location ~ \.php$ {
            auth_basic "Authentication Required";
            auth_basic_user_file %s;
            try_files $uri =404;
            fastcgi_split_path_info ^(.+\.php)(/.+)$;
            fastcgi_pass unix:%s;
            fastcgi_index index.php;
            include fastcgi_params;
            fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
            fastcgi_param HTTPS on;
        }
    }
`, path, file, file, socket)
	}
	_ = rows.Err()
	return b.String()
}

const (
	vhostHardenSentinel = "/var/lib/servika/.vhost_hardening_v2_done"
	panelVhostPath      = "/etc/nginx/conf.d/_panel.conf"
	panelSecSentinel    = "# SERVIKA-PANEL-SEC v2"
)

func healVhostsOnStartup() {
	if packageDB == nil {
		return
	}
	if _, err := os.Stat(vhostHardenSentinel); err == nil {
		return
	}

	rows, err := packageDB.Query(`SELECT id, system_user, php_version FROM domains`)
	if err != nil {
		log.Printf("vhost hardening: could not list domains: %v", err)
		return
	}
	type domain struct {
		id         int64
		systemUser string
		phpVersion string
	}
	var domains []domain
	rowReadFailed := false
	for rows.Next() {
		var item domain
		if err := rows.Scan(&item.id, &item.systemUser, &item.phpVersion); err != nil {
			log.Printf("vhost hardening: could not read domain row: %v", err)
			rowReadFailed = true
			continue
		}
		domains = append(domains, item)
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		log.Printf("vhost hardening: domain iteration failed: %v", rowsErr)
		return
	}
	if rowReadFailed {
		log.Printf("vhost hardening: at least one domain row could not be read, retry scheduled for next startup")
		return
	}

	failed := 0
	for _, item := range domains {
		domainFailed := false
		var socket string
		if TenantFPMActive(item.systemUser) {
			socket = tenantSocket(item.systemUser)
		} else {
			resolved, _, err := writePoolValidated(item.systemUser, item.phpVersion)
			if err != nil {
				log.Printf("vhost hardening: %s PHP pool update failed: %v", item.systemUser, err)
				domainFailed = true
				if fallback, resolveErr := sharedSocketPath(item.systemUser, item.phpVersion); resolveErr == nil {
					resolved = fallback
				}
			}
			socket = resolved
			if socket == "" {
				socket = "/run/php-fpm/" + item.systemUser + ".sock"
			}
		}
		if err := ApplyVhostForDomain(packageDB, item.id, socket, item.phpVersion); err != nil {
			log.Printf("vhost hardening: %s vhost update failed: %v", item.systemUser, err)
			domainFailed = true
		}
		if domainFailed {
			failed++
		}
	}
	if failed != 0 {
		log.Printf("vhost hardening: %d of %d domains failed, retry scheduled for next startup", failed, len(domains))
		return
	}

	if err := os.MkdirAll(filepath.Dir(vhostHardenSentinel), 0755); err != nil {
		log.Printf("vhost hardening: could not create sentinel directory: %v", err)
		return
	}
	if err := os.WriteFile(vhostHardenSentinel, []byte("done\n"), 0644); err != nil {
		log.Printf("vhost hardening: could not write sentinel: %v", err)
	}
}

func healPanelVhostHeadersOnStartup() {
	original, err := os.ReadFile(panelVhostPath)
	if err != nil {
		return
	}
	content := string(original)
	if strings.Contains(content, panelSecSentinel) {
		return
	}
	anchor := "server_name _;"
	anchorIndex := strings.Index(content, anchor)
	if anchorIndex < 0 {
		log.Printf("panel security repair: %q anchor not found", anchor)
		return
	}

	headers := "\n    " + panelSecSentinel + "\n" +
		"    add_header X-Content-Type-Options \"nosniff\" always;\n" +
		"    add_header X-Frame-Options \"SAMEORIGIN\" always;\n" +
		"    add_header Referrer-Policy \"strict-origin-when-cross-origin\" always;\n" +
		"    add_header Permissions-Policy \"geolocation=(), microphone=(), camera=(), interest-cohort=()\" always;\n" +
		"    add_header Content-Security-Policy \"default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; img-src 'self' data: blob:; font-src 'self' data: https://fonts.gstatic.com; connect-src 'self'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'\" always;\n" +
		"    add_header Strict-Transport-Security \"max-age=31536000; includeSubDomains\" always;\n"
	insertAt := anchorIndex + len(anchor)
	updated := content[:insertAt] + headers + content[insertAt:]

	cacheHeader := "        add_header Cache-Control \"public\";"
	repeatedHeaders := cacheHeader + "\n" +
		"        add_header X-Content-Type-Options \"nosniff\" always;\n" +
		"        add_header X-Frame-Options \"SAMEORIGIN\" always;\n" +
		"        add_header Referrer-Policy \"strict-origin-when-cross-origin\" always;\n" +
		"        add_header Strict-Transport-Security \"max-age=31536000; includeSubDomains\" always;"
	updated = strings.ReplaceAll(updated, cacheHeader, repeatedHeaders)

	if err := os.WriteFile(panelVhostPath, []byte(updated), 0644); err != nil {
		log.Printf("panel security repair: could not write vhost: %v", err)
		return
	}
	if output, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		_ = os.WriteFile(panelVhostPath, original, 0644)
		log.Printf("panel security repair: nginx -t failed, vhost restored: %s", strings.TrimSpace(string(output)))
		return
	}
	if output, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		log.Printf("panel security repair: nginx reload failed: %s", strings.TrimSpace(string(output)))
	}
}
