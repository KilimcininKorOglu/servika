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
	"strings"
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
}

// GetPlanLimits returns limits from the domain's assigned plan.
// An unassigned plan returns zero values, which remove enforcement.
func GetPlanLimits(ctx context.Context, db *sql.DB, domainID int64) (Limits, error) {
	var l Limits
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(p.cpu_percent,0), COALESCE(p.ram_mb,0),
		       COALESCE(p.max_process,0), COALESCE(p.inode_quota,0),
		       COALESCE(p.io_weight,100), COALESCE(p.mysql_max_connections,0),
		       COALESCE(p.disk_quota_mb,0)
		FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id
		WHERE d.id=?`, domainID).
		Scan(&l.CPUPercent, &l.RAMMB, &l.MaxProcess, &l.InodeQuota,
			&l.IOWeight, &l.MySQLMaxConnections, &l.DiskQuotaMB)
	return l, err
}

const sliceDir = "/etc/systemd/system"

func sliceName(systemUser string) string {
	// systemd slice, for example servika-c_registry_persistent_test_local.slice.
	return "servika-" + systemUser + ".slice"
}

func slicePath(systemUser string) string {
	return filepath.Join(sliceDir, sliceName(systemUser))
}

// WriteSystemdSlice writes /etc/systemd/system/servika-<system-user>.slice with cgroup v2
// CPUQuota, MemoryMax, TasksMax, and IOWeight controls.
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
`, systemUser, systemUser,
		nonzero(l.CPUPercent, 100),
		nonzero(l.RAMMB, 512),
		nonzero(l.RAMMB, 512)*90/100, // MemoryHigh = 90% of Max (soft throttle)
		nonzero(l.MaxProcess, 50),
		nonzero(l.IOWeight, 100))

	if err := os.WriteFile(slicePath(systemUser), []byte(content), 0644); err != nil {
		return fmt.Errorf("write slice: %w", err)
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// Restart the slice so existing processes receive the new limits.
	_, _ = exec.Command("systemctl", "restart", sliceName(systemUser)).CombinedOutput()
	return nil
}

// DeleteSystemdSlice removes the systemd slice when it exists.
func DeleteSystemdSlice(systemUser string) error {
	p := slicePath(systemUser)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	_ = os.Remove(p)
	_, _ = exec.Command("systemctl", "daemon-reload").CombinedOutput()
	return nil
}

// ApplyPHPFPMSlicePool adds resource-limit fallback settings to /etc/php-fpm.d/<system-user>.conf.
// PHP-FPM pools run as children of a shared systemd-managed master process, so assigning a per-pool
// systemd slice through a service override would incorrectly move the entire PHP-FPM service.
// A practical future approach is to place initial pool workers in the slice with
// `systemd-run --slice=<slice-name>` or move pool processes directly into the cgroup.
func ApplyPHPFPMSlicePool(systemUser string, l Limits) error {
	// Per-pool cgroup enrollment is not persistent because PHP-FPM workers are children of a shared
	// master process. A Delegate=yes cgroup could isolate enrollment without affecting the master.
	// The current implementation writes the domain slice and uses pool-level rlimit_* settings as
	// a persistent fallback. Manual cgclassify enrollment would be lost after a PHP-FPM restart.
	pool := "/etc/php-fpm.d/" + systemUser + ".conf"
	if _, err := os.Stat(pool); os.IsNotExist(err) {
		return nil // No-op when the pool does not exist.
	}
	// Add rlimit settings as a cgroup-independent fallback.
	b, err := os.ReadFile(pool)
	if err != nil {
		return err
	}
	body := string(b)
	// Remove obsolete limit lines.
	lines := []string{}
	for _, line := range strings.Split(body, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "rlimit_") || strings.HasPrefix(s, "; servika-limit") {
			continue
		}
		lines = append(lines, line)
	}
	body = strings.Join(lines, "\n")
	limitBlock := fmt.Sprintf("\n; servika-limit - managed by the plan\nrlimit_files = %d\nrlimit_core = 0\n",
		nonzero(l.MaxProcess, 50)*4) // Set rlimit_files to approximately four times pm.max_children.
	body += limitBlock
	if err := os.WriteFile(pool, []byte(body), 0644); err != nil {
		return err
	}
	// Preserve the version hook for a future PHP-FPM master reload.
	phpVersion := "8.3"
	if b, _ := os.ReadFile("/etc/php-fpm.d/" + systemUser + ".conf"); len(b) > 0 {
		// Use 8.3 until the pool contains a reliable version hint.
	}
	_ = phpVersion
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
	uidOut, err := exec.Command("id", "-u", systemUser).Output()
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
	_ = exec.Command("xfs_quota", "-x", "-c",
		fmt.Sprintf("project -s -p %s %s", home, projID), "/home").Run()

	limit := fmt.Sprintf("limit -p bsoft=%dk bhard=%dk isoft=%d ihard=%d %s",
		blockKB, blockKB, inode, inode, projID)
	if out, err := exec.Command("xfs_quota", "-x", "-c", limit, "/home").CombinedOutput(); err != nil {
		// Continue when XFS quotas are unavailable, such as when the pquota mount option is absent.
		log.Printf("xfs_quota %s: %s (the pquota mount option may be inactive)", systemUser, strings.TrimSpace(string(out)))
	}
	return nil
}

var mysqlAccountPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

func mysqlLimitSQL(mysqlDBUser string, maxConnections int) (string, error) {
	if !mysqlAccountPattern.MatchString(mysqlDBUser) {
		return "", fmt.Errorf("invalid MySQL username")
	}
	return fmt.Sprintf(
		"GRANT USAGE ON *.* TO '%s'@'localhost' WITH MAX_USER_CONNECTIONS %d;FLUSH PRIVILEGES;",
		mysqlDBUser, maxConnections), nil
}

// ApplyMySQLLimit sets MAX_USER_CONNECTIONS for a database user.
func ApplyMySQLLimit(_ string, l Limits, mysqlDBUser string) error {
	if l.MySQLMaxConnections <= 0 {
		return nil
	}
	sqlCmd, err := mysqlLimitSQL(mysqlDBUser, l.MySQLMaxConnections)
	if err != nil {
		return err
	}
	cmd := exec.Command("mysql", "-uroot", "-e", sqlCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mysql limit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ApplyAll applies systemd slice, PHP-FPM, XFS, and MySQL limits from a domain's plan.
func ApplyAll(ctx context.Context, db *sql.DB, domainID int64) error {
	var systemUser, dbUser string
	if err := db.QueryRowContext(ctx,
		`SELECT system_user, COALESCE(db_user,'') FROM domains WHERE id=?`, domainID).
		Scan(&systemUser, &dbUser); err != nil {
		return err
	}
	if systemUser == "" {
		return fmt.Errorf("system_user is empty")
	}
	l, err := GetPlanLimits(ctx, db, domainID)
	if err != nil {
		return err
	}
	// Remove enforcement when no plan is assigned.
	if l.CPUPercent == 0 && l.RAMMB == 0 && l.MaxProcess == 0 {
		_ = DeleteSystemdSlice(systemUser)
		return nil
	}
	if err := WriteSystemdSlice(systemUser, l); err != nil {
		log.Printf("write slice %s: %v", systemUser, err)
	}
	if err := ApplyPHPFPMSlicePool(systemUser, l); err != nil {
		log.Printf("fpm pool %s: %v", systemUser, err)
	}
	if err := ApplyXFSQuota(systemUser, l); err != nil {
		log.Printf("xfs quota %s: %v", systemUser, err)
	}
	if dbUser != "" {
		if err := ApplyMySQLLimit(systemUser, l, dbUser); err != nil {
			log.Printf("mysql limit %s: %v", systemUser, err)
		}
	}
	return nil
}

func nonzero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
