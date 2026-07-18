package domains

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os/user"
	"strconv"
	"strings"

	"servika/internal/credentials"
	"servika/internal/dns"
	"servika/internal/httpx"
	"servika/internal/provisioner"
	"servika/internal/quota"
	"servika/internal/redis"

	"github.com/go-chi/chi/v5"
)

type Domain struct {
	ID         int64  `json:"id"`
	DomainName string `json:"domain_name"`
	PHPVersion string `json:"php_version"`
	SSL        bool   `json:"ssl"`
	SSLExpiry  string `json:"ssl_expiry,omitempty"`
	Status     string `json:"status"`
	SystemUser string `json:"system_user"`
	SizeKB     int64  `json:"size_kb"`
	TrafficKB  int64  `json:"traffic_kb"`
	CreatedAt  string `json:"created_at"`
	IPv4       string `json:"ipv4"`
	FTPHost    string `json:"ftp_host"`
	FTPUser    string `json:"ftp_user"`
	DBHost     string `json:"db_host"`
	DBUser     string `json:"db_user"`
	DBName     string `json:"db_name"`
	WebRoot    string `json:"web_root"`
	IsDemo     bool   `json:"is_demo"`
	Notes      string `json:"notes,omitempty"`
	PlanID     *int64 `json:"plan_id,omitempty"`
	PlanName   string `json:"plan_name,omitempty"`
	SshAccess  bool   `json:"ssh_access"`
	Suspended  bool   `json:"suspended"`
}

type Handlers struct {
	DB   *sql.DB
	IPv4 string
}

const selectAll = `SELECT d.id, d.domain_name, d.system_user, d.php_version, d.ssl_enabled,
  COALESCE(DATE_FORMAT(d.ssl_expiry,'%Y-%m-%d'),''), d.status, d.ipv4, d.ftp_host, d.ftp_user,
  d.db_host, d.db_user, d.db_name, d.web_root, d.size_kb, d.traffic_kb, d.is_demo,
  COALESCE(d.notes,''), DATE_FORMAT(d.created_at,'%Y-%m-%d'),
  d.plan_id, COALESCE(p.name,''), d.ssh_access, COALESCE(d.suspended,0)
  FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id`

func scan(rs interface{ Scan(...any) error }) (Domain, error) {
	var d Domain
	var ssl, demo, sshE, suspended int
	var planID sql.NullInt64
	err := rs.Scan(&d.ID, &d.DomainName, &d.SystemUser, &d.PHPVersion, &ssl,
		&d.SSLExpiry, &d.Status, &d.IPv4, &d.FTPHost, &d.FTPUser,
		&d.DBHost, &d.DBUser, &d.DBName, &d.WebRoot, &d.SizeKB, &d.TrafficKB, &demo,
		&d.Notes, &d.CreatedAt,
		&planID, &d.PlanName, &sshE, &suspended)
	d.SSL = ssl == 1
	d.IsDemo = demo == 1
	d.SshAccess = sshE == 1
	d.Suspended = suspended == 1
	if planID.Valid {
		v := planID.Int64
		d.PlanID = &v
	}
	return d, err
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), selectAll+" ORDER BY d.id DESC")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database operation failed")
		return
	}
	defer rows.Close()
	out := make([]Domain, 0)
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Database read failed")
			return
		}
		out = append(out, d)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE d.id=?", id)
	d, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "Domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database read failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

type createReq struct {
	DomainName string `json:"domain_name"`
	PHPVersion string `json:"php_version"`
	CustomerID *int64 `json:"customer_id,omitempty"`
	PlanID     *int64 `json:"plan_id,omitempty"`
}

