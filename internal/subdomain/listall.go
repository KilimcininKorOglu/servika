package subdomain

import (
	"net/http"

	"servika/internal/httpx"
	"servika/internal/middleware"
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
	// SSL and SSLSource let a subdomain row report its certificate the same way
	// the domain row above it does, instead of a badge that only says what kind
	// of record it is.
	SSL       bool   `json:"ssl"`
	SSLSource string `json:"ssl_source"`
}

// GET /subdomains lists the subdomains the caller may see, each with its parent domain, so
// the Domains screen can nest subdomains under the domain they belong to and global search
// can offer them.
//
// The scope is applied inside the query with middleware.ScopeSQL: a row-by-row ownership
// check cannot secure a list endpoint, and this route is open to resellers, who must see
// only the subdomains under their own customers.
func (h *Handlers) ListAll(w http.ResponseWriter, r *http.Request) {
	condition, args := middleware.ScopeSQL(r, "d")
	// #nosec G701 G202 -- condition is a constant scope fragment from ScopeSQL with a literal alias; all user values are bound via args placeholders.
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT s.id, s.subdomain, s.fqdn, s.domain_id, d.domain_name, d.system_user,
		        s.php_version, DATE_FORMAT(s.created_at,'%Y-%m-%d %H:%i')
		   FROM subdomains s JOIN domains d ON d.id = s.domain_id`+condition+`
		  ORDER BY d.domain_name, s.subdomain`, args...)
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
			s.SSL, s.SSLSource = sslState(s.SystemUser, s.Subdomain, s.FQDN)
			out = append(out, s)
		}
	}
	_ = rows.Err()
	httpx.WriteJSON(w, http.StatusOK, out)
}
