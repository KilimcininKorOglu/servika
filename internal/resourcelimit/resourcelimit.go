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
}

// GetPlanLimits returns limits from the domain's assigned plan.
// An unassigned plan returns zero values, which remove enforcement.
func GetPlanLimits(ctx context.Context, db *sql.DB, domainID int64) (Limits, error) {
	var l Limits
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(p.cpu_percent,0), COALESCE(p.ram_mb,0),
		       COALESCE(p.max_process,0), COALESCE(p.inode_quota,0),
		       COALESCE(p.io_weight,100), COALESCE(p.mysql_max_connections,0),
		       COALESCE(p.disk_quota_mb,0), COALESCE(p.pm_max_children,0)
		FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id
		WHERE d.id=?`, domainID).
		Scan(&l.CPUPercent, &l.RAMMB, &l.MaxProcess, &l.InodeQuota,
			&l.IOWeight, &l.MySQLMaxConnections, &l.DiskQuotaMB, &l.PMMaxChildren)
	return l, err
}

const sliceDir = "/etc/systemd/system"

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
	command := exec.Command(name, args...)
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
		if out, err := resourceCommand("systemctl", properties...).CombinedOutput(); err != nil {
			return fmt.Errorf("update active slice: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	return nil
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
	cmd := resourceCommand("mysql", "-uroot", "-e", sqlCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mysql limit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ApplyAll applies systemd slice, tenant PHP-FPM, XFS, and MySQL limits from a domain's plan.
func ApplyAll(ctx context.Context, db *sql.DB, domainID int64) error {
	var systemUser, dbUser, phpVersion string
	var planID sql.NullInt64
	if err := db.QueryRowContext(ctx,
		`SELECT system_user, COALESCE(db_user,''), COALESCE(php_version,'8.3'), plan_id
		 FROM domains WHERE id=?`, domainID).
		Scan(&systemUser, &dbUser, &phpVersion, &planID); err != nil {
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