type createResp struct {
	Domain
	CreatedPasswords struct {
		FTP string `json:"ftp"`
		DB  string `json:"db"`
	} `json:"created_passwords"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.DomainName = strings.ToLower(strings.TrimSpace(req.DomainName))
	if req.PHPVersion == "" {
		req.PHPVersion = "8.3"
		// If a plan is selected, inherit the PHP version from the plan
		if req.PlanID != nil {
			var pv string
			if e := h.DB.QueryRowContext(r.Context(), `SELECT php_version FROM service_plans WHERE id=?`, *req.PlanID).Scan(&pv); e == nil && strings.TrimSpace(pv) != "" {
				req.PHPVersion = pv
			}
		}
	}
	if err := provisioner.ValidateDomain(req.DomainName); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid domain name")
		return
	}

	var existing int64
	err := h.DB.QueryRowContext(r.Context(), `SELECT id FROM domains WHERE domain_name=?`, req.DomainName).Scan(&existing)
	if err == nil {
		httpx.WriteError(w, http.StatusConflict, "This domain name is already registered")
		return
	}

	// 1) Linux user + nginx + PHP pool
	if err := quota.CheckDomainAllowed(r.Context(), h.DB, nil); err != nil {
		httpx.WriteError(w, http.StatusForbidden, "Plan limit exceeded")
		return
	}
	pr, err := provisioner.Provision(req.DomainName, req.PHPVersion)
	if err != nil {
		log.Printf("provision %q failed: %v", req.DomainName, err)
		httpx.WriteError(w, http.StatusInternalServerError, "Domain provisioning failed")
		return
	}

	dbUser := pr.SystemUser + "_db"
	dbName := pr.SystemUser + "_main"

	// 2) domains row
	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO domains(domain_name, system_user, php_version, ssl_enabled, status, ipv4,
		   ftp_host, ftp_user, db_host, db_user, db_name, web_root, is_demo)
		 VALUES(?,?,?,0,'active',?,?,?, 'localhost',?,?,?, 0)`,
		req.DomainName, pr.SystemUser, req.PHPVersion, h.IPv4,
		h.IPv4, pr.SystemUser, dbUser, dbName, pr.WebRoot)
	if err != nil {
		_ = provisioner.Deprovision(req.DomainName, pr.SystemUser)
		httpx.WriteError(w, http.StatusInternalServerError, "Domain record creation failed")
		return
	}
	id, _ := res.LastInsertId()

	if req.CustomerID != nil || req.PlanID != nil {
		_, _ = h.DB.ExecContext(r.Context(),
			`UPDATE domains SET customer_id=?, plan_id=? WHERE id=?`,
			req.CustomerID, req.PlanID, id)
	}
	// If a plan is selected, seed the nginx web-server defaults to the domain + refresh vhost
	if req.PlanID != nil {
		h.applyPlanNginxDefaults(r.Context(), id, *req.PlanID, pr.SystemUser, req.PHPVersion)
	}

	// 3) FTP account with a random password.
	ftpPass := credentials.RandomPassword(20)
	uidN, gidN := uidGidOf(pr.SystemUser)
	if err := credentials.FTPCreate(h.DB, id, pr.SystemUser, ftpPass, uidN, gidN); err != nil {
		log.Printf("FTP create %q error: %v", pr.SystemUser, err)
	}

	// 4) Default MySQL database + user
	dbPass := credentials.RandomPassword(24)
	if err := credentials.MySQLCreateDB(h.DB, id, dbName, dbUser, dbPass); err != nil {
		log.Printf("MySQL create %q error: %v", dbName, err)
	}

	// 5) Auto-seed the DNS template + write BIND zone + reload
	if _, err := dns.SeedDefaults(r.Context(), h.DB, id, req.DomainName, h.IPv4); err != nil {
		log.Printf("DNS SeedDefaults %q error: %v", req.DomainName, err)
	}
	if err := dns.WriteZone(r.Context(), h.DB, id); err != nil {
		log.Printf("DNS WriteZone %q error: %v", req.DomainName, err)
	}

	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE d.id=?", id)
	d, _ := scan(row)

	resp := createResp{Domain: d}
	resp.CreatedPasswords.FTP = ftpPass
	resp.CreatedPasswords.DB = dbPass
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

