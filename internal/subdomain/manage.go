package subdomain

import (
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"servika/internal/httpx"

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
