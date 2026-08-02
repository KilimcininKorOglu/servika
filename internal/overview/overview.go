// Package overview serves server-wide, read-only summary lists.
//
// Every existing DNS/SSL/mail/database endpoint is domain-scoped
// (/domains/{id}/dns and friends). This package provides their server-wide
// counterpart: "which certificate expires when", "which domain is missing an
// MX record", "how many mailboxes in total" answered without walking domains
// one by one. It only reads — edits still go through the domain-scoped
// endpoints, so authorization and validation logic stays in one place.
//
// Access: admin + reseller (ResellerOrAbove). Lists are narrowed with
// middleware.ScopeSQL — an admin sees every domain, a reseller only its own
// customers', a customer only its own. The narrowing is inside the query;
// filtering row by row would not prevent leaks on list endpoints.
package overview

import (
	"context"
	"database/sql"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"servika/internal/httpx"
	"servika/internal/middleware"
)

// Handlers provides the server-wide overview HTTP handlers.
type Handlers struct{ DB *sql.DB }

// ---------- DNS ----------

type DNSRow struct {
	DomainID    int64  `json:"domain_id"`
	DomainName  string `json:"domain_name"`
	Status      string `json:"status"`
	RecordCount int    `json:"record_count"`
	ACount      int    `json:"a_count"`
	MXCount     int    `json:"mx_count"`
	TXTCount    int    `json:"txt_count"`
	DisabledN   int    `json:"disabled_count"`
	DNSSEC      bool   `json:"dnssec_active"`
}