// Delete removes a domain and its panel-managed resources.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName, sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, is_demo FROM domains WHERE id=?`, id).
		Scan(&domainName, &sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "Domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database read failed")
		return
	}

	if isDemo == 0 {
		// Remove the real DBs in MariaDB (CASCADE FK only deletes the panel DB metadata)
		_ = credentials.MySQLDropAllForDomain(h.DB, id)
		// nginx vhost + PHP pool + Linux user
		if err := provisioner.Deprovision(domainName, sk); err != nil {
			log.Printf("deprovision warn (%s): %v", domainName, err)
		}
		// Redis tenant cache: Valkey ACL user + WP drop-in + domain_redis row.
		// Since domain_redis has no CASCADE FK, the row was orphaned when the domain was deleted.
		redis.CloseDomain(h.DB, id, sk)
		// NOTE: Preserve /var/backups/servika/<sk>/ intentionally.
		// The customer may have deleted the domain by accident, so backups are kept for recovery.
		// (backups.RemoveDomainBackups is available for manual cleanup.)
	}

	// Existing installations may not have foreign keys on the traffic tables.
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM domain_traffic WHERE domain_id=?`, id); err != nil {
		log.Printf("domain traffic cleanup warn (%d): %v", id, err)
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM domain_traffic_cursor WHERE domain_id=?`, id); err != nil {
		log.Printf("domain traffic cursor cleanup warn (%d): %v", id, err)
	}

	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM domains WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Domain deletion failed")
		return
	}

	// BIND zone cleanup AFTER the DELETE: updateZoneIncludes regenerates zones.conf from the domains
	// table; if the domain were still in the table (old order) the last deleted
	// domain zone include would be rewritten (dangling, named reload error).
	if isDemo == 0 {
		if err := dns.DeleteZone(r.Context(), h.DB, domainName); err != nil {
			log.Printf("DNS DeleteZone warn (%s): %v", domainName, err)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": map[string]string{"domain_name": domainName, "system_user": sk},
	})
}

func uidGidOf(u string) (int, int) {
	uu, err := user.Lookup(u)
	if err != nil {
		return 0, 0
	}
	uid, _ := strconv.Atoi(uu.Uid)
	gid, _ := strconv.Atoi(uu.Gid)
	return uid, gid
}

type setPHPReq struct {
	PHPVersion string `json:"php_version"`
}

func (h *Handlers) SetPHP(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setPHPReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.PHPVersion == "" {
		httpx.WriteError(w, http.StatusBadRequest, "php_version is required")
		return
	}
	var domainName, sk, backend, certPath, keyPath, sslSource string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, is_demo, COALESCE(web_backend,'php-fpm'), COALESCE(cert_path,''), COALESCE(key_path,''), COALESCE(ssl_source,'') FROM domains WHERE id=?`, id).
		Scan(&domainName, &sk, &isDemo, &backend, &certPath, &keyPath, &sslSource)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "Domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database read failed")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "PHP versions cannot be changed for demo subscriptions")
		return
	}
	socket, err := provisioner.SetPHPVersion(domainName, sk, req.PHPVersion, certPath, keyPath, sslSource, backend)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "PHP version change failed")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET php_version=? WHERE id=?`, req.PHPVersion, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "php_version": req.PHPVersion, "socket": socket,
	})
}

// Web backend selector: "php-fpm" | "apache" | "static"
type setBackendReq struct {
	Backend string `json:"backend"`
}

var validBackends = map[string]bool{"php-fpm": true, "apache": true, "static": true}

