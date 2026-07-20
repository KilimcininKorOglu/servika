package provisioner

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	tenantUnitDir = "/etc/systemd/system"
	tenantCfgRoot = "/etc/php-fpm-tenant"
	tenantLogDir  = "/var/log/php-fpm"
)

func tenantUnitName(systemUser string) string {
	return "php-fpm-" + systemUser + ".service"
}

func tenantUnitPath(systemUser string) string {
	return filepath.Join(tenantUnitDir, tenantUnitName(systemUser))
}

func tenantRunDir(systemUser string) string {
	return "/run/php-fpm-" + systemUser
}

func tenantSocket(systemUser string) string {
	return filepath.Join(tenantRunDir(systemUser), systemUser+".sock")
}

func tenantCfgDir(systemUser string) string {
	return filepath.Join(tenantCfgRoot, systemUser)
}

const fpmSocketFcontextSpec = "/run/php-fpm-[^/]+(/.*)?"

var (
	fcontextMu       sync.Mutex
	fcontextDone     bool
	httpdBooleanMu   sync.Mutex
	httpdBooleanDone bool
)

// ensureFPMSELinuxFcontext persistently labels per-tenant socket paths for nginx access.
func ensureFPMSELinuxFcontext() {
	fcontextMu.Lock()
	defer fcontextMu.Unlock()
	if fcontextDone {
		return
	}
	if !selinuxActive() {
		fcontextDone = true
		return
	}
	if _, err := exec.LookPath("semanage"); err != nil {
		fcontextDone = true
		return
	}

	output, _ := tenantCommand("semanage", "fcontext", "-l").CombinedOutput()
	if strings.Contains(string(output), "/run/php-fpm-[") {
		fcontextDone = true
		return
	}
	if _, err := tenantCommand("semanage", "fcontext", "-a", "-t", "httpd_var_run_t", fpmSocketFcontextSpec).CombinedOutput(); err == nil {
		fcontextDone = true
	}
}

func selinuxActive() bool {
	output, err := tenantCommand("getenforce").Output()
	if err != nil {
		return false
	}
	status := strings.TrimSpace(string(output))
	return status == "Enforcing" || status == "Permissive"
}

// ensureHTTPDHomeBooleans persistently permits web servers to read tenant home content.
func ensureHTTPDHomeBooleans() {
	httpdBooleanMu.Lock()
	defer httpdBooleanMu.Unlock()
	if httpdBooleanDone {
		return
	}
	if !selinuxActive() {
		httpdBooleanDone = true
		return
	}
	if _, err := exec.LookPath("setsebool"); err != nil {
		httpdBooleanDone = true
		return
	}

	required := []string{"httpd_enable_homedirs", "httpd_read_user_content"}
	disabled := make([]string, 0, len(required))
	for _, boolean := range required {
		output, err := tenantCommand("getsebool", boolean).Output()
		if err != nil {
			continue
		}
		if !strings.Contains(string(output), "--> on") {
			disabled = append(disabled, boolean)
		}
	}
	if len(disabled) == 0 {
		httpdBooleanDone = true
		return
	}

	args := []string{"-P"}
	for _, boolean := range disabled {
		args = append(args, boolean+"=on")
	}
	output, err := tenantCommand("setsebool", args...).CombinedOutput()
	if err != nil {
		log.Printf("SELinux HTTP home boolean update failed: %s: %v", strings.TrimSpace(string(output)), err)
		return
	}
	httpdBooleanDone = true
	log.Printf("SELinux: enabled HTTP home access booleans: %v", disabled)
}

// TenantFPMActive reports whether a tenant PHP-FPM unit is installed.
func TenantFPMActive(systemUser string) bool {
	if !tenantUserPattern.MatchString(systemUser) {
		return false
	}
	_, err := os.Stat(tenantUnitPath(systemUser))
	return err == nil
}

func tenantSanitizeScalar(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fallback
	}
	return value
}

