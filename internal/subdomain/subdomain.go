// Package subdomain manages subdomains under the parent domain's user and PHP pool.
// Each subdomain receives a separate document root, nginx server block, and DNS A record.
package subdomain

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/dns"
	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB   *sql.DB
	IPv4 string
}

var subdomainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type Sub struct {
	ID         int64  `json:"id"`
	Subdomain  string `json:"subdomain"`
	FQDN       string `json:"fqdn"`
	PHPVersion string `json:"php_version"`
	DocRoot    string `json:"docroot"`
	CreatedAt  string `json:"created_at"`
}

func (h *Handlers) parent(r *http.Request) (id int64, systemUser, domainName, phpVersion string, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, domain_name, COALESCE(php_version,'8.3'), COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&systemUser, &domainName, &phpVersion, &isDemo); err != nil {
		return id, "", "", "", false, false
	}
	return id, systemUser, domainName, phpVersion, isDemo == 1, true
}

func docrootOf(systemUser, fqdn string) string { return "/home/" + systemUser + "/subdomains/" + fqdn }

// Scope describes the document root a request targets. Tools that operate on a
// site's files (WordPress, Composer) and on its nginx logs use it so a subdomain
// is acted on in its own root instead of the parent domain's public_html.
type Scope struct {
	SubdomainID int64
	FQDN        string
	DocRoot     string
	PHPVersion  string
	// HasTLS reports whether the subdomain's certificate pair is installed, so callers
	// can build a site URL with the scheme nginx actually serves.
	HasTLS bool
}

// ResolveScope loads the scope for a subdomain that belongs to domainID. It returns
// ok=false when the subdomain does not exist or belongs to another domain, so a
// tenant cannot reach another domain's files by guessing an id.
func ResolveScope(ctx context.Context, db *sql.DB, domainID, subdomainID int64) (Scope, bool) {
	if subdomainID <= 0 {
		return Scope{}, false
	}
	var systemUser, name, fqdn, phpVersion string
	if err := db.QueryRowContext(ctx,
		`SELECT d.system_user, s.subdomain, s.fqdn, COALESCE(s.php_version,'8.3')
		   FROM subdomains s JOIN domains d ON d.id = s.domain_id
		  WHERE s.id=? AND s.domain_id=?`, subdomainID, domainID).
		Scan(&systemUser, &name, &fqdn, &phpVersion); err != nil {
		return Scope{}, false
	}
	// Re-validate the stored values before they reach a filesystem path.
	if !subdomainPattern.MatchString(name) || provisioner.ValidateDomain(fqdn) != nil {
		return Scope{}, false
	}
	certPath, keyPath := certificatePaths(systemUser, fqdn)
	return Scope{
		SubdomainID: subdomainID,
		FQDN:        fqdn,
		DocRoot:     docrootOf(systemUser, fqdn),
		PHPVersion:  phpVersion,
		HasTLS:      fileExists(certPath) && fileExists(keyPath),
	}, true
}
func confPath(systemUser, subdomainName string) string {
	return "/etc/nginx/conf.d/sub_" + systemUser + "_" + subdomainName + ".conf"
}

// GET /domains/{id}/subdomain lists subdomains.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id, systemUser, _, _, _, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, subdomain, fqdn, php_version, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i') FROM subdomains WHERE domain_id=? ORDER BY subdomain`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list records")
		return
	}
	defer func() { _ = rows.Close() }()
	out := []Sub{}
	for rows.Next() {
		var s Sub
		if err := rows.Scan(&s.ID, &s.Subdomain, &s.FQDN, &s.PHPVersion, &s.CreatedAt); err == nil {
			s.DocRoot = docrootOf(systemUser, s.FQDN)
			out = append(out, s)
		}
	}
	_ = rows.Err()
	httpx.WriteJSON(w, http.StatusOK, out)
}