func (h *Handlers) GetWebBackend(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var backend string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(web_backend,'php-fpm') FROM domains WHERE id=?`, id).Scan(&backend)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "Domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database read failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"backend":   backend,
		"available": []string{"php-fpm", "apache", "static"},
	})
}

func (h *Handlers) SetWebBackend(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setBackendReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !validBackends[req.Backend] {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid backend (php-fpm|apache|static)")
		return
	}
	var domainName, sk, phpVersion string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, php_version, is_demo FROM domains WHERE id=?`, id).
		Scan(&domainName, &sk, &phpVersion, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "Domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database read failed")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "The backend cannot be changed for demo subscriptions")
		return
	}
	_ = domainName
	// 1) update DB
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET web_backend=? WHERE id=?`, req.Backend, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database update failed")
		return
	}
	// 2) Reapply the vhost (nginx + apache manager read web_backend from the DB)
	socket, _ := provisioner.PHPSocketFor(sk, phpVersion)
	if err := provisioner.ApplyVhostForDomain(h.DB, id, socket, phpVersion); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Virtual host update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "backend": req.Backend,
	})
}

// setFTPPwReq contains a replacement FTP password.
type setFTPPwReq struct {
	Password string `json:"password"`
}

func (h *Handlers) SetFTPPassword(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setFTPPwReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Password == "" {
		req.Password = credentials.RandomPassword(20)
	}
	if !credentials.ValidPassword(req.Password) {
		httpx.WriteError(w, http.StatusBadRequest, "Password contains invalid characters")
		return
	}
	var sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, is_demo FROM domains WHERE id=?`, id).
		Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "Domain not found")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "FTP passwords cannot be changed for demo subscriptions")
		return
	}
	if err := credentials.FTPUpdatePassword(h.DB, sk, req.Password); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "FTP password update failed")
		return
	}
	// If SSH is enabled, sync the system (SSH) password with FTP too
	var sshOn int
	_ = h.DB.QueryRowContext(r.Context(), `SELECT ssh_access FROM domains WHERE id=?`, id).Scan(&sshOn)
	if sshOn == 1 {
		_ = credentials.SyncSSHPassword(h.DB, sk)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "username": sk, "password": req.Password,
	})
}

// DBAccount describes a database account belonging to a domain.
type DBAccount struct {
	ID        int64  `json:"id"`
	DomainID  int64  `json:"domain_id"`
	DBName    string `json:"db_name"`
	DBUser    string `json:"db_user"`
	DBHost    string `json:"db_host"`
	DBPass    string `json:"db_pass"`
	CreatedAt string `json:"created_at"`
}

func (h *Handlers) ListDatabases(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, domain_id, db_name, db_user, db_host, db_pass_plain, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		 FROM db_accounts WHERE domain_id=? ORDER BY id`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()
	out := make([]DBAccount, 0)
	for rows.Next() {
		var d DBAccount
		if err := rows.Scan(&d.ID, &d.DomainID, &d.DBName, &d.DBUser, &d.DBHost, &d.DBPass, &d.CreatedAt); err != nil {
			continue
		}
		out = append(out, d)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type createDBReq struct {
	DBName string `json:"db_name"`
	DBUser string `json:"db_user"`
}

func (h *Handlers) CreateDatabase(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req createDBReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	var sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, is_demo FROM domains WHERE id=?`, id).
		Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "Domain not found")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "Databases cannot be added to demo subscriptions")
		return
	}
	if err := quota.CheckDatabaseAllowed(r.Context(), h.DB, id); err != nil {
		httpx.WriteError(w, http.StatusForbidden, "Plan limit exceeded")
		return
	}
	if req.DBName == "" {
		req.DBName = sk + "_db" + strconv.FormatInt(id, 10)
	}
	if req.DBUser == "" {
		req.DBUser = req.DBName
	}
	if !credentials.ValidCustomerDBIdentifier(sk, req.DBName) ||
		!credentials.ValidCustomerDBIdentifier(sk, req.DBUser) {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid database name or user; both must use the domain user prefix")
		return
	}
	pass := credentials.RandomPassword(24)
	if err := credentials.MySQLCreateDB(h.DB, id, req.DBName, req.DBUser, pass); err != nil {
		if errors.Is(err, credentials.ErrInvalidMySQLCredentials) {
			httpx.WriteError(w, http.StatusBadRequest, "Invalid database name or user")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "Database creation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "domain_id": id, "db_name": req.DBName, "db_user": req.DBUser, "db_pass": pass,
	})
}