func resolveTenantPMMaxChildren(pmMaxChildren, ramMB int) int {
	if pmMaxChildren > 0 {
		return pmMaxChildren
	}
	if ramMB > 0 {
		return max(4, ramMB/64)
	}
	return 8
}

func tenantPMMaxChildren(db *sql.DB, domainID int64) int {
	var pmMaxChildren, ramMB int
	if db != nil && domainID > 0 {
		_ = db.QueryRow(`SELECT COALESCE(p.pm_max_children,0), COALESCE(p.ram_mb,0)
			FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id
			WHERE d.id=?`, domainID).Scan(&pmMaxChildren, &ramMB)
	}
	return resolveTenantPMMaxChildren(pmMaxChildren, ramMB)
}

type tenantPoolSettings struct {
	MemoryLimit       string
	MaxExecutionTime  int
	MaxInputTime      int
	PostMaxSize       string
	UploadMaxFilesize string
	DisableFunctions  string
	PMStrategy        string
	PMMaxRequests     int
}

const hardenedDisableFunctions = "exec,passthru,shell_exec,system,proc_open,popen,proc_close,proc_get_status,proc_terminate,proc_nice,pcntl_exec,dl,symlink,link,posix_kill,posix_mkfifo,posix_setpgid,posix_setsid,posix_setuid,posix_setgid"

func tenantReadPoolSettings(db *sql.DB, domainID int64) tenantPoolSettings {
	settings := tenantPoolSettings{
		MemoryLimit:       "256M",
		MaxExecutionTime:  30,
		MaxInputTime:      60,
		PostMaxSize:       "64M",
		UploadMaxFilesize: "32M",
		DisableFunctions:  hardenedDisableFunctions,
		PMStrategy:        "ondemand",
		PMMaxRequests:     500,
	}
	if db == nil || domainID <= 0 {
		return settings
	}

	var memoryLimit, postMaxSize, uploadMaxFilesize, disableFunctions, strategy string
	var maxExecutionTime, maxInputTime, maxRequests int
	err := db.QueryRow(`SELECT memory_limit, max_execution_time, max_input_time,
		post_max_size, upload_max_filesize, disable_functions, pm_strategy, pm_max_requests
		FROM php_settings WHERE domain_id=?`, domainID).
		Scan(&memoryLimit, &maxExecutionTime, &maxInputTime, &postMaxSize,
			&uploadMaxFilesize, &disableFunctions, &strategy, &maxRequests)
	if err != nil {
		return settings
	}

	settings.MemoryLimit = tenantSanitizeScalar(memoryLimit, settings.MemoryLimit)
	settings.PostMaxSize = tenantSanitizeScalar(postMaxSize, settings.PostMaxSize)
	settings.UploadMaxFilesize = tenantSanitizeScalar(uploadMaxFilesize, settings.UploadMaxFilesize)
	settings.DisableFunctions = tenantSanitizeScalar(disableFunctions, settings.DisableFunctions)
	settings.PMStrategy = tenantSanitizeScalar(strategy, settings.PMStrategy)
	if maxExecutionTime > 0 {
		settings.MaxExecutionTime = maxExecutionTime
	}
	if maxInputTime > 0 {
		settings.MaxInputTime = maxInputTime
	}
	if maxRequests > 0 {
		settings.PMMaxRequests = maxRequests
	}
	switch settings.PMStrategy {
	case "static", "dynamic", "ondemand":
	default:
		settings.PMStrategy = "ondemand"
	}
	return settings
}

