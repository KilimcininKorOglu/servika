// Package backups manages per-domain archives and database dumps.
package backups

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// BackupRoot is the root directory for domain backup archives.
const BackupRoot = "/var/backups/servika"

var systemUserPattern = regexp.MustCompile(`^c_[A-Za-z0-9_]+$`)

func validSystemUser(systemUser string) bool {
	return systemUserPattern.MatchString(systemUser)
}

// RemoveDomainBackups removes a domain's backup directory after validating its system user.
// It is intentionally not called by domain deletion so operators can recover accidental deletions.
func RemoveDomainBackups(systemUser string) error {
	if !validSystemUser(systemUser) {
		return fmt.Errorf("invalid system user: %q", systemUser)
	}
	dir := filepath.Join(BackupRoot, systemUser)
	if dir == BackupRoot || !strings.HasPrefix(dir, BackupRoot+"/") {
		return fmt.Errorf("unsafe backup path: %q", dir)
	}
	return os.RemoveAll(dir)
}

// Backup describes a stored domain backup.
type Backup struct {
	ID        int64  `json:"id"`
	DomainID  int64  `json:"domain_id"`
	Type      string `json:"type"`
	File      string `json:"file"`
	SizeBytes int64  `json:"size_b"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
}

// Handlers provides backup HTTP handlers.
type Handlers struct {
	DB *sql.DB
}

func (h *Handlers) lookupDomain(r *http.Request) (id int64, domainName, systemUser string, demo bool, err error) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var demoValue int
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, is_demo FROM domains WHERE id=?`, id).
		Scan(&domainName, &systemUser, &demoValue)
	demo = demoValue == 1
	return
}

// List returns a domain's backup records.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, domain_id, type, file, size_b, notes, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		 FROM backups WHERE domain_id=? ORDER BY id DESC`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()
	out := make([]Backup, 0)
	for rows.Next() {
		var y Backup
		if err := rows.Scan(&y.ID, &y.DomainID, &y.Type, &y.File, &y.SizeBytes, &y.Notes, &y.CreatedAt); err == nil {
			out = append(out, y)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// SummaryRow describes one domain in the server-wide backup summary.
type SummaryRow struct {
	DomainID   int64  `json:"domain_id"`
	DomainName string `json:"domain_name"`
	Count      int    `json:"count"`
	TotalBytes int64  `json:"total_bytes"`
	LastBackup string `json:"last_backup"`
}

// Summary returns the server-wide backup summary using filesystem disk usage.
func (h *Handlers) Summary(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, domain_name, system_user FROM domains ORDER BY domain_name`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list backups")
		return
	}
	defer rows.Close()
	out := []SummaryRow{}
	var totalBytes int64
	var totalBackups int
	for rows.Next() {
		var id int64
		var domainName, systemUser string
		if err := rows.Scan(&id, &domainName, &systemUser); err != nil {
			continue
		}
		s := SummaryRow{DomainID: id, DomainName: domainName}
		var latestModification time.Time
		if entries, e := os.ReadDir(filepath.Join(BackupRoot, systemUser)); e == nil {
			for _, en := range entries {
				if en.IsDir() || !strings.HasSuffix(en.Name(), ".tar.gz") {
					continue
				}
				fi, e2 := en.Info()
				if e2 != nil {
					continue
				}
				s.Count++
				s.TotalBytes += fi.Size()
				if fi.ModTime().After(latestModification) {
					latestModification = fi.ModTime()
				}
			}
		}
		if !latestModification.IsZero() {
			s.LastBackup = latestModification.Format("2006-01-02 15:04")
		}
		out = append(out, s)
		totalBytes += s.TotalBytes
		totalBackups += s.Count
	}
	_ = rows.Err()
	var destinationCount int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM backup_destinations WHERE enabled=1`).Scan(&destinationCount)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"domains":           out,
		"total_size_bytes":  totalBytes,
		"total_backups":     totalBackups,
		"destination_count": destinationCount,
		"schedule":          "Daily at 03:00 (automatic)",
	})
}

// Create generates and stores a full domain backup.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id, domainName, systemUser, demo, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "backups are unavailable for demo subscriptions")
		return
	}
	if !validSystemUser(systemUser) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid system user")
		return
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	dir := filepath.Join(BackupRoot, systemUser)
	_ = os.MkdirAll(dir, 0700)
	file := fmt.Sprintf("%s-%s.tar.gz", systemUser, stamp)
	abs := filepath.Join(dir, file)

	// DB dump
	dbName := systemUser + "_main"
	sqlDump := filepath.Join(dir, file+".sql")
	if out, derr := exec.Command("bash", "-c",
		fmt.Sprintf("mysqldump --single-transaction %s > %s 2>&1 || true", dbName, sqlDump)).CombinedOutput(); derr != nil {
		_ = os.WriteFile(sqlDump+".err", out, 0600)
	}

	// Archive the home directory and database dump together.
	args := []string{
		"czf", abs,
		"-C", "/home", systemUser,
		"-C", dir, file + ".sql",
	}
	if _, tarErr := exec.Command("tar", args...).CombinedOutput(); tarErr != nil {
		_ = os.Remove(sqlDump)
		httpx.WriteError(w, http.StatusInternalServerError, "could not create backup archive")
		return
	}
	_ = os.Remove(sqlDump)

	st, _ := os.Stat(abs)
	var sizeBytes int64
	if st != nil {
		sizeBytes = st.Size()
	}

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO backups(domain_id, type, file, size_b, notes) VALUES(?,?,?,?,?)`,
		id, "full", file, sizeBytes, "domain: "+domainName)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not save backup record")
		return
	}
	backupID, _ := res.LastInsertId()
	// If a remote destination exists, upload in the background (do not block the API response)
	pushToDestinationAsync(h.DB, id, abs, file)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":         true,
		"id":         backupID,
		"file":       file,
		"size_bytes": sizeBytes,
	})
}

// Delete removes a domain backup record and archive.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	backupID, _ := strconv.ParseInt(chi.URLParam(r, "backupID"), 10, 64)
	var systemUser, file string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT d.system_user, b.file FROM backups b
		 JOIN domains d ON d.id=b.domain_id
		 WHERE b.id=? AND b.domain_id=?`, backupID, id).Scan(&systemUser, &file)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err == nil {
		_ = os.Remove(filepath.Join(BackupRoot, systemUser, file))
	}
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM backups WHERE id=?`, backupID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Download streams a domain backup archive.
func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	backupID, _ := strconv.ParseInt(chi.URLParam(r, "backupID"), 10, 64)
	var systemUser, file string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT d.system_user, b.file FROM backups b
		 JOIN domains d ON d.id=b.domain_id
		 WHERE b.id=? AND b.domain_id=?`, backupID, id).Scan(&systemUser, &file)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "backup not found")
		return
	}
	abs := filepath.Join(BackupRoot, systemUser, file)
	f, err := os.Open(abs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+file+`"`)
	if st != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	_, _ = io.Copy(w, f)
}
