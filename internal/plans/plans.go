// Package plans provides service-plan CRUD, seed data, and resource-limit fields.
package plans

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// Plan describes a service plan and its resource limits.
type Plan struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	DiskQuotaMB          int    `json:"disk_quota_mb"` // Zero means unlimited.
	TrafficQuotaMB       int    `json:"traffic_quota_mb"`
	MaxDomain            int    `json:"max_domain"`
	MaxDB                int    `json:"max_db"`
	MaxEmail             int    `json:"max_email"`
	MaxFTP               int    `json:"max_ftp"`
	CPUPercent           int    `json:"cpu_percent"` // 100 equals one CPU core.
	RAMMB                int    `json:"ram_mb"`      // Hard limit in MB.
	MaxProcess           int    `json:"max_process"` // systemd TasksMax.
	InodeQuota           int    `json:"inode_quota"`
	IOWeight             int    `json:"io_weight"` // systemd IOWeight from 1 to 1000.
	MySQLMaxConnections  int    `json:"mysql_max_connections"`
	PMMaxChildren        int    `json:"pm_max_children"`         // Zero derives the limit from plan memory.
	IOReadMBps           int    `json:"io_read_mbps"`            // Zero means unlimited.
	IOWriteMBps          int    `json:"io_write_mbps"`           // Zero means unlimited.
	IOReadIOPS           int    `json:"io_read_iops"`            // Zero means unlimited.
	IOWriteIOPS          int    `json:"io_write_iops"`           // Zero means unlimited.
	DBMaxQueriesPerHour  int    `json:"db_max_queries_per_hour"` // Zero means unlimited.
	DBMaxUpdatesPerHour  int    `json:"db_max_updates_per_hour"` // Zero means unlimited.
	DBMaxQuerySeconds    int    `json:"db_max_query_seconds"`    // Zero disables query termination.
	PHPVersion           string `json:"php_version"`
	FastCGICache         bool   `json:"fastcgi_cache"`
	ClientMaxBodyMB      int    `json:"client_max_body_mb"`
	NginxExtraDirectives string `json:"nginx_extra_directives"`
	// WAF (ModSecurity + OWASP CRS) plan defaults — domains in this plan inherit these values.
	WAFEnabled  bool   `json:"waf_enabled"`
	WAFMode     string `json:"waf_mode"`     // "on" (block) | "detect" (log only) | "off"
	WAFParanoia int    `json:"waf_paranoia"` // CRS paranoia 1..4
	IsDefault   bool   `json:"is_default"`
	CreatedAt   string `json:"created_at"`
}

// Handlers provides service plan HTTP handlers.
type Handlers struct {
	DB *sql.DB
}

const selectAll = `SELECT id, name, description, disk_quota_mb, traffic_quota_mb,
  max_domain, max_db, max_email, max_ftp,
  cpu_percent, ram_mb, max_process, inode_quota, io_weight, mysql_max_connections,
  COALESCE(pm_max_children,0),
  COALESCE(io_read_mbps,0), COALESCE(io_write_mbps,0),
  COALESCE(io_read_iops,0), COALESCE(io_write_iops,0),
  COALESCE(db_max_queries_per_hour,0), COALESCE(db_max_updates_per_hour,0),
  COALESCE(db_max_query_seconds,0),
  php_version, fastcgi_cache, client_max_body_mb, COALESCE(nginx_extra_directives,''),
  COALESCE(waf_enabled,0), COALESCE(waf_mode,'on'), COALESCE(waf_paranoia,1),
  is_default, DATE_FORMAT(created_at,'%Y-%m-%d') FROM service_plans`