func (h *Handlers) DeleteDatabase(w http.ResponseWriter, r *http.Request) {
	dbid, _ := strconv.ParseInt(chi.URLParam(r, "dbid"), 10, 64)
	var dbName, dbUser string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db.db_name, db.db_user, d.is_demo
		 FROM db_accounts db JOIN domains d ON d.id=db.domain_id
		 WHERE db.id=?`, dbid).Scan(&dbName, &dbUser, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "Database record not found")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "Databases cannot be deleted from demo subscriptions")
		return
	}
	if err := credentials.MySQLDropDB(h.DB, dbName, dbUser); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Database deletion failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": dbName})
}

// BulkOwner updates customer_id for multiple domains.
type bulkOwnerReq struct {
	IDs        []int64 `json:"ids"`
	CustomerID *int64  `json:"customer_id"`
}

func (h *Handlers) BulkOwner(w http.ResponseWriter, r *http.Request) {
	var req bulkOwnerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "At least one domain ID is required")
		return
	}
	// customer_id may be NULL or a positive value.
	if req.CustomerID != nil && *req.CustomerID > 0 {
		var exists int
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM customers WHERE id=?`, *req.CustomerID).Scan(&exists)
		if exists == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "Customer not found")
			return
		}
	}
	// Build placeholders for the IN clause.
	placeholders := make([]string, len(req.IDs))
	args := []any{}
	if req.CustomerID != nil && *req.CustomerID > 0 {
		args = append(args, *req.CustomerID)
	} else {
		args = append(args, nil)
	}
	for i, id := range req.IDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	sql := `UPDATE domains SET customer_id=? WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	res, err := h.DB.ExecContext(r.Context(), sql, args...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Bulk update failed")
		return
	}
	n, _ := res.RowsAffected()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": n})
}

// BulkStatus toggles multiple domains between active and passive states.
type bulkStatusReq struct {
	IDs    []int64 `json:"ids"`
	Status string  `json:"status"` // "active" | "passive"
}

func (h *Handlers) BulkStatus(w http.ResponseWriter, r *http.Request) {
	var req bulkStatusReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "At least one domain ID is required")
		return
	}
	if req.Status != "active" && req.Status != "passive" {
		httpx.WriteError(w, http.StatusBadRequest, "Status must be active or passive")
		return
	}
	placeholders := make([]string, len(req.IDs))
	args := []any{req.Status}
	for i, id := range req.IDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	sql := `UPDATE domains SET status=? WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	res, err := h.DB.ExecContext(r.Context(), sql, args...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Bulk update failed")
		return
	}
	n, _ := res.RowsAffected()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": n})
}

// applyPlanNginxDefaults writes the plan nginx
// defaults (FastCGI cache + client_max_body + extra directives) to the domain
// nginx_settings row when a new domain is attached to a plan, and re-renders the vhost with these settings.
// Best-effort: on error the domain still remains created (only logged).
func (h *Handlers) applyPlanNginxDefaults(ctx context.Context, domainID, planID int64, sk, php string) {
	var fastCGICache, clientMaxBodyMB int
	var planDirectives string
	if err := h.DB.QueryRowContext(ctx,
		`SELECT fastcgi_cache, client_max_body_mb, COALESCE(nginx_extra_directives,'')
		   FROM service_plans WHERE id=?`, planID).Scan(&fastCGICache, &clientMaxBodyMB, &planDirectives); err != nil {
		log.Printf("read plan nginx defaults (plan=%d): %v", planID, err)
		return
	}
	extraDirectives := ""
	if clientMaxBodyMB > 0 {
		extraDirectives = "client_max_body_size " + strconv.Itoa(clientMaxBodyMB) + "m;\n"
	}
	if strings.TrimSpace(planDirectives) != "" {
		extraDirectives += planDirectives
	}
	if _, err := h.DB.ExecContext(ctx,
		`INSERT INTO nginx_settings(domain_id, fastcgi_cache, extra_directives)
		 VALUES(?,?,?)
		 ON DUPLICATE KEY UPDATE fastcgi_cache=VALUES(fastcgi_cache), extra_directives=VALUES(extra_directives)`,
		domainID, fastCGICache, extraDirectives); err != nil {
		log.Printf("seed nginx_settings (domain=%d): %v", domainID, err)
		return
	}
	socket, err := provisioner.PHPSocketFor(sk, php)
	if err != nil {
		log.Printf("php socket (domain=%d): %v", domainID, err)
		return
	}
	if err := provisioner.ApplyVhostForDomain(h.DB, domainID, socket, php); err != nil {
		log.Printf("rerender plan virtual host (domain=%d): %v", domainID, err)
	}
}
