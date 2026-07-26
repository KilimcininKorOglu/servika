package subdomain

import (
	"net/http"

	"servika/internal/httpx"
)

// Overview is a subdomain enriched with its parent domain, for the Domains screen.
type Overview struct {
	ID         int64  `json:"id"`
	Subdomain  string `json:"subdomain"`
	FQDN       string `json:"fqdn"`
	ParentID   int64  `json:"parent_id"`
	ParentName string `json:"parent_name"`
	SystemUser string `json:"system_user"`
	PHPVersion string `json:"php_version"`
	DocRoot    string `json:"docroot"`
	CreatedAt  string `json:"created_at"`
}

// GET /subdomains lists every subdomain with its parent domain so the Domains
// screen can nest subdomains under the domain they belong to.
func (h *Handlers) ListAll(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT s.id, s.subdomain, s.fqdn, s.domain_id, d.domain_name, d.system_user,
		        s.php_version, DATE_FORMAT(s.created_at,'%Y-%m-%d %H:%i')
		   FROM subdomains s JOIN domains d ON d.id = s.domain_id
		  ORDER BY d.domain_name, s.subdomain`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list records")
		return
	}
	defer func() { _ = rows.Close() }()
	out := []Overview{}
	for rows.Next() {
		var s Overview
		if err := rows.Scan(&s.ID, &s.Subdomain, &s.FQDN, &s.ParentID, &s.ParentName,
			&s.SystemUser, &s.PHPVersion, &s.CreatedAt); err == nil {
			s.DocRoot = docrootOf(s.SystemUser, s.FQDN)
			out = append(out, s)
		}
	}
	_ = rows.Err()
	httpx.WriteJSON(w, http.StatusOK, out)
}