// POST /domains/{id}/subdomain creates a subdomain from {subdomain, php_version?}.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id, systemUser, domainName, parentPHP, demo, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "not available for demo subscriptions")
		return
	}
	if !strings.HasPrefix(systemUser, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user")
		return
	}
	var req struct {
		Subdomain  string `json:"subdomain"`
		PHPVersion string `json:"php_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	subdomainName := strings.ToLower(strings.TrimSpace(req.Subdomain))
	if !subdomainPattern.MatchString(subdomainName) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid subdomain (lowercase letters, digits, and hyphens only)")
		return
	}
	phpVersion := strings.TrimSpace(req.PHPVersion)
	if phpVersion == "" {
		phpVersion = parentPHP
	}
	fqdn := subdomainName + "." + domainName
	if err := provisioner.ValidateDomain(fqdn); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid domain name")
		return
	}
	// Reject conflicts with existing domains or subdomains.
	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM subdomains WHERE fqdn=?`, fqdn).Scan(&n)
	if n == 0 {
		_ = h.DB.QueryRow(`SELECT COUNT(*) FROM domains WHERE domain_name=?`, fqdn).Scan(&n)
	}
	if n > 0 {
		httpx.WriteError(w, http.StatusConflict, "this domain name is already in use")
		return
	}
	socket, err := provisioner.PHPSocketFor(systemUser, phpVersion)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "PHP version is not installed on the server: "+phpVersion)
		return
	}
	docroot := docrootOf(systemUser, fqdn)
	// #nosec G301 G703 -- root-owned system directory whose daemon (nginx/php-fpm/named) must traverse it; contains no secret material.
	if err := os.MkdirAll(docroot, 0o755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create document root")
		return
	}
	// Create the initial landing page.
	// #nosec G703 -- docroot derives from a validated systemUser/subdomain path, not raw tenant input.
	if _, e := os.Stat(filepath.Join(docroot, "index.html")); e != nil {
		// #nosec G306 G703 -- root-owned welcome page under a validated tenant docroot that nginx must read; no secret and no raw tenant path input.
		_ = os.WriteFile(filepath.Join(docroot, "index.html"),
			[]byte(provisioner.WelcomeHTML(fqdn)), 0o644)
	}
	// #nosec G204 G702 -- fixed binary with separate args (no shell); tenant input is validated before exec.
	_ = exec.Command("chown", "-R", systemUser+":"+systemUser, "/home/"+systemUser+"/subdomains").Run()
	// #nosec G204 G702 -- fixed binary with separate args (no shell); tenant input is validated before exec.
	_ = exec.Command("chcon", "-R", "-t", "httpd_sys_content_t", docroot).Run()

	// Write the nginx server block.
	conf := confPath(systemUser, subdomainName)
	// A newly created subdomain has no protected directories yet.
	// #nosec G306 G703 -- root-owned system integration file (nginx/php-fpm/named/systemd config, script, or web content) that its daemon must read/execute; no secret stored here (secrets use 0600/0640).
	if err := os.WriteFile(conf, []byte(vhost(fqdn, docroot, socket, "")), 0o644); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not write virtual host configuration")
		return
	}
	// #nosec G204 G702 -- fixed binary with separate args (no shell); tenant input is validated before exec.
	_ = exec.Command("restorecon", conf).Run()
	if _, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		// #nosec G703 -- path is built from a validated identifier (systemUser ^c_[A-Za-z0-9_]+$ / validated domainName), a fixed system path, or a server-internal temp path; tenant file-manager paths use safeio (openat2) instead.
		_ = os.Remove(conf) // Remove the invalid configuration so the running nginx instance remains unaffected.
		_ = exec.Command("nginx", "-t").Run()
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		// Config validated with `nginx -t` above but reload failed: the vhost is on
		// disk yet not live. Remove it and report failure rather than a false success.
		// #nosec G703 -- path is built from a validated identifier (systemUser ^c_[A-Za-z0-9_]+$ / validated domainName), a fixed system path, or a server-internal temp path; tenant file-manager paths use safeio (openat2) instead.
		_ = os.Remove(conf)
		// #nosec G706 -- logged values are integer IDs, validated identifiers (^c_[A-Za-z0-9_]+$), template-derived names, or error/command output; no raw tenant string with CR/LF reaches the log.
		log.Printf("nginx reload after subdomain create %s: %v: %s", fqdn, err, strings.TrimSpace(string(out)))
		httpx.WriteError(w, http.StatusInternalServerError, "subdomain configured but nginx reload failed")
		return
	}

	if _, err := h.DB.Exec(`INSERT INTO subdomains (domain_id, subdomain, fqdn, php_version) VALUES (?,?,?,?)`,
		// #nosec G703 -- path is built from a validated identifier (systemUser ^c_[A-Za-z0-9_]+$ / validated domainName), a fixed system path, or a server-internal temp path; tenant file-manager paths use safeio (openat2) instead.
		id, subdomainName, fqdn, phpVersion); err != nil {
		// #nosec G703 -- path built from a validated identifier / fixed system path / server-internal temp path; tenant paths use safeio (openat2).
		_ = os.Remove(conf)
		_ = exec.Command("systemctl", "reload", "nginx").Run()
		httpx.WriteError(w, http.StatusInternalServerError, "could not add record")
		return
	}
	// Add the DNS A record to the parent zone and rewrite the zone file.
	if h.IPv4 != "" {
		_, _ = h.DB.Exec(`INSERT INTO dns_records (domain_id, name, type, value, ttl, priority, enabled) VALUES (?,?,?,?,?,?,1)`,
			id, subdomainName, "A", h.IPv4, 3600, 0)
		_ = dns.WriteZone(r.Context(), h.DB, id)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "fqdn": fqdn, "docroot": docroot})
}

