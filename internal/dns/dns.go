// Package dns manages per-domain DNS records and BIND zone files.
package dns

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// Record describes a DNS record stored for a domain.
type Record struct {
	ID        int64  `json:"id"`
	DomainID  int64  `json:"domain_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	TTL       int    `json:"ttl"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

// ValidTypes contains the supported DNS record types.
var ValidTypes = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA", "PTR", "DS", "TLSA", "SSHFP", "NAPTR"}

var dnsRecordNamePattern = regexp.MustCompile(`^(@|\*|[A-Za-z0-9_{}*-]+(?:\.[A-Za-z0-9_{}*-]+)*)$`)

// Handlers provides DNS record HTTP handlers.
type Handlers struct {
	DB *sql.DB
}

const selectAll = `SELECT id, domain_id, name, type, value, ttl, priority, enabled,
  DATE_FORMAT(created_at,'%Y-%m-%d %H:%i') FROM dns_records`

func scan(rs interface{ Scan(...any) error }) (Record, error) {
	var record Record
	var enabledValue int
	err := rs.Scan(&record.ID, &record.DomainID, &record.Name, &record.Type, &record.Value, &record.TTL, &record.Priority, &enabledValue, &record.CreatedAt)
	record.Enabled = enabledValue == 1
	return record, err
}

func (h *Handlers) lookup(r *http.Request) (string, bool, error) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, is_demo FROM domains WHERE id=?`, id).Scan(&domainName, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, os.ErrNotExist
	}
	return domainName, isDemo == 1, err
}

// List returns all DNS records for a domain.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rows, err := h.DB.QueryContext(r.Context(), selectAll+" WHERE domain_id=? ORDER BY type, name", id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = rows.Close() }()
	out := make([]Record, 0)
	for rows.Next() {
		record, err := scan(rows)
		if err == nil {
			out = append(out, record)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Create adds a DNS record to a domain.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, isDemo, err := h.lookup(r)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if isDemo {
		httpx.WriteError(w, http.StatusForbidden, "dNS records are read-only for demo subscriptions")
		return
	}
	var record Record
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	record.Name = strings.TrimSpace(record.Name)
	if record.Name == "" {
		record.Name = "@"
	}
	record.Type = strings.ToUpper(strings.TrimSpace(record.Type))
	if !validType(record.Type) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid DNS record type")
		return
	}
	if !validRecordFields(record.Name, record.Value) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid DNS record")
		return
	}
	if record.Name == "" {
		record.Name = "@"
	}
	if record.TTL <= 0 {
		record.TTL = 3600
	}
	record.Priority = normalizePriority(record.Type, record.Priority)
	enabledValue := 0
	if record.Enabled {
		enabledValue = 1
	}
	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO dns_records(domain_id, name, type, value, ttl, priority, enabled)
		 VALUES(?,?,?,?,?,?,?)`,
		id, record.Name, record.Type, record.Value, record.TTL, record.Priority, enabledValue)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	nid, _ := res.LastInsertId()
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE id=?", nid)
	saved, _ := scan(row)
	_ = WriteZone(r.Context(), h.DB, id)
	httpx.WriteJSON(w, http.StatusCreated, saved)
}

// Update replaces a DNS record for a domain.
func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rid, _ := strconv.ParseInt(chi.URLParam(r, "rid"), 10, 64)
	_, isDemo, err := h.lookup(r)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if isDemo {
		httpx.WriteError(w, http.StatusForbidden, "dNS records are read-only for demo subscriptions")
		return
	}
	var record Record
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	record.Name = strings.TrimSpace(record.Name)
	if record.Name == "" {
		record.Name = "@"
	}
	record.Type = strings.ToUpper(strings.TrimSpace(record.Type))
	if !validType(record.Type) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid DNS record type")
		return
	}
	if !validRecordFields(record.Name, record.Value) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid DNS record")
		return
	}
	record.Priority = normalizePriority(record.Type, record.Priority)
	enabledValue := 0
	if record.Enabled {
		enabledValue = 1
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE dns_records SET name=?, type=?, value=?, ttl=?, priority=?, enabled=?
		 WHERE id=? AND domain_id=?`,
		record.Name, record.Type, record.Value, record.TTL, record.Priority, enabledValue, rid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE id=? AND domain_id=?", rid, id)
	saved, _ := scan(row)
	_ = WriteZone(r.Context(), h.DB, id)
	httpx.WriteJSON(w, http.StatusOK, saved)
}

// Delete removes a DNS record from a domain.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rid, _ := strconv.ParseInt(chi.URLParam(r, "rid"), 10, 64)
	_, isDemo, err := h.lookup(r)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if isDemo {
		httpx.WriteError(w, http.StatusForbidden, "dNS records are read-only for demo subscriptions")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM dns_records WHERE id=? AND domain_id=?`, rid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// BulkDelete removes selected DNS records in one request.
func (h *Handlers) BulkDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, isDemo, err := h.lookup(r)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if isDemo {
		httpx.WriteError(w, http.StatusForbidden, "dNS records are read-only for demo subscriptions")
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "no records selected")
		return
	}
	ph := make([]string, len(req.IDs))
	args := make([]any, 0, len(req.IDs)+1)
	for i, rid := range req.IDs {
		ph[i] = "?"
		args = append(args, rid)
	}
	args = append(args, id)
	res, err := h.DB.ExecContext(r.Context(),
		"DELETE FROM dns_records WHERE id IN ("+strings.Join(ph, ",")+") AND domain_id=?", args...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	n, _ := res.RowsAffected()
	_ = WriteZone(r.Context(), h.DB, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": n})
}

// BulkStatus enables or disables selected DNS records.
func (h *Handlers) BulkStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, isDemo, err := h.lookup(r)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if isDemo {
		httpx.WriteError(w, http.StatusForbidden, "dNS records are read-only for demo subscriptions")
		return
	}
	var req struct {
		IDs     []int64 `json:"ids"`
		Enabled bool    `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "no records selected")
		return
	}
	enabledValue := 0
	if req.Enabled {
		enabledValue = 1
	}
	ph := make([]string, len(req.IDs))
	args := make([]any, 0, len(req.IDs)+2)
	args = append(args, enabledValue)
	for i, rid := range req.IDs {
		ph[i] = "?"
		args = append(args, rid)
	}
	args = append(args, id)
	res, err := h.DB.ExecContext(r.Context(),
		"UPDATE dns_records SET enabled=? WHERE id IN ("+strings.Join(ph, ",")+") AND domain_id=?", args...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	n, _ := res.RowsAffected()
	_ = WriteZone(r.Context(), h.DB, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": n})
}

// ApplyTemplate inserts the default DNS records without duplicating existing records.
func (h *Handlers) ApplyTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	domainName, isDemo, err := h.lookup(r)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if isDemo {
		httpx.WriteError(w, http.StatusForbidden, "dNS templates are unavailable for demo subscriptions")
		return
	}
	var ipv4 string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT ipv4 FROM domains WHERE id=?`, id).Scan(&ipv4)
	n, err := SeedDefaults(r.Context(), h.DB, id, domainName, ipv4)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	_ = WriteZone(r.Context(), h.DB, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "added": n})
}

