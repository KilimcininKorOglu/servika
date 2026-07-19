// Package resource reports per-domain resource usage and plan limits.
package resource

import (
	"context"
	"database/sql"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Limit struct {
	Usage int64 `json:"usage"`
	Limit int64 `json:"limit"` // Zero means unlimited.
}

type Summary struct {
	DomainName string `json:"domain_name"`
	SystemUser string `json:"system_user"`
	PlanName   string `json:"plan_name"`
	PHPVersion string `json:"php_version"`
	IPv4       string `json:"ipv4"`
	SSLEnabled bool   `json:"ssl_enabled"`
	SSLExpiry  string `json:"ssl_expiry,omitempty"`

	// Metrics with plan limits, expressed as usage and limit.
	DiskMB      Limit `json:"disk_mb"`      // Limit comes from disk_quota_mb.
	TrafficMB   Limit `json:"traffic_mb"`   // Limit comes from traffic_quota_mb.
	DBCount     Limit `json:"db_count"`     // Limit comes from max_db.
	FTPCount    Limit `json:"ftp_count"`    // Limit comes from max_ftp.
	EmailCount  Limit `json:"email_count"`  // Limit comes from max_email.
	DomainCount Limit `json:"domain_count"` // Limit comes from max_domain and includes subdomains.

	// Additional counters without plan limits.
	DNSRecordCount int64 `json:"dns_record"`
	CronJobCount   int64 `json:"cron_job"`
	BackupCount    int64 `json:"backup_count"`
	BackupMB       int64 `json:"backup_mb"`
}

type Handlers struct {
	DB *sql.DB
}

// duMB returns the home directory's disk usage in megabytes.
func duMB(home string) int64 {
	out, err := exec.Command("du", "-sm", home).CombinedOutput()
	if err != nil {
		return 0
	}
	parts := strings.Fields(string(out))
	if len(parts) == 0 {
		return 0
	}
	n, _ := strconv.ParseInt(parts[0], 10, 64)
	return n
}

// dbTotalMB returns the total database size for panel-managed users in megabytes.
func dbTotalMB(ctx context.Context, db *sql.DB, dbUsers []string) int64 {
	if len(dbUsers) == 0 {
		return 0
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(dbUsers)), ",")
	args := make([]any, len(dbUsers))
	for i, user := range dbUsers {
		args[i] = user
	}
	// Sum data and index sizes from information_schema.
	query := `SELECT COALESCE(SUM((data_length+index_length))/1024/1024, 0)
	      FROM information_schema.tables
	      WHERE table_schema IN (
	          SELECT db_name FROM panel.db_accounts WHERE db_user IN (` + placeholders + `)
	      )`
	var mb float64
	_ = db.QueryRowContext(ctx, query, args...).Scan(&mb)
	return int64(mb)
}

func (h *Handlers) Show(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	ctx := r.Context()

	var o Summary
	var planID sql.NullInt64
	var sslExpiry sql.NullString
	var sslEnabled int
	err := h.DB.QueryRowContext(ctx,
		`SELECT d.domain_name, d.system_user, d.php_version, d.ipv4, d.ssl_enabled,
		        DATE_FORMAT(d.ssl_expiry,'%Y-%m-%d'), d.plan_id
		 FROM domains d WHERE d.id=?`, id).
		Scan(&o.DomainName, &o.SystemUser, &o.PHPVersion, &o.IPv4, &sslEnabled, &sslExpiry, &planID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	o.SSLEnabled = sslEnabled == 1
	if sslExpiry.Valid {
		o.SSLExpiry = sslExpiry.String
	}

	// Load plan limits.
	var planName sql.NullString
	var diskQuota, trafficQuota, maxDomain, maxDB, maxEmail, maxFTP int64
	if planID.Valid {
		_ = h.DB.QueryRowContext(ctx,
			`SELECT name, disk_quota_mb, traffic_quota_mb, max_domain, max_db, max_email, max_ftp
			 FROM service_plans WHERE id=?`, planID.Int64).
			Scan(&planName, &diskQuota, &trafficQuota, &maxDomain, &maxDB, &maxEmail, &maxFTP)
	}
	if planName.Valid {
		o.PlanName = planName.String
	} else {
		o.PlanName = "Unlimited (no plan assigned)"
	}

	// Calculate disk usage.
	home := "/home/" + o.SystemUser
	o.DiskMB.Usage = duMB(home)
	_, _ = h.DB.ExecContext(ctx, `UPDATE domains SET size_kb=? WHERE id=?`, o.DiskMB.Usage*1024, id)
	o.DiskMB.Limit = diskQuota

	// Convert domains.traffic_kb from kilobytes to megabytes.
	var trafficKB int64
	_ = h.DB.QueryRowContext(ctx, `SELECT traffic_kb FROM domains WHERE id=?`, id).Scan(&trafficKB)
	o.TrafficMB.Usage = trafficKB / 1024
	o.TrafficMB.Limit = trafficQuota

	// Count databases and collect users for size calculation.
	rows, err := h.DB.QueryContext(ctx, `SELECT db_user FROM db_accounts WHERE domain_id=?`, id)
	dbUsers := []string{}
	if err == nil {
		for rows.Next() {
			var u string
			if rows.Scan(&u) == nil {
				dbUsers = append(dbUsers, u)
			}
		}
		_ = rows.Close()
	}
	o.DBCount.Usage = int64(len(dbUsers))
	o.DBCount.Limit = maxDB

	// Count FTP accounts.
	_ = h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM ftp_accounts WHERE domain_id=?`, id).Scan(&o.FTPCount.Usage)
	o.FTPCount.Limit = maxFTP

	// Report zero email accounts until the email module is available.
	o.EmailCount.Usage = 0
	o.EmailCount.Limit = maxEmail

	// The primary domain counts as one. Subdomains should eventually be included in the subscription count.
	// For now, this follows the subscription model of one primary domain and zero subdomains.
	o.DomainCount.Usage = 1
	o.DomainCount.Limit = maxDomain

	// Count DNS records.
	_ = h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM dns_records WHERE domain_id=?`, id).Scan(&o.DNSRecordCount)

	// Count jobs in the user's host crontab.
	if out, err := exec.Command("crontab", "-u", o.SystemUser, "-l").CombinedOutput(); err == nil {
		for _, ln := range strings.Split(string(out), "\n") {
			s := strings.TrimSpace(ln)
			if s != "" && !strings.HasPrefix(s, "#") {
				o.CronJobCount++
			}
		}
	}

	// Count backups and calculate their total size.
	_ = h.DB.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(size_b),0) FROM backups WHERE domain_id=?`, id).
		Scan(&o.BackupCount, &o.BackupMB)
	o.BackupMB = o.BackupMB / (1024 * 1024) // Convert bytes to megabytes.

	// Keep database usage separate from DiskMB.Usage because databases may reside on another disk.
	// DiskMB currently measures only the home directory.
	_ = dbTotalMB // future use

	httpx.WriteJSON(w, http.StatusOK, o)
}
