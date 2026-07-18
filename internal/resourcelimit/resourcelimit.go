// Package resourcelimit applies per-domain cgroup, XFS quota, and MariaDB limits.
// Limits are loaded from the domain's assigned plan and enforced at the system level.
package resourcelimit

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"servika/internal/provisioner"
)

// Limits contains active resource values loaded from a service plan.
type Limits struct {
	CPUPercent          int
	RAMMB               int
	MaxProcess          int
	InodeQuota          int
	IOWeight            int
	MySQLMaxConnections int
	DiskQuotaMB         int
	PMMaxChildren       int
	IOReadMBps          int
	IOWriteMBps         int
	IOReadIOPS          int
	IOWriteIOPS         int
	DBMaxQueriesPerHour int
	DBMaxUpdatesPerHour int
	DBMaxQuerySeconds   int
}

// GetPlanLimits returns limits from the domain's assigned plan.
// An unassigned plan returns zero values, which remove enforcement.
func GetPlanLimits(ctx context.Context, db *sql.DB, domainID int64) (Limits, error) {
	var l Limits
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(p.cpu_percent,0), COALESCE(p.ram_mb,0),
		       COALESCE(p.max_process,0), COALESCE(p.inode_quota,0),
		       COALESCE(p.io_weight,100), COALESCE(p.mysql_max_connections,0),
		       COALESCE(p.disk_quota_mb,0), COALESCE(p.pm_max_children,0),
		       COALESCE(p.io_read_mbps,0), COALESCE(p.io_write_mbps,0),
		       COALESCE(p.io_read_iops,0), COALESCE(p.io_write_iops,0),
		       COALESCE(p.db_max_queries_per_hour,0), COALESCE(p.db_max_updates_per_hour,0),
		       COALESCE(p.db_max_query_seconds,0)
		FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id
		WHERE d.id=?`, domainID).
		Scan(&l.CPUPercent, &l.RAMMB, &l.MaxProcess, &l.InodeQuota,
			&l.IOWeight, &l.MySQLMaxConnections, &l.DiskQuotaMB, &l.PMMaxChildren,
			&l.IOReadMBps, &l.IOWriteMBps, &l.IOReadIOPS, &l.IOWriteIOPS,
			&l.DBMaxQueriesPerHour, &l.DBMaxUpdatesPerHour, &l.DBMaxQuerySeconds)
	return l, err
}

const (
	sliceDir     = "/etc/systemd/system"
	ioDevicePath = "/home"
)

func calculatePMMaxChildren(l Limits) int {
	if l.PMMaxChildren > 0 {
		return l.PMMaxChildren
	}
	if l.RAMMB > 0 {
		return max(4, l.RAMMB/64)
	}
	return 8
}

func resourceCommand(name string, args ...string) *exec.Cmd {
	return resourceCommandContext(context.Background(), name, args...)
}

func resourceCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C",
		"LC_ALL=C",
	}
	return command
}

func sliceName(systemUser string) string {
	// systemd slice, for example servika-c_registry_persistent_test_local.slice.
	return "servika-" + systemUser + ".slice"
}

func slicePath(systemUser string) string {
	return filepath.Join(sliceDir, sliceName(systemUser))
}

// WriteSystemdSlice writes /etc/systemd/system/servika-<system-user>.slice with cgroup v2
// CPUQuota, MemoryMax, TasksMax, IOWeight, and optional absolute disk I/O limits.
func WriteSystemdSlice(systemUser string, l Limits) error {
	content := fmt.Sprintf(`# Servika per-domain resource slice: %s
[Unit]
Description=Servika panel slice for %s
Before=slices.target

[Slice]
CPUAccounting=yes
MemoryAccounting=yes
TasksAccounting=yes
IOAccounting=yes

CPUQuota=%d%%
MemoryMax=%dM
MemoryHigh=%dM
TasksMax=%d
IOWeight=%d
%s`, systemUser, systemUser,
		nonzero(l.CPUPercent, 100),
		nonzero(l.RAMMB, 512),
		nonzero(l.RAMMB, 512)*90/100, // MemoryHigh = 90% of Max (soft throttle)
		nonzero(l.MaxProcess, 50),
		nonzero(l.IOWeight, 100),
		ioSliceLines(l))

	if err := os.WriteFile(slicePath(systemUser), []byte(content), 0644); err != nil {
		return fmt.Errorf("write slice: %w", err)
	}
	if out, err := resourceCommand("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if err := resourceCommand("systemctl", "is-active", "--quiet", sliceName(systemUser)).Run(); err == nil {
		properties := []string{
			"set-property", "--runtime", sliceName(systemUser),
			fmt.Sprintf("CPUQuota=%d%%", nonzero(l.CPUPercent, 100)),
			fmt.Sprintf("MemoryMax=%dM", nonzero(l.RAMMB, 512)),
			fmt.Sprintf("MemoryHigh=%dM", nonzero(l.RAMMB, 512)*90/100),
			fmt.Sprintf("TasksMax=%d", nonzero(l.MaxProcess, 50)),
			fmt.Sprintf("IOWeight=%d", nonzero(l.IOWeight, 100)),
		}
		properties = append(properties, ioSetPropertyArgs(l)...)
		if out, err := resourceCommand("systemctl", properties...).CombinedOutput(); err != nil {
			return fmt.Errorf("update active slice: %s: %w", strings.TrimSpace(string(out)), err)
		}
		clearKernelIOLimits(systemUser, l)
	}
	return nil
}

func ioSliceLines(l Limits) string {
	var lines strings.Builder
	if l.IOReadMBps > 0 {
		fmt.Fprintf(&lines, "IOReadBandwidthMax=%s %dM\n", ioDevicePath, l.IOReadMBps)
	}
	if l.IOWriteMBps > 0 {
		fmt.Fprintf(&lines, "IOWriteBandwidthMax=%s %dM\n", ioDevicePath, l.IOWriteMBps)
	}
	if l.IOReadIOPS > 0 {
		fmt.Fprintf(&lines, "IOReadIOPSMax=%s %d\n", ioDevicePath, l.IOReadIOPS)
	}
	if l.IOWriteIOPS > 0 {
		fmt.Fprintf(&lines, "IOWriteIOPSMax=%s %d\n", ioDevicePath, l.IOWriteIOPS)
	}
	return lines.String()
}

func ioSetPropertyArgs(l Limits) []string {
	argument := func(property string, value int, bandwidth bool) string {
		if value <= 0 {
			return property + "="
		}
		if bandwidth {
			return fmt.Sprintf("%s=%s %dM", property, ioDevicePath, value)
		}
		return fmt.Sprintf("%s=%s %d", property, ioDevicePath, value)
	}
	return []string{
		argument("IOReadBandwidthMax", l.IOReadMBps, true),
		argument("IOWriteBandwidthMax", l.IOWriteMBps, true),
		argument("IOReadIOPSMax", l.IOReadIOPS, false),
		argument("IOWriteIOPSMax", l.IOWriteIOPS, false),
	}
}

// clearKernelIOLimits removes stale live limits that systemd empty assignments may retain.
func clearKernelIOLimits(systemUser string, l Limits) {
	clears := make([]string, 0, 4)
	if l.IOReadMBps <= 0 {
		clears = append(clears, "rbps=max")
	}
	if l.IOWriteMBps <= 0 {
		clears = append(clears, "wbps=max")
	}
	if l.IOReadIOPS <= 0 {
		clears = append(clears, "riops=max")
	}
	if l.IOWriteIOPS <= 0 {
		clears = append(clears, "wiops=max")
	}
	if len(clears) == 0 {
		return
	}

	output, err := resourceCommand(
		"systemctl", "show", sliceName(systemUser), "-p", "ControlGroup", "--value",
	).Output()
	controlGroup := strings.TrimSpace(string(output))
	if err != nil || controlGroup == "" {
		return
	}
	ioMaxPath := filepath.Join("/sys/fs/cgroup", controlGroup, "io.max")
	data, err := os.ReadFile(ioMaxPath)
	if err != nil {
		return
	}
	suffix := " " + strings.Join(clears, " ")
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		_ = os.WriteFile(ioMaxPath, []byte(fields[0]+suffix), 0644)
	}
}

// DeleteSystemdSlice removes the systemd slice when it exists.
func DeleteSystemdSlice(systemUser string) error {
	p := slicePath(systemUser)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	_ = os.Remove(p)
	_, _ = resourceCommand("systemctl", "daemon-reload").CombinedOutput()
	return nil
}

// ApplyXFSQuota applies inode and block project quotas to a user's directory through xfs_quota.
// The /home filesystem must use XFS with the pquota mount option enabled.
func ApplyXFSQuota(systemUser string, l Limits) error {
	home := "/home/" + systemUser
	if _, err := os.Stat(home); os.IsNotExist(err) {
		return nil
	}
	// Use the UID as a simple project ID mapping.
	uidOut, err := resourceCommand("id", "-u", systemUser).Output()
	if err != nil {
		return fmt.Errorf("get UID: %w", err)
	}
	projID := strings.TrimSpace(string(uidOut))
	if projID == "" || projID == "0" {
		return fmt.Errorf("invalid UID: %s", projID)
	}

	// Apply xfs_quota when supported and skip silently otherwise.
	// Block limits use kilobytes, so disk_quota_mb is multiplied by 1024.
	blockKB := l.DiskQuotaMB * 1024
	inode := l.InodeQuota
	if blockKB <= 0 && inode <= 0 {
		return nil
	}
	// Add the project mapping idempotently.
	line := fmt.Sprintf("%s:%s\n", projID, home)
	f, _ := os.OpenFile("/etc/projid", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if f != nil {
		defer f.Close()
		f.WriteString(line)
	}
	// Initialize the project quota idempotently and ignore unsupported operations.
	_ = resourceCommand("xfs_quota", "-x", "-c",
		fmt.Sprintf("project -s -p %s %s", home, projID), "/home").Run()

	limit := fmt.Sprintf("limit -p bsoft=%dk bhard=%dk isoft=%d ihard=%d %s",
		blockKB, blockKB, inode, inode, projID)
	if out, err := resourceCommand("xfs_quota", "-x", "-c", limit, "/home").CombinedOutput(); err != nil {
		// Continue when XFS quotas are unavailable, such as when the pquota mount option is absent.
		log.Printf("xfs_quota %s: %s (the pquota mount option may be inactive)", systemUser, strings.TrimSpace(string(out)))
	}
	return nil
}

var (
	mysqlAccountPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	mysqlHostPattern    = regexp.MustCompile(`^[A-Za-z0-9_.%\-]{1,64}$`)
	protectedMySQLUsers = map[string]bool{
		"root": true, "mysql": true, "mariadb.sys": true, "panel": true,
		"event_scheduler": true, "debian-sys-maint": true, "replication": true,
		"repl": true, "healthcheck": true, "": true,
	}
)

func governedMySQLAccount(user string) bool {
	return mysqlAccountPattern.MatchString(user) && !protectedMySQLUsers[strings.ToLower(user)]
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func mysqlLimitSQL(user, host string, l Limits) (string, error) {
	if !governedMySQLAccount(user) {
		return "", fmt.Errorf("invalid or protected MySQL username")
	}
	if !mysqlHostPattern.MatchString(host) {
		return "", fmt.Errorf("invalid MySQL host")
	}
	return fmt.Sprintf(
		"ALTER USER '%s'@'%s' WITH MAX_USER_CONNECTIONS %d MAX_QUERIES_PER_HOUR %d MAX_UPDATES_PER_HOUR %d;",
		user, host, nonNegative(l.MySQLMaxConnections), nonNegative(l.DBMaxQueriesPerHour),
		nonNegative(l.DBMaxUpdatesPerHour)), nil
}

func parseMySQLAccountHosts(output string) map[string][]string {
	hosts := make(map[string][]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			continue
		}
		user := strings.TrimSpace(fields[0])
		host := strings.TrimSpace(fields[1])
		if user != "" && host != "" {
			hosts[user] = append(hosts[user], host)
		}
	}
	return hosts
}

func mysqlAccountHosts(ctx context.Context) (map[string][]string, error) {
	command := resourceCommandContext(ctx, "mysql", "-uroot", "-N", "-B", "-e",
		"SELECT User,Host FROM mysql.user")
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list MariaDB account hosts: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return parseMySQLAccountHosts(string(output)), nil
}

func mysqlLimitStatements(users []string, accountHosts map[string][]string, l Limits) []string {
	var statements []string
	for _, user := range users {
		if !governedMySQLAccount(user) {
			log.Printf("mysql governor skipped account %q: invalid or protected username", user)
			continue
		}
		hosts := accountHosts[user]
		if len(hosts) == 0 {
			log.Printf("mysql governor skipped account %q: no MariaDB host found", user)
			continue
		}
		for _, host := range hosts {
			statement, err := mysqlLimitSQL(user, host, l)
			if err != nil {
				log.Printf("mysql governor skipped account %q at host %q: %v", user, host, err)
				continue
			}
			statements = append(statements, statement)
		}
	}
	return statements
}

// ApplyMySQLLimits applies native MariaDB limits to every host of each database account owned by a domain.
func ApplyMySQLLimits(ctx context.Context, db *sql.DB, domainID int64, l Limits) error {
	rows, err := db.QueryContext(ctx, `SELECT db_user FROM db_accounts WHERE domain_id=?`, domainID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var users []string
	for rows.Next() {
		var user string
		if err := rows.Scan(&user); err != nil {
			return err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}

	accountHosts, err := mysqlAccountHosts(ctx)
	if err != nil {
		return err
	}
	statements := mysqlLimitStatements(users, accountHosts, l)
	if len(statements) == 0 {
		return nil
	}
	statements = append(statements, "FLUSH USER_RESOURCES;")
	command := resourceCommandContext(ctx, "mysql", "-uroot", "-e", strings.Join(statements, ""))
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("mysql governor: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

const governorPollInterval = 5 * time.Second

// SlowQueryWatchdog terminates tenant queries that exceed their plan duration limit.
func SlowQueryWatchdog(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	ticker := time.NewTicker(governorPollInterval)
	defer ticker.Stop()
	log.Printf("MySQL governor slow-query watchdog started with interval %s", governorPollInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			governorScanOnce(ctx, db)
		}
	}
}

func queryExceedsLimit(seconds, limit int) bool {
	return limit > 0 && seconds > limit
}

func governorScanOnce(ctx context.Context, db *sql.DB) {
	output, err := resourceCommandContext(ctx, "mysql", "-uroot", "-N", "-B", "-e",
		"SELECT ID,USER,TIME FROM information_schema.PROCESSLIST WHERE COMMAND<>'Sleep' AND TIME>0").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		queryID, idErr := strconv.Atoi(strings.TrimSpace(fields[0]))
		user := strings.TrimSpace(fields[1])
		seconds, secondsErr := strconv.Atoi(strings.TrimSpace(fields[2]))
		if idErr != nil || secondsErr != nil || seconds <= 0 || !governedMySQLAccount(user) {
			continue
		}

		var limit int
		err := db.QueryRowContext(ctx,
			`SELECT COALESCE(p.db_max_query_seconds,0)
			 FROM db_accounts a JOIN domains d ON d.id=a.domain_id
			 LEFT JOIN service_plans p ON p.id=d.plan_id
			 WHERE a.db_user=? LIMIT 1`, user).Scan(&limit)
		if err != nil || !queryExceedsLimit(seconds, limit) {
			continue
		}
		killOutput, killErr := resourceCommandContext(ctx, "mysql", "-uroot", "-e",
			fmt.Sprintf("KILL QUERY %d", queryID)).CombinedOutput()
		if killErr != nil {
			log.Printf("MySQL governor failed to terminate query for %s (id=%d): %s: %v",
				user, queryID, strings.TrimSpace(string(killOutput)), killErr)
			continue
		}
		log.Printf("MySQL governor terminated query for %s after %ds, limit %ds (id=%d)",
			user, seconds, limit, queryID)
	}
}

// ApplyAll applies systemd slice, tenant PHP-FPM, XFS, and MySQL limits from a domain's plan.
func ApplyAll(ctx context.Context, db *sql.DB, domainID int64) error {
	var systemUser, phpVersion string
	var planID sql.NullInt64
	if err := db.QueryRowContext(ctx,
		`SELECT system_user, COALESCE(php_version,'8.3'), plan_id
		 FROM domains WHERE id=?`, domainID).
		Scan(&systemUser, &phpVersion, &planID); err != nil {
		return err
	}
	if systemUser == "" {
		return fmt.Errorf("system_user is empty")
	}
	if !planID.Valid {
		if provisioner.TenantFPMActive(systemUser) {
			if err := provisioner.RollbackToSharedFPM(db, domainID, systemUser, phpVersion); err != nil {
				return fmt.Errorf("rollback tenant PHP-FPM: %w", err)
			}
		}
		return DeleteSystemdSlice(systemUser)
	}

	l, err := GetPlanLimits(ctx, db, domainID)
	if err != nil {
		return err
	}
	if err := WriteSystemdSlice(systemUser, l); err != nil {
		log.Printf("write slice %s: %v", systemUser, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO php_settings(domain_id, pm_max_children)
		VALUES(?,?) ON DUPLICATE KEY UPDATE pm_max_children=VALUES(pm_max_children)`,
		domainID, calculatePMMaxChildren(l)); err != nil {
		return fmt.Errorf("store PHP-FPM worker limit: %w", err)
	}
	if _, err := provisioner.EnableTenantFPM(db, domainID, systemUser, phpVersion); err != nil {
		log.Printf("tenant PHP-FPM %s: %v", systemUser, err)
	}
	if err := ApplyXFSQuota(systemUser, l); err != nil {
		log.Printf("xfs quota %s: %v", systemUser, err)
	}
	if err := ApplyMySQLLimits(ctx, db, domainID, l); err != nil {
		log.Printf("mysql governor %s: %v", systemUser, err)
	}
	return nil
}