func b01(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scan(rs interface{ Scan(...any) error }) (Plan, error) {
	var p Plan
	var vars, fc, wafEn int
	err := rs.Scan(&p.ID, &p.Name, &p.Description, &p.DiskQuotaMB, &p.TrafficQuotaMB,
		&p.MaxDomain, &p.MaxDB, &p.MaxEmail, &p.MaxFTP,
		&p.CPUPercent, &p.RAMMB, &p.MaxProcess, &p.InodeQuota, &p.IOWeight, &p.MySQLMaxConnections,
		&p.PMMaxChildren,
		&p.IOReadMBps, &p.IOWriteMBps, &p.IOReadIOPS, &p.IOWriteIOPS,
		&p.DBMaxQueriesPerHour, &p.DBMaxUpdatesPerHour, &p.DBMaxQuerySeconds,
		&p.PHPVersion, &fc, &p.ClientMaxBodyMB, &p.NginxExtraDirectives,
		&wafEn, &p.WAFMode, &p.WAFParanoia,
		&vars, &p.CreatedAt)
	p.IsDefault = vars == 1
	p.FastCGICache = fc == 1
	p.WAFEnabled = wafEn == 1
	return p, err
}

// List returns all service plans.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), selectAll+" ORDER BY is_default DESC, id ASC")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	defer func() { _ = rows.Close() }()
	out := make([]Plan, 0)
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Get returns a service plan and its assigned domain count.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE id=?", id)
	p, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "plan not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	// Count domains using the plan.
	var dCount int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM domains WHERE plan_id=?`, id).Scan(&dCount)
	resp := map[string]any{
		"plan":         p,
		"domain_count": dCount,
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func fillDefaults(p *Plan) {
	if p.CPUPercent == 0 {
		p.CPUPercent = 100
	}
	if p.RAMMB == 0 {
		p.RAMMB = 512
	}
	if p.MaxProcess == 0 {
		p.MaxProcess = 50
	}
	if p.InodeQuota == 0 {
		p.InodeQuota = 50000
	}
	if p.IOWeight == 0 {
		p.IOWeight = 100
	}
	if p.MySQLMaxConnections == 0 {
		p.MySQLMaxConnections = 25
	}
	if strings.TrimSpace(p.PHPVersion) == "" {
		p.PHPVersion = "8.3"
	}
	if p.ClientMaxBodyMB == 0 {
		p.ClientMaxBodyMB = 64
	}
	// WAF defaults
	switch strings.ToLower(strings.TrimSpace(p.WAFMode)) {
	case "on", "detect", "off":
		p.WAFMode = strings.ToLower(strings.TrimSpace(p.WAFMode))
	default:
		p.WAFMode = "on"
	}
	if p.WAFParanoia < 1 || p.WAFParanoia > 4 {
		p.WAFParanoia = 1
	}
}

// Create creates a service plan.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var p Plan
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "plan name is required")
		return
	}
	fillDefaults(&p)
	if err := provisioner.ValidateNginxDirectives(p.NginxExtraDirectives); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid nginx directives")
		return
	}
	v := 0
	if p.IsDefault {
		v = 1
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE service_plans SET is_default=0`)
	}
	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO service_plans(name, description, disk_quota_mb, traffic_quota_mb,
		   max_domain, max_db, max_email, max_ftp,
		   cpu_percent, ram_mb, max_process, inode_quota, io_weight, mysql_max_connections,
		   pm_max_children, io_read_mbps, io_write_mbps, io_read_iops, io_write_iops,
		   db_max_queries_per_hour, db_max_updates_per_hour, db_max_query_seconds,
		   php_version, fastcgi_cache, client_max_body_mb, nginx_extra_directives,
		   waf_enabled, waf_mode, waf_paranoia, is_default)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.Name, p.Description, p.DiskQuotaMB, p.TrafficQuotaMB,
		p.MaxDomain, p.MaxDB, p.MaxEmail, p.MaxFTP,
		p.CPUPercent, p.RAMMB, p.MaxProcess, p.InodeQuota, p.IOWeight, p.MySQLMaxConnections,
		p.PMMaxChildren, p.IOReadMBps, p.IOWriteMBps, p.IOReadIOPS, p.IOWriteIOPS,
		p.DBMaxQueriesPerHour, p.DBMaxUpdatesPerHour, p.DBMaxQuerySeconds,
		p.PHPVersion, b01(p.FastCGICache), p.ClientMaxBodyMB, p.NginxExtraDirectives,
		b01(p.WAFEnabled), p.WAFMode, p.WAFParanoia, v)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	id, _ := res.LastInsertId()
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE id=?", id)
	// WAF plan default may have changed — re-render vhosts for domains in this plan
	// whose WAF override is set to inherit. Runs in the background.
	go h.wafPlanReapply(id)

	saved, _ := scan(row)
	httpx.WriteJSON(w, http.StatusCreated, saved)
}

// Update updates a service plan.
func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var p Plan
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "plan name is required")
		return
	}
	fillDefaults(&p)
	if err := provisioner.ValidateNginxDirectives(p.NginxExtraDirectives); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid nginx directives")
		return
	}
	v := 0
	if p.IsDefault {
		v = 1
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE service_plans SET is_default=0 WHERE id<>?`, id)
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE service_plans SET name=?, description=?, disk_quota_mb=?, traffic_quota_mb=?,
		   max_domain=?, max_db=?, max_email=?, max_ftp=?,
		   cpu_percent=?, ram_mb=?, max_process=?, inode_quota=?, io_weight=?, mysql_max_connections=?,
		   pm_max_children=?, io_read_mbps=?, io_write_mbps=?, io_read_iops=?, io_write_iops=?,
		   db_max_queries_per_hour=?, db_max_updates_per_hour=?, db_max_query_seconds=?,
		   php_version=?, fastcgi_cache=?, client_max_body_mb=?, nginx_extra_directives=?, waf_enabled=?, waf_mode=?, waf_paranoia=?, is_default=?
		 WHERE id=?`,
		p.Name, p.Description, p.DiskQuotaMB, p.TrafficQuotaMB,
		p.MaxDomain, p.MaxDB, p.MaxEmail, p.MaxFTP,
		p.CPUPercent, p.RAMMB, p.MaxProcess, p.InodeQuota, p.IOWeight, p.MySQLMaxConnections,
		p.PMMaxChildren, p.IOReadMBps, p.IOWriteMBps, p.IOReadIOPS, p.IOWriteIOPS,
		p.DBMaxQueriesPerHour, p.DBMaxUpdatesPerHour, p.DBMaxQuerySeconds,
		p.PHPVersion, b01(p.FastCGICache), p.ClientMaxBodyMB, p.NginxExtraDirectives, b01(p.WAFEnabled), p.WAFMode, p.WAFParanoia, v, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE id=?", id)
	// WAF plan default may have changed — re-render vhosts for domains in this plan
	// whose WAF override is set to inherit. Runs in the background.
	go h.wafPlanReapply(id)

	saved, _ := scan(row)
	httpx.WriteJSON(w, http.StatusOK, saved)
}

