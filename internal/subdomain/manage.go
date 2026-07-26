package subdomain

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// GET /domains/{id}/subdomain/{sid} returns a single subdomain with the data its
// management page needs. The lookup is scoped to the parent domain so a tenant
// cannot read a subdomain that belongs to another domain.
func (h *Handlers) Detail(w http.ResponseWriter, r *http.Request) {
	id, systemUser, domainName, _, _, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	sid, _ := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
	var s Sub
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, subdomain, fqdn, php_version, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		   FROM subdomains WHERE id=? AND domain_id=?`, sid, id).
		Scan(&s.ID, &s.Subdomain, &s.FQDN, &s.PHPVersion, &s.CreatedAt); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	s.DocRoot = docrootOf(systemUser, s.FQDN)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":          s.ID,
		"subdomain":   s.Subdomain,
		"fqdn":        s.FQDN,
		"php_version": s.PHPVersion,
		"docroot":     s.DocRoot,
		"created_at":  s.CreatedAt,
		"parent_id":   id,
		"parent_name": domainName,
		"disk_kb":     docrootDiskKB(s.DocRoot),
		"ipv4":        h.IPv4,
	})
}

// PUT /domains/{id}/subdomain/{sid}/php switches the subdomain's PHP-FPM pool
// from {php_version}. The vhost is rewritten with the new socket and validated
// before the record is updated, so a rejected configuration leaves both nginx and
// the database untouched. The rewrite is TLS-aware: an existing certificate keeps
// the HTTPS vhost, so switching PHP never drops the site to plain HTTP.
func (h *Handlers) SetPHP(w http.ResponseWriter, r *http.Request) {
	id, systemUser, _, _, demo, ok := h.parent(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "not available on a demo subscription")
		return
	}
	if !strings.HasPrefix(systemUser, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid system user")
		return
	}
	var req struct {
		PHPVersion string `json:"php_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	phpVersion := strings.TrimSpace(req.PHPVersion)
	sid, _ := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
	var subdomainName, fqdn string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT subdomain, fqdn FROM subdomains WHERE id=? AND domain_id=?`, sid, id).
		Scan(&subdomainName, &fqdn); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	socket, err := provisioner.PHPSocketFor(systemUser, phpVersion)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "PHP version is not installed on the server")
		return
	}
	docroot := docrootOf(systemUser, fqdn)
	certPath, keyPath := certificatePaths(systemUser, fqdn)
	config := vhost(fqdn, docroot, socket)
	if fileExists(certPath) && fileExists(keyPath) {
		config = vhostSSL(fqdn, docroot, socket, certPath, keyPath)
	}
	if err := applyVhost(confPath(systemUser, subdomainName), config); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "nginx rejected the configuration")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE subdomains SET php_version=? WHERE id=? AND domain_id=?`, phpVersion, sid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not update the record")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "php_version": phpVersion})
}

// docrootDiskKB reports the document root size in kilobytes. It is best effort:
// a missing directory or a du failure reports 0 rather than failing the request.
func docrootDiskKB(docroot string) int64 {
	out, err := exec.Command("du", "-sk", docroot).Output()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0
	}
	kb, _ := strconv.ParseInt(fields[0], 10, 64)
	return kb
}