func renderTenantPool(db *sql.DB, systemUser string, domainID int64) string {
	settings := tenantReadPoolSettings(db, domainID)
	maxChildren := tenantPMMaxChildren(db, domainID)
	startServers := max(1, maxChildren/4)
	maxSpareServers := max(1, maxChildren/2)

	var body strings.Builder
	fmt.Fprintf(&body, "[%s]\n", systemUser)
	fmt.Fprintf(&body, "user = %s\n", systemUser)
	fmt.Fprintf(&body, "group = %s\n", systemUser)
	fmt.Fprintf(&body, "listen = %s\n", tenantSocket(systemUser))
	body.WriteString("listen.owner = nginx\nlisten.group = nginx\nlisten.mode = 0660\n")
	fmt.Fprintf(&body, "pm = %s\n", settings.PMStrategy)
	fmt.Fprintf(&body, "pm.max_children = %d\n", maxChildren)
	switch settings.PMStrategy {
	case "dynamic":
		fmt.Fprintf(&body, "pm.start_servers = %d\n", startServers)
		body.WriteString("pm.min_spare_servers = 1\n")
		fmt.Fprintf(&body, "pm.max_spare_servers = %d\n", maxSpareServers)
	case "ondemand":
		body.WriteString("pm.process_idle_timeout = 30s\n")
	}
	fmt.Fprintf(&body, "pm.max_requests = %d\n", settings.PMMaxRequests)
	body.WriteString("; Security settings cannot be overridden by tenant code.\n")
	fmt.Fprintf(&body, "php_admin_value[open_basedir] = /home/%s/:/tmp/\n", systemUser)
	fmt.Fprintf(&body, "php_admin_value[disable_functions] = %s\n", settings.DisableFunctions)
	fmt.Fprintf(&body, "php_admin_value[upload_tmp_dir] = /home/%s/tmp\n", systemUser)
	fmt.Fprintf(&body, "php_admin_value[sys_temp_dir] = /home/%s/tmp\n", systemUser)
	fmt.Fprintf(&body, "php_admin_value[session.save_path] = /home/%s/tmp\n", systemUser)
	fmt.Fprintf(&body, "php_admin_value[memory_limit] = %s\n", settings.MemoryLimit)
	fmt.Fprintf(&body, "php_admin_value[max_execution_time] = %d\n", settings.MaxExecutionTime)
	fmt.Fprintf(&body, "php_admin_value[max_input_time] = %d\n", settings.MaxInputTime)
	fmt.Fprintf(&body, "php_admin_value[post_max_size] = %s\n", settings.PostMaxSize)
	fmt.Fprintf(&body, "php_admin_value[upload_max_filesize] = %s\n", settings.UploadMaxFilesize)
	// Explicitly enable logging and disable display_errors for production.
	// Intentionally NOT setting error_log -- PHP errors go to stderr and are
	// captured by catch_workers_output into the per-tenant error log.
	body.WriteString("php_admin_flag[log_errors] = on\n")
	body.WriteString("php_admin_flag[display_errors] = off\n")
	body.WriteString("catch_workers_output = yes\n")
	return body.String()
}

func renderTenantGlobalConfig(systemUser string) string {
	return fmt.Sprintf(`[global]
pid = %s/php-fpm.pid
error_log = %s/tenant-%s.log
log_level = warning
daemonize = no
include=%s/pool.conf
`, tenantRunDir(systemUser), tenantLogDir, systemUser, tenantCfgDir(systemUser))
}

func renderTenantUnit(systemUser, fpmBinary string) string {
	return fmt.Sprintf(`[Unit]
Description=Servika per-tenant PHP-FPM for %s
After=network.target
Before=nginx.service

[Service]
Type=notify
NotifyAccess=all
Slice=servika-%s.slice
ExecStart=%s --nodaemonize --fpm-config %s/php-fpm.conf
ExecReload=/bin/kill -USR2 $MAINPID
RuntimeDirectory=php-fpm-%s
RuntimeDirectoryMode=0755
RuntimeDirectoryPreserve=yes
ProtectHome=tmpfs
BindPaths=/home/%s
PrivateTmp=yes
ProtectSystem=strict
ReadWritePaths=/home/%s %s
ProtectProc=invisible
NoNewPrivileges=yes
RestrictNamespaces=yes
RestrictSUIDSGID=yes
ProtectKernelTunables=yes
ProtectControlGroups=yes
LimitCORE=0
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
`, systemUser, systemUser, fpmBinary, tenantCfgDir(systemUser), systemUser, systemUser, systemUser, tenantLogDir)
}