func nonzero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func planProbeHTTPS(domainName string) int {
	if provisioner.ValidateDomain(domainName) != nil {
		return 0
	}
	output, _ := resourceCommand("curl", "-sk", "--max-time", "10",
		"-o", os.DevNull, "-w", "%{http_code}",
		"-H", "Host: "+domainName, "https://127.0.0.1/").Output()
	status, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return status
}

func tenantServiceActive(unit string) bool {
	output, _ := resourceCommand("systemctl", "is-active", unit).CombinedOutput()
	return strings.TrimSpace(string(output)) == "active"
}

func tenantCutoverRegressed(baseline, post int) bool {
	return baseline >= 200 && baseline < 500 && post >= 500
}

// HealTenantFPM migrates planned domains to isolated PHP-FPM services with rollback checks.
func HealTenantFPM(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, system_user, COALESCE(php_version,'8.3'), domain_name
		 FROM domains WHERE plan_id IS NOT NULL ORDER BY id`)
	if err != nil {
		log.Printf("tenant PHP-FPM healing could not list domains: %v", err)
		return
	}

	type domain struct {
		id         int64
		systemUser string
		phpVersion string
		domainName string
	}
	var domains []domain
	for rows.Next() {
		var item domain
		if err := rows.Scan(&item.id, &item.systemUser, &item.phpVersion, &item.domainName); err != nil {
			log.Printf("tenant PHP-FPM healing skipped an unreadable domain row: %v", err)
			continue
		}
		domains = append(domains, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("tenant PHP-FPM healing stopped while reading domains: %v", err)
		_ = rows.Close()
		return
	}
	if err := rows.Close(); err != nil {
		log.Printf("tenant PHP-FPM healing could not close domain rows: %v", err)
		return
	}

	var migrated, alreadyActive, rolledBack int
	for _, item := range domains {
		select {
		case <-ctx.Done():
			log.Printf("tenant PHP-FPM healing canceled: migrated=%d active=%d rolled_back=%d", migrated, alreadyActive, rolledBack)
			return
		default:
		}
		if item.systemUser == "" || !strings.HasPrefix(item.systemUser, "c_") {
			continue
		}
		if provisioner.TenantFPMActive(item.systemUser) {
			alreadyActive++
			continue
		}

		baseline := planProbeHTTPS(item.domainName)
		if err := ApplyAll(ctx, db, item.id); err != nil {
			log.Printf("tenant PHP-FPM healing failed to apply limits for %s: %v", item.systemUser, err)
		}
		if !provisioner.TenantFPMActive(item.systemUser) {
			log.Printf("tenant PHP-FPM healing left %s on the shared service after cutover failure", item.systemUser)
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(700 * time.Millisecond):
		}
		active := tenantServiceActive("php-fpm-" + item.systemUser + ".service")
		post := planProbeHTTPS(item.domainName)
		if !active || tenantCutoverRegressed(baseline, post) {
			log.Printf("tenant PHP-FPM healing is rolling back %s: active=%v baseline=%d post=%d", item.systemUser, active, baseline, post)
			if err := provisioner.RollbackToSharedFPM(db, item.id, item.systemUser, item.phpVersion); err != nil {
				log.Printf("tenant PHP-FPM healing rollback failed for %s: %v", item.systemUser, err)
			}
			_ = DeleteSystemdSlice(item.systemUser)
			rolledBack++
			continue
		}
		log.Printf("tenant PHP-FPM healing completed cutover for %s: baseline=%d post=%d", item.systemUser, baseline, post)
		migrated++
	}
	log.Printf("tenant PHP-FPM healing completed: migrated=%d active=%d rolled_back=%d planned=%d", migrated, alreadyActive, rolledBack, len(domains))
}