func (h *Handlers) DNS(w http.ResponseWriter, r *http.Request) {
	q := `
SELECT d.id, d.domain_name, d.status, d.dnssec_active,
       COUNT(r.id),
       COALESCE(SUM(r.type='A'), 0),
       COALESCE(SUM(r.type='MX'), 0),
       COALESCE(SUM(r.type='TXT'), 0),
       COALESCE(SUM(r.enabled=0), 0)
FROM domains d
LEFT JOIN dns_records r ON r.domain_id = d.id`

	cond, arg := middleware.ScopeSQL(r, "d")
	// #nosec G202 -- cond is a constant scope fragment from ScopeSQL with a literal alias; user values are bound via arg placeholders.
	q += cond + `
GROUP BY d.id, d.domain_name, d.status, d.dnssec_active
ORDER BY d.domain_name`

	// #nosec G701 G202 -- cond is a constant scope fragment from ScopeSQL with a literal alias; all user values are bound via arg placeholders.
	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "dns overview failed")
		return
	}
	defer func() { _ = rows.Close() }() // read-only query: closing the result set has nothing to flush

	out := make([]DNSRow, 0)
	for rows.Next() {
		var s DNSRow
		var dnssec int
		if err := rows.Scan(&s.DomainID, &s.DomainName, &s.Status, &dnssec,
			&s.RecordCount, &s.ACount, &s.MXCount, &s.TXTCount, &s.DisabledN); err != nil {
			continue
		}
		s.DNSSEC = dnssec == 1
		out = append(out, s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ---------- SSL ----------

type SSLRow struct {
	DomainID      int64  `json:"domain_id"`
	DomainName    string `json:"domain_name"`
	Status        string `json:"status"`
	Enabled       bool   `json:"ssl_enabled"`
	Expiry        string `json:"ssl_expiry"` // YYYY-MM-DD, "" if unknown
	RemainingDays *int   `json:"remaining_days"`
}

func (h *Handlers) SSL(w http.ResponseWriter, r *http.Request) {
	// Ordered for the screen's real job: expired/expiring certificates first,
	// then future-dated ones, and domains with no SSL at all last.
	q := `
SELECT d.id, d.domain_name, d.status, d.ssl_enabled,
       COALESCE(DATE_FORMAT(d.ssl_expiry, '%Y-%m-%d'), ''),
       CASE WHEN d.ssl_expiry IS NULL THEN NULL ELSE DATEDIFF(d.ssl_expiry, CURDATE()) END
FROM domains d`

	cond, arg := middleware.ScopeSQL(r, "d")
	// #nosec G202 -- cond is a constant scope fragment from ScopeSQL with a literal alias; user values are bound via arg placeholders.
	q += cond + `
ORDER BY (d.ssl_expiry IS NULL), d.ssl_expiry ASC, d.domain_name`

	// #nosec G701 G202 -- cond is a constant scope fragment from ScopeSQL with a literal alias; all user values are bound via arg placeholders.
	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "ssl overview failed")
		return
	}
	defer func() { _ = rows.Close() }() // read-only query: closing the result set has nothing to flush

	out := make([]SSLRow, 0)
	for rows.Next() {
		var s SSLRow
		var enabled int
		var remaining sql.NullInt64
		if err := rows.Scan(&s.DomainID, &s.DomainName, &s.Status, &enabled, &s.Expiry, &remaining); err != nil {
			continue
		}
		s.Enabled = enabled == 1
		if remaining.Valid {
			d := int(remaining.Int64)
			s.RemainingDays = &d
		}
		out = append(out, s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ---------- Mail ----------

type MailRow struct {
	DomainID     int64  `json:"domain_id"`
	DomainName   string `json:"domain_name"`
	MailEnabled  bool   `json:"mail_enabled"`
	MailStatus   string `json:"mail_status"` // active | suspended | "" (never provisioned)
	MailboxCount int    `json:"mailbox_count"`
	AliasCount   int    `json:"alias_count"`
	SuspendedBox int    `json:"suspended_mailbox_count"`
}

func (h *Handlers) Mail(w http.ResponseWriter, r *http.Request) {
	// Subqueries are used: JOINing mailboxes and mail_aliases at once produces a
	// cartesian product and inflates the counts.
	q := `
SELECT d.id, d.domain_name,
       COALESCE(md.status, ''),
       (SELECT COUNT(*) FROM mailboxes mb WHERE mb.domain_id = d.id),
       (SELECT COUNT(*) FROM mail_aliases a WHERE a.domain_id = d.id),
       (SELECT COUNT(*) FROM mailboxes mb2 WHERE mb2.domain_id = d.id AND mb2.status = 'suspended')
FROM domains d
LEFT JOIN mail_domains md ON md.domain_id = d.id`

	cond, arg := middleware.ScopeSQL(r, "d")
	// #nosec G202 -- cond is a constant scope fragment from ScopeSQL with a literal alias; user values are bound via arg placeholders.
	q += cond + `
ORDER BY d.domain_name`

	// #nosec G701 -- cond is a constant scope fragment from ScopeSQL with a literal alias; all user values are bound via arg placeholders.
	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mail overview failed")
		return
	}
	defer func() { _ = rows.Close() }() // read-only query: closing the result set has nothing to flush

	out := make([]MailRow, 0)
	for rows.Next() {
		var s MailRow
		if err := rows.Scan(&s.DomainID, &s.DomainName, &s.MailStatus,
			&s.MailboxCount, &s.AliasCount, &s.SuspendedBox); err != nil {
			continue
		}
		s.MailEnabled = s.MailStatus != ""
		out = append(out, s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ---------- Databases ----------

type DBRow struct {
	ID         int64  `json:"id"`
	DomainID   int64  `json:"domain_id"`
	DomainName string `json:"domain_name"`
	DBName     string `json:"db_name"`
	DBUser     string `json:"db_user"`
	DBHost     string `json:"db_host"`
	SizeKB     int64  `json:"size_kb"`
	CreatedAt  string `json:"created_at"`
}

// dbSizes: schema name -> KB.
//
// The panel's own DSN (the `panel` user) is only privileged on the `panel`
// schema; because MySQL filters information_schema.TABLES by privilege, the
// size of customer databases is NEVER visible over that connection (it silently
// returns 0). So we follow the same path the panel uses when creating a
// database: the `mysql` client running as root's unix-socket identity (see
// internal/credentials). On error an empty map is returned — the size column
// shows "—" and the list still loads.
func dbSizes() map[string]int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "mysql", "-N", "-B", "-e",
		`SELECT table_schema, COALESCE(SUM(data_length + index_length), 0) DIV 1024
		 FROM information_schema.TABLES GROUP BY table_schema`).Output()
	if err != nil {
		return map[string]int64{}
	}
	return parseDBSizes(out)
}

// parseDBSizes turns the tab-separated `mysql -N -B` output ("schema\tKB" per
// line) into a schema->KB map. Malformed lines (wrong field count, non-numeric
// size) are skipped so one bad row never blanks the whole list. Kept separate
// from dbSizes so this untrusted-subprocess parsing is unit-testable.
func parseDBSizes(raw []byte) map[string]int64 {
	sizes := make(map[string]int64)
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		field := strings.Split(line, "\t")
		if len(field) != 2 {
			continue
		}
		kb, err := strconv.ParseInt(strings.TrimSpace(field[1]), 10, 64)
		if err != nil {
			continue
		}
		sizes[strings.TrimSpace(field[0])] = kb
	}
	return sizes
}

func (h *Handlers) Databases(w http.ResponseWriter, r *http.Request) {
	q := `
SELECT a.id, a.domain_id, d.domain_name, a.db_name, a.db_user, a.db_host,
       COALESCE(DATE_FORMAT(a.created_at, '%Y-%m-%d'), '')
FROM db_accounts a
JOIN domains d ON d.id = a.domain_id`

	cond, arg := middleware.ScopeSQL(r, "d")
	// #nosec G202 -- cond is a constant scope fragment from ScopeSQL with a literal alias; user values are bound via arg placeholders.
	q += cond + `
ORDER BY d.domain_name, a.db_name`

	// #nosec G701 G202 -- cond is a constant scope fragment from ScopeSQL with a literal alias; all user values are bound via arg placeholders.
	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "databases overview failed")
		return
	}
	defer func() { _ = rows.Close() }() // read-only query: closing the result set has nothing to flush

	out := make([]DBRow, 0)
	for rows.Next() {
		var s DBRow
		if err := rows.Scan(&s.ID, &s.DomainID, &s.DomainName, &s.DBName, &s.DBUser, &s.DBHost, &s.CreatedAt); err != nil {
			continue
		}
		out = append(out, s)
	}

	sizes := dbSizes()
	for i := range out {
		out[i].SizeKB = sizes[out[i].DBName]
	}

	httpx.WriteJSON(w, http.StatusOK, out)
}