func waitForSocket(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// EnableTenantFPM moves a domain to an isolated PHP-FPM master service.
func EnableTenantFPM(db *sql.DB, domainID int64, systemUser, phpVersion string) (string, error) {
	if !tenantUserPattern.MatchString(systemUser) {
		return "", fmt.Errorf("invalid system user: %q", systemUser)
	}
	phpVersion = normalizePHP(phpVersion)
	config := phpMap[phpVersion]
	if config.FPMBin == "" {
		return "", fmt.Errorf("PHP-FPM binary is undefined for %s", phpVersion)
	}
	if _, err := os.Stat(config.FPMBin); err != nil {
		return "", fmt.Errorf("PHP-FPM binary is unavailable for %s: %w", phpVersion, err)
	}
	if _, err := os.Stat(filepath.Join("/home", systemUser)); err != nil {
		return "", fmt.Errorf("tenant home is unavailable: %w", err)
	}

	firstInstall := !TenantFPMActive(systemUser)
	configDir := tenantCfgDir(systemUser)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("create tenant configuration directory: %w", err)
	}
	if err := os.MkdirAll(tenantLogDir, 0755); err != nil {
		return "", fmt.Errorf("create PHP-FPM log directory: %w", err)
	}

	poolPath := filepath.Join(configDir, "pool.conf")
	previousPool, readErr := os.ReadFile(poolPath)
	if err := os.WriteFile(poolPath, []byte(renderTenantPool(db, systemUser, domainID)), 0644); err != nil {
		return "", fmt.Errorf("write tenant pool: %w", err)
	}
	globalPath := filepath.Join(configDir, "php-fpm.conf")
	if err := os.WriteFile(globalPath, []byte(renderTenantGlobalConfig(systemUser)), 0644); err != nil {
		return "", fmt.Errorf("write tenant global configuration: %w", err)
	}
	if output, err := tenantCommand(config.FPMBin, "-t", "-y", globalPath).CombinedOutput(); err != nil {
		if readErr == nil {
			_ = os.WriteFile(poolPath, previousPool, 0644)
		} else {
			_ = os.Remove(poolPath)
		}
		return "", fmt.Errorf("validate tenant PHP-FPM configuration: %s: %w", strings.TrimSpace(string(output)), err)
	}
	if err := os.WriteFile(tenantUnitPath(systemUser), []byte(renderTenantUnit(systemUser, config.FPMBin)), 0644); err != nil {
		return "", fmt.Errorf("write tenant service: %w", err)
	}
	if output, err := tenantCommand("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		_ = RollbackToSharedFPM(db, domainID, systemUser, phpVersion)
		return "", fmt.Errorf("reload systemd units: %s: %w", strings.TrimSpace(string(output)), err)
	}

	if firstInstall {
		sharedPool := filepath.Join(config.PoolDir, systemUser+".conf")
		if _, err := os.Stat(sharedPool); err == nil {
			if err := os.Rename(sharedPool, sharedPool+".bak"); err != nil {
				_ = RollbackToSharedFPM(db, domainID, systemUser, phpVersion)
				return "", fmt.Errorf("preserve shared PHP-FPM pool: %w", err)
			}
			_, _ = tenantCommand("systemctl", "reload-or-restart", config.Service).CombinedOutput()
		}
	}

	if output, err := tenantCommand("systemctl", "enable", tenantUnitName(systemUser)).CombinedOutput(); err != nil {
		_ = RollbackToSharedFPM(db, domainID, systemUser, phpVersion)
		return "", fmt.Errorf("enable tenant PHP-FPM: %s: %w", strings.TrimSpace(string(output)), err)
	}
	if output, err := tenantCommand("systemctl", "restart", tenantUnitName(systemUser)).CombinedOutput(); err != nil {
		_ = RollbackToSharedFPM(db, domainID, systemUser, phpVersion)
		return "", fmt.Errorf("restart tenant PHP-FPM: %s: %w", strings.TrimSpace(string(output)), err)
	}

	ensureFPMSELinuxFcontext()
	_, _ = tenantCommand("restorecon", "-R", tenantRunDir(systemUser)).CombinedOutput()
	_, _ = tenantCommand("restorecon", "-R", configDir).CombinedOutput()
	socket := tenantSocket(systemUser)
	if !waitForSocket(socket, 6*time.Second) {
		_ = RollbackToSharedFPM(db, domainID, systemUser, phpVersion)
		return "", fmt.Errorf("tenant PHP-FPM socket was not created: %s", socket)
	}
	_, _ = tenantCommand("restorecon", socket).CombinedOutput()
	if db != nil && domainID > 0 {
		if err := ApplyVhostForDomain(db, domainID, socket, phpVersion); err != nil {
			_ = RollbackToSharedFPM(db, domainID, systemUser, phpVersion)
			return "", fmt.Errorf("render tenant nginx virtual host: %w", err)
		}
	}
	return socket, nil
}

