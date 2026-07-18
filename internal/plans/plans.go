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
	PHPVersion           string `json:"php_version"`
	FastCGICache         bool   `json:"fastcgi_cache"`
	ClientMaxBodyMB      int    `json:"client_max_body_mb"`
	NginxExtraDirectives string `json:"nginx_extra_directives"`
	IsDefault            bool   `json:"is_default"`
	CreatedAt            string `json:"created_at"`
}

// Handlers provides service plan HTTP handlers.
type Handlers struct {
	DB *sql.DB
}

const selectAll = `SELECT id, name, description, disk_quota_mb, traffic_quota_mb,
  max_domain, max_db, max_email, max_ftp,
  cpu_percent, ram_mb, max_process, inode_quota, io_weight, mysql_max_connections,
  php_version, fastcgi_cache, client_max_body_mb, COALESCE(nginx_extra_directives,''), is_default, DATE_FORMAT(created_at,'%Y-%m-%d') FROM service_plans`

func b01(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scan(rs interface{ Scan(...any) error }) (Plan, error) {
	var p Plan
	var vars, fc int
	err := rs.Scan(&p.ID, &p.Name, &p.Description, &p.DiskQuotaMB, &p.TrafficQuotaMB,
		&p.MaxDomain, &p.MaxDB, &p.MaxEmail, &p.MaxFTP,
		&p.CPUPercent, &p.RAMMB, &p.MaxProcess, &p.InodeQuota, &p.IOWeight, &p.MySQLMaxConnections,
		&p.PHPVersion, &fc, &p.ClientMaxBodyMB, &p.NginxExtraDirectives, &vars, &p.CreatedAt)
	p.IsDefault = vars == 1
	p.FastCGICache = fc == 1
	return p, err
}

// List returns all service plans.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), selectAll+" ORDER BY is_default DESC, id ASC")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	defer rows.Close()
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
		   php_version, fastcgi_cache, client_max_body_mb, nginx_extra_directives, is_default)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.Name, p.Description, p.DiskQuotaMB, p.TrafficQuotaMB,
		p.MaxDomain, p.MaxDB, p.MaxEmail, p.MaxFTP,
		p.CPUPercent, p.RAMMB, p.MaxProcess, p.InodeQuota, p.IOWeight, p.MySQLMaxConnections,
		p.PHPVersion, b01(p.FastCGICache), p.ClientMaxBodyMB, p.NginxExtraDirectives, v)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	id, _ := res.LastInsertId()
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE id=?", id)
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
		   php_version=?, fastcgi_cache=?, client_max_body_mb=?, nginx_extra_directives=?, is_default=?
		 WHERE id=?`,
		p.Name, p.Description, p.DiskQuotaMB, p.TrafficQuotaMB,
		p.MaxDomain, p.MaxDB, p.MaxEmail, p.MaxFTP,
		p.CPUPercent, p.RAMMB, p.MaxProcess, p.InodeQuota, p.IOWeight, p.MySQLMaxConnections,
		p.PHPVersion, b01(p.FastCGICache), p.ClientMaxBodyMB, p.NginxExtraDirectives, v, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "plan operation failed")
		return
	}
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE id=?", id)
	saved, _ := scan(row)
	httpx.WriteJSON(w, http.StatusOK, saved)
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
	defer rows.Close()
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
	rows := []struct {
		Name, Description                                string
		Disk, Traffic, MaxDomain, MaxDB, MaxMail, MaxFTP int
		CPU, RAM, Process, Inode, IO, MySQLConnections   int
		Default                                          int
	}{
		{"Starter", "One site for a small project", 1024, 5120, 1, 1, 5, 2,
			50, 256, 30, 25000, 100, 15, 1},
		{"Standard", "Multiple projects and email", 10240, 51200, 5, 10, 25, 10,
			100, 512, 60, 100000, 100, 30, 0},
		{"Professional", "High traffic and large sites", 51200, 204800, 25, 50, 100, 50,
			200, 2048, 150, 500000, 200, 100, 0},
	}
	for _, p := range rows {
		_, err := db.ExecContext(ctx,
			`INSERT INTO service_plans(name, description, disk_quota_mb, traffic_quota_mb,
			   max_domain, max_db, max_email, max_ftp,
			   cpu_percent, ram_mb, max_process, inode_quota, io_weight, mysql_max_connections,
			   is_default)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.Name, p.Description, p.Disk, p.Traffic, p.MaxDomain, p.MaxDB, p.MaxMail, p.MaxFTP,
			p.CPU, p.RAM, p.Process, p.Inode, p.IO, p.MySQLConnections, p.Default)
		if err != nil {
			log.Printf("seed plan %s: %v", p.Name, err)
		}
	}
	return nil
}
