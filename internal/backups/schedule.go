// The backup scheduler runs in a background goroutine and checks schedules hourly.
// Each tick: SELECT due domains, run backup, prune old by retention.
package backups

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Schedule defines automatic backup timing and retention.
type Schedule struct {
	Frequency    string `json:"frequency"`      // "none" | "daily" | "weekly"
	Hour         int    `json:"hour"`           // 0-23
	Retention    int    `json:"retention"`      // keep last N
	LastBackupAt string `json:"last_backup_at"` // RFC3339 or empty
}

func validFrequency(f string) bool {
	return f == "none" || f == "daily" || f == "weekly"
}

// StartScheduler starts the hourly backup scheduler in a background goroutine.
// At the top of each hour (~ +60s offset) it scans due domains and backs them up.
func StartScheduler(db *sql.DB) {
	go func() {
		// First run: 2 minutes after the panel starts (warmup)
		time.Sleep(2 * time.Minute)
		tickOnce(db)
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for range t.C {
			tickOnce(db)
		}
	}()
}

type dueDomain struct {
	ID         int64
	DomainName string
	SystemUser string
	Frequency  string
	Hour       int
	Retention  int
	IsDemo     int
}

// TickOnce runs one scheduler pass for tests or an operator-triggered backup.
func TickOnce(db *sql.DB) { tickOnce(db) }

// tickOnce: find domains due for this hour, back them up, apply retention.
func tickOnce(db *sql.DB) {
	now := time.Now()
	currentHour := now.Hour()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT id, domain_name, system_user,
		       COALESCE(backup_freq,'none'), COALESCE(backup_hour,3),
		       COALESCE(backup_retention,7), is_demo,
		       UNIX_TIMESTAMP(last_backup_at)
		FROM domains
		WHERE COALESCE(backup_freq,'none') != 'none'
		  AND COALESCE(backup_hour,3) = ?
		  AND is_demo = 0`,
		currentHour)
	if err != nil {
		log.Printf("backup scheduler tick query: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()

	var due []dueDomain
	for rows.Next() {
		var d dueDomain
		var lastTs sql.NullInt64
		if err := rows.Scan(&d.ID, &d.DomainName, &d.SystemUser, &d.Frequency, &d.Hour, &d.Retention, &d.IsDemo, &lastTs); err != nil {
			log.Printf("backup scheduler scan: %v", err)
			continue
		}
		// Filter: if freq=daily, 23 hours must have passed; if weekly, 6.5 days
		// (slack: to avoid missing when it lands on a day/week boundary)
		minSec := int64(23 * 3600)
		if d.Frequency == "weekly" {
			minSec = int64(6*24*3600 + 12*3600)
		}
		if lastTs.Valid && (now.Unix()-lastTs.Int64) < minSec {
			continue
		}
		due = append(due, d)
	}

	if len(due) == 0 {
		return
	}
	log.Printf("backup scheduler: %d due domain found", len(due))

	for _, d := range due {
		if err := runOneBackup(db, d); err != nil {
			log.Printf("backup scheduler %s: %v", d.DomainName, err)
			continue
		}
		if err := pruneOld(db, d.ID, d.SystemUser, d.Retention); err != nil {
			log.Printf("backup retention %s: %v", d.DomainName, err)
		}
	}
}

// runOneBackup: create a backup for a domain + save to DB + update last_backup_at.
func runOneBackup(db *sql.DB, d dueDomain) error {
	if !validSystemUser(d.SystemUser) {
		return fmt.Errorf("unsafe system user: %s", d.SystemUser)
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	dir := filepath.Join(backupRoot(), d.SystemUser)
	_ = os.MkdirAll(dir, 0700)
	file := fmt.Sprintf("%s-auto-%s.tar.gz", d.SystemUser, stamp)
	abs := filepath.Join(dir, file)

	// Dump stdout only (no stderr into the .sql) and drop "|| true" so a failed dump
	// aborts the scheduled backup instead of archiving a corrupt/empty dump as success.
	// Archive the dump under the canonical name "dump.sql" (in a unique temp dir to
	// avoid concurrent-backup collisions) so the restore path can find and import it.
	dbName := d.SystemUser + "_main"
	dumpDir, err := os.MkdirTemp("", "servika-dump-*")
	if err != nil {
		return fmt.Errorf("prepare dump dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dumpDir) }()
	sqlDump := filepath.Join(dumpDir, "dump.sql")
	if out, err := exec.Command("bash", "-c",
		fmt.Sprintf("mysqldump --single-transaction %s > %s", dbName, sqlDump)).CombinedOutput(); err != nil {
		return fmt.Errorf("mysqldump %s: %s: %w", dbName, strings.TrimSpace(string(out)), err)
	}

	args := []string{"czf", abs, "-C", "/home", d.SystemUser, "-C", dumpDir, "dump.sql"}
	if out, err := exec.Command("tar", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %s: %w", strings.TrimSpace(string(out)), err)
	}

	st, _ := os.Stat(abs)
	var sizeBytes int64
	if st != nil {
		sizeBytes = st.Size()
	}

	if _, err := db.Exec(
		`INSERT INTO backups(domain_id, type, file, size_b, notes) VALUES(?,?,?,?,?)`,
		d.ID, "scheduled", file, sizeBytes, "Scheduled backup ("+d.Frequency+")"); err != nil {
		return fmt.Errorf("could not save backup record: %w", err)
	}
	if _, err := db.Exec(`UPDATE domains SET last_backup_at=NOW() WHERE id=?`, d.ID); err != nil {
		log.Printf("last_backup_at could not be updated: %v", err)
	}
	// If a remote destination exists, upload in the background
	pushToDestinationAsync(db, d.ID, abs, file)
	log.Printf("scheduled backup %s: file=%s size_bytes=%d", d.DomainName, file, sizeBytes)
	return nil
}

// pruneOld keeps the newest scheduled backups and preserves manual backups.
func pruneOld(db *sql.DB, domainID int64, systemUser string, retention int) error {
	if retention < 1 {
		retention = 1
	}
	rows, err := db.Query(
		`SELECT id, file FROM backups
		 WHERE domain_id=? AND type='scheduled'
		 ORDER BY id DESC`, domainID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type item struct {
		ID   int64
		File string
	}
	var all []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.File); err != nil {
			continue
		}
		all = append(all, it)
	}
	_ = rows.Close()
	if len(all) <= retention {
		return nil
	}
	// Keep the newest N backups and delete the rest.
	old := all[retention:]
	sort.Slice(old, func(i, j int) bool { return old[i].ID < old[j].ID })
	for _, it := range old {
		path := filepath.Join(backupRoot(), systemUser, it.File)
		_ = os.Remove(path)
		_, _ = db.Exec(`DELETE FROM backups WHERE id=?`, it.ID)
	}
	log.Printf("backup retention domain=%d: %d old backups deleted (keep %d)", domainID, len(old), retention)
	return nil
}