// RollbackToSharedFPM restores a domain to the shared PHP-FPM master service.
func RollbackToSharedFPM(db *sql.DB, domainID int64, systemUser, phpVersion string) error {
	if !tenantUserPattern.MatchString(systemUser) {
		return fmt.Errorf("invalid system user: %q", systemUser)
	}
	phpVersion = normalizePHP(phpVersion)
	config := phpMap[phpVersion]

	_, _ = tenantCommand("systemctl", "disable", "--now", tenantUnitName(systemUser)).CombinedOutput()
	_ = os.Remove(tenantUnitPath(systemUser))
	_, _ = tenantCommand("systemctl", "daemon-reload").CombinedOutput()

	sharedPool := filepath.Join(config.PoolDir, systemUser+".conf")
	backupPool := sharedPool + ".bak"
	var socket string
	if _, err := os.Stat(backupPool); err == nil {
		if err := os.Rename(backupPool, sharedPool); err != nil {
			return fmt.Errorf("restore shared PHP-FPM pool: %w", err)
		}
		_, _ = tenantCommand("systemctl", "reload-or-restart", config.Service).CombinedOutput()
		socket = filepath.Join(config.SockDir, systemUser+".sock")
	} else {
		var err error
		socket, _, err = writePoolValidated(systemUser, phpVersion)
		if err != nil {
			return fmt.Errorf("rebuild shared PHP-FPM pool: %w", err)
		}
	}

	_ = os.RemoveAll(tenantCfgDir(systemUser))
	_ = os.RemoveAll(tenantRunDir(systemUser))
	if db != nil && domainID > 0 {
		if err := ApplyVhostForDomain(db, domainID, socket, phpVersion); err != nil {
			return fmt.Errorf("render shared nginx virtual host: %w", err)
		}
	}
	return nil
}

// TeardownTenantFPM removes tenant service files during domain deletion.
func TeardownTenantFPM(systemUser string) {
	if !tenantUserPattern.MatchString(systemUser) {
		return
	}
	_, _ = tenantCommand("systemctl", "disable", "--now", tenantUnitName(systemUser)).CombinedOutput()
	_ = os.Remove(tenantUnitPath(systemUser))
	_, _ = tenantCommand("systemctl", "daemon-reload").CombinedOutput()
	_ = os.RemoveAll(tenantCfgDir(systemUser))
	_ = os.RemoveAll(tenantRunDir(systemUser))
	for _, config := range phpMap {
		_ = os.Remove(filepath.Join(config.PoolDir, systemUser+".conf.bak"))
	}
}