// wafPlanReapply re-applies the WAF settings (including plan-default inheritors)
// for all domains in this plan. Runs in a background goroutine.
func (h *Handlers) wafPlanReapply(planID int64) {
	rows, err := h.DB.Query(`SELECT id FROM domains WHERE plan_id=?`, planID)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var did int64
		if rows.Scan(&did) == nil {
			ids = append(ids, did)
		}
	}
	_ = rows.Close()
	for _, did := range ids {
		if err := provisioner.WAFApply(h.DB, did); err != nil {
			log.Printf("waf plan reapply domain=%d: %v", did, err)
		}
	}
}

// Delete deletes an unused service plan.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var n int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM domains WHERE plan_id=?`, id).Scan(&n); err == nil && n > 0 {
		httpx.WriteError(w, http.StatusConflict,
			"this plan cannot be deleted because it is used by "+strconv.Itoa(n)+" subscriptions")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM service_plans WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// SearchDomains returns domains assigned to a plan.
func (h *Handlers) SearchDomains(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, domain_name, system_user, status, DATE_FORMAT(created_at,'%Y-%m-%d')
		 FROM domains WHERE plan_id=? ORDER BY id`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	defer func() { _ = rows.Close() }()
	type dom struct {
		ID         int64  `json:"id"`
		DomainName string `json:"domain_name"`
		SK         string `json:"system_user"`
		Status     string `json:"status"`
		CreatedAt  string `json:"created_at"`
	}
	out := make([]dom, 0)
	for rows.Next() {
		var d dom
		if err := rows.Scan(&d.ID, &d.DomainName, &d.SK, &d.Status, &d.CreatedAt); err == nil {
			out = append(out, d)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type seedTier struct {
	Name, Description                                string
	Disk, Traffic, MaxDomain, MaxDB, MaxMail, MaxFTP int
	CPU, RAM, Process, Inode, IO, MySQL, PMMax       int
	Default                                          int
}

func seedPlans() []seedTier {
	return []seedTier{
		{"Starter", "One site for a small project", 1024, 5120, 1, 1, 5, 2,
			50, 256, 30, 25000, 100, 15, 4, 1},
		{"Standard", "Multiple projects and email", 10240, 51200, 5, 10, 25, 10,
			100, 512, 60, 100000, 100, 30, 8, 0},
		{"Professional", "High traffic and large sites", 51200, 204800, 25, 50, 100, 50,
			200, 2048, 150, 500000, 200, 100, 32, 0},
	}
}

// SeedIfEmpty creates three default plans with resource limits when none exist.
func SeedIfEmpty(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM service_plans`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	log.Printf("seed: adding 3 default plans")
	for _, p := range seedPlans() {
		_, err := db.ExecContext(ctx,
			`INSERT INTO service_plans(name, description, disk_quota_mb, traffic_quota_mb,
			   max_domain, max_db, max_email, max_ftp,
			   cpu_percent, ram_mb, max_process, inode_quota, io_weight, mysql_max_connections,
			   pm_max_children, is_default)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.Name, p.Description, p.Disk, p.Traffic, p.MaxDomain, p.MaxDB, p.MaxMail, p.MaxFTP,
			p.CPU, p.RAM, p.Process, p.Inode, p.IO, p.MySQL, p.PMMax, p.Default)
		if err != nil {
			log.Printf("seed plan %s: %v", p.Name, err)
		}
	}
	return nil
}

// SeedSync inserts missing standard plans without modifying existing plans.
func SeedSync(ctx context.Context, db *sql.DB) error {
	for _, p := range seedPlans() {
		_, err := db.ExecContext(ctx,
			`INSERT INTO service_plans(name, description, disk_quota_mb, traffic_quota_mb,
			   max_domain, max_db, max_email, max_ftp,
			   cpu_percent, ram_mb, max_process, inode_quota, io_weight, mysql_max_connections,
			   pm_max_children, is_default)
			 SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0
			 FROM DUAL
			 WHERE NOT EXISTS (SELECT 1 FROM service_plans WHERE name=?)`,
			p.Name, p.Description, p.Disk, p.Traffic, p.MaxDomain, p.MaxDB, p.MaxMail, p.MaxFTP,
			p.CPU, p.RAM, p.Process, p.Inode, p.IO, p.MySQL, p.PMMax, p.Name)
		if err != nil {
			log.Printf("seed sync plan %s: %v", p.Name, err)
		}
	}
	return nil
}