// SeedDefaults creates records from the server-wide DNS template without duplicating them.
func SeedDefaults(ctx context.Context, db *sql.DB, domainID int64, domainName, ipv4 string) (int, error) {
	if ipv4 == "" {
		ipv4 = "127.0.0.1"
	}
	rows, err := LoadTemplate(ctx, db)
	if err != nil || len(rows) == 0 {
		rows = builtinDefaults()
	}
	meta := LoadTemplateMeta(ctx, db)
	selector := meta.DKIMSelector

	dkimTXT := ""
	if meta.DKIMEnabled {
		for _, row := range rows {
			if row.Enabled && strings.Contains(row.Value, "{DKIM}") {
				dkimTXT, err = EnsureDKIM(ctx, db, domainID, domainName, selector)
				if err != nil {
					log.Printf("generate DKIM key domain=%d: %v", domainID, err)
				}
				break
			}
		}
	}

	added := 0
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		if strings.Contains(row.Value, "{DKIM}") && (!meta.DKIMEnabled || dkimTXT == "") {
			continue
		}
		name := substituteTemplate(row.Name, domainName, ipv4, selector, dkimTXT)
		value := substituteTemplate(row.Value, domainName, ipv4, selector, dkimTXT)
		recordType := strings.ToUpper(strings.TrimSpace(row.Type))
		var count int
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM dns_records WHERE domain_id=? AND name=? AND type=? AND value=?`,
			domainID, name, recordType, value).Scan(&count)
		if count > 0 {
			continue
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO dns_records(domain_id, name, type, value, ttl, priority, enabled)
			 VALUES(?,?,?,?,?,?,1)`,
			domainID, name, recordType, value, row.TTL, normalizePriority(recordType, row.Priority)); err != nil {
			log.Printf("dns seed %s/%s: %v", name, recordType, err)
			continue
		}
		added++
	}
	seedSOAFromMeta(ctx, db, domainID, domainName, meta)
	return added, nil
}

func seedSOAFromMeta(ctx context.Context, db *sql.DB, domainID int64, domainName string, meta TemplateMeta) {
	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dns_soa WHERE domain_id=?`, domainID).Scan(&count)
	if count > 0 {
		return
	}
	defaults := defaultSOA(domainName)
	_, _ = db.ExecContext(ctx,
		`INSERT INTO dns_soa(domain_id, primary_ns, hostmaster, refresh, retry, expire, minimum, ttl)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE domain_id=domain_id`,
		domainID, defaults.PrimaryNS, defaults.Hostmaster, meta.SOARefresh, meta.SOARetry,
		meta.SOAExpire, meta.SOAMinimum, meta.SOATTL)
}

// normalizePriority restricts priority values to record types that support them.
func normalizePriority(recordType string, priority int) int {
	if recordType == "MX" || recordType == "SRV" {
		return priority
	}
	return 0
}

func validRecordFields(name, value string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "@"
	}
	value = strings.TrimSpace(value)
	if !dnsRecordNamePattern.MatchString(name) || value == "" {
		return false
	}
	if strings.HasPrefix(name, "$") || strings.HasPrefix(value, "$") {
		return false
	}
	return !strings.ContainsAny(name+value, "\r\n\x00")
}

func validType(t string) bool {
	return slices.Contains(ValidTypes, t)
}