// EnsureTenantFPMOnStartup starts installed tenant services or rolls them back safely.
func EnsureTenantFPMOnStartup() {
	if packageDB == nil {
		return
	}
	rows, err := packageDB.Query(`SELECT id, system_user, php_version FROM domains`)
	if err != nil {
		log.Printf("tenant PHP-FPM startup check: %v", err)
		return
	}
	type domain struct {
		id         int64
		systemUser string
		phpVersion string
	}
	var domains []domain
	for rows.Next() {
		var item domain
		if err := rows.Scan(&item.id, &item.systemUser, &item.phpVersion); err == nil {
			domains = append(domains, item)
		}
	}
	if err := rows.Close(); err != nil {
		log.Printf("tenant PHP-FPM startup rows: %v", err)
	}
	for _, item := range domains {
		if !TenantFPMActive(item.systemUser) {
			continue
		}
		// Config-drift repair: old provisions may have left pool files with
		// unwritable error_log overrides that silently swallowed PHP fatals.
		repairTenantPoolDrift(item.id, item.systemUser, item.phpVersion)
		if output, _ := tenantCommand("systemctl", "is-active", tenantUnitName(item.systemUser)).CombinedOutput(); strings.TrimSpace(string(output)) == "active" {
			continue
		}
		if output, err := tenantCommand("systemctl", "start", tenantUnitName(item.systemUser)).CombinedOutput(); err != nil {
			log.Printf("tenant PHP-FPM startup failed for %s: %s", item.systemUser, strings.TrimSpace(string(output)))
			if rollbackErr := RollbackToSharedFPM(packageDB, item.id, item.systemUser, item.phpVersion); rollbackErr != nil {
				log.Printf("tenant PHP-FPM rollback failed for %s: %v", item.systemUser, rollbackErr)
			}
		}
	}
}

// repairTenantPoolDrift rewrites a tenant pool.conf if it has drifted from the
// current renderTenantPool template. Old provisions may have left unwritable
// php_admin_value[error_log] overrides that silently swallowed PHP fatal errors.
// Validates with php-fpm -t before committing; rolls back on failure. Graceful
// reload (USR2) avoids site downtime.
func repairTenantPoolDrift(domainID int64, systemUser, phpVersion string) {
	if packageDB == nil || systemUser == "" || !strings.HasPrefix(systemUser, "c_") {
		return
	}
	configDir := tenantCfgDir(systemUser)
	poolPath := filepath.Join(configDir, "pool.conf")
	current, err := os.ReadFile(poolPath)
	if err != nil {
		return // pool.conf missing -- EnableTenantFPM handles creation
	}
	expected := renderTenantPool(packageDB, systemUser, domainID)
	if string(current) == expected {
		return // no drift, no-op
	}
	phpVersion = normalizePHP(phpVersion)
	config := phpMap[phpVersion]
	if config.FPMBin == "" {
		return
	}
	// Write the new pool config, validate, and rollback on failure.
	if err := os.WriteFile(poolPath, []byte(expected), 0644); err != nil {
		return
	}
	globalPath := filepath.Join(configDir, "php-fpm.conf")
	if output, err := tenantCommand(config.FPMBin, "-t", "-y", globalPath).CombinedOutput(); err != nil {
		_ = os.WriteFile(poolPath, current, 0644) // rollback
		log.Printf("repairTenantPoolDrift: %s php-fpm -t failed, rolled back: %s", systemUser, strings.TrimSpace(string(output)))
		return
	}
	// Graceful reload (USR2) -- does not drop active requests.
	if output, err := tenantCommand("systemctl", "reload", tenantUnitName(systemUser)).CombinedOutput(); err != nil {
		log.Printf("repairTenantPoolDrift: %s reload warning: %s", systemUser, strings.TrimSpace(string(output)))
	}
	log.Printf("repairTenantPoolDrift: %s pool.conf updated (logging hardening + config drift repair)", systemUser)
}