// DELETE /domains/{id}/subdomain/{sid} removes a subdomain.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, systemUser, _, _, demo, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "not available for demo subscriptions")
		return
	}
	if !strings.HasPrefix(systemUser, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user")
		return
	}
	sid, _ := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
	var subdomainName, fqdn string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT subdomain, fqdn FROM subdomains WHERE id=? AND domain_id=?`, sid, id).Scan(&subdomainName, &fqdn); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	// Delete the DB rows FIRST so a DB failure aborts before the filesystem is
	// touched; otherwise a swallowed delete leaves a dangling record pointing at a
	// removed vhost.
	if _, err := h.DB.Exec(`DELETE FROM subdomains WHERE id=? AND domain_id=?`, sid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete subdomain")
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM dns_records WHERE domain_id=? AND name=? AND type='A'`, id, subdomainName); err != nil {
		log.Printf("delete subdomain DNS record %s: %v", subdomainName, err)
		// #nosec G703 -- path is built from a validated identifier (systemUser ^c_[A-Za-z0-9_]+$ / validated domainName), a fixed system path, or a server-internal temp path; tenant file-manager paths use safeio (openat2) instead.
	}
	// #nosec G703 -- path built from a validated identifier / fixed system path / server-internal temp path; tenant paths use safeio (openat2).
	_ = os.Remove(confPath(systemUser, subdomainName))
	_ = exec.Command("systemctl", "reload", "nginx").Run()
	// Remove the document root only when it remains under subdomains and matches the FQDN.
	docroot := docrootOf(systemUser, fqdn)
	base := "/home/" + systemUser + "/subdomains/"
	// #nosec G703 -- path is built from a validated identifier (systemUser ^c_[A-Za-z0-9_]+$ / validated domainName), a fixed system path, or a server-internal temp path; tenant file-manager paths use safeio (openat2) instead.
	if strings.HasPrefix(docroot, base) && filepath.Clean(docroot) != filepath.Clean(base) {
		_ = os.RemoveAll(docroot)
	}
	if err := dns.WriteZone(r.Context(), h.DB, id); err != nil {
		log.Printf("write DNS zone after subdomain delete %s: %v", subdomainName, err)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// vhost renders the plain HTTP server block. protected carries the auth_basic
// blocks for this subdomain's protected directories and is empty when none exist.
func vhost(fqdn, docroot, socket, protected string) string {
	return `server {
    listen 80;
    listen [::]:80;
    server_name ` + fqdn + `;

    root ` + docroot + `;
    index index.php index.html index.htm;

    access_log /var/log/nginx/` + fqdn + `.access.log;
    error_log  /var/log/nginx/` + fqdn + `.error.log warn;

    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

` + protected + `
    location /.well-known/acme-challenge/ {
        auth_basic off;
        root /var/www/_acme;
        try_files $uri =404;
    }

    error_page 404 /_srv_404.html;
    location = /_srv_404.html {
        root /usr/share/servika/errors;
        internal;
        access_log off;
    }
    location ^~ /_srv/ {
        alias /usr/share/servika/errors/;
        access_log off;
        expires 7d;
        gzip on;
        gzip_types application/json application/javascript;
    }

    location / { try_files $uri $uri/ /index.php?$query_string; }

    location ~ \.php$ {
        try_files $uri =404;
        fastcgi_split_path_info ^(.+\.php)(/.+)$;
        fastcgi_pass unix:` + socket + `;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_read_timeout 60s;
    }

    location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2?|svg|webp|avif|pdf|zip|gz)$ {
        expires 30d;
        add_header Cache-Control "public";
        access_log off;
    }

    location ~ /\.(?!well-known) { deny all; }

    # Servika subdomain — ` + fqdn + `
}
`
}
