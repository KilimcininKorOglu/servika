package backups

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// Restore handles POST /api/v1/domains/:id/backups/:backupID/restore.
// It extracts the archive and imports the database when dump.sql exists.
// This destructive operation overwrites public_html and recreates database tables.
func (h *Handlers) Restore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	backupID, _ := strconv.ParseInt(chi.URLParam(r, "backupID"), 10, 64)

	var systemUser, file, domainName string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT d.system_user, d.domain_name, d.is_demo, b.file FROM backups b
		 JOIN domains d ON d.id=b.domain_id
		 WHERE b.id=? AND b.domain_id=?`, backupID, id).Scan(&systemUser, &domainName, &isDemo, &file)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "restore is unavailable for demo subscriptions")
		return
	}
	if !strings.HasPrefix(systemUser, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid system user")
		return
	}

	abs := filepath.Join(BackupRoot, systemUser, file)
	if _, err := os.Stat(abs); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "backup file not found")
		return
	}

	// Temporary extract directory
	tmpDir, _ := os.MkdirTemp("", "servika-restore-*")
	defer os.RemoveAll(tmpDir)

	if _, err := exec.Command("tar", "xzf", abs, "-C", tmpDir).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not extract backup archive")
		return
	}

	// Home replace (overwrite the existing /home/c_<user>)
	// Safe: the backup contains a c_<systemUser> folder, copy that
	extractedHome := filepath.Join(tmpDir, systemUser)
	if _, err := os.Stat(extractedHome); err == nil {
		out, err := exec.Command("rsync", "-a", "--delete", extractedHome+"/", "/home/"+systemUser+"/").CombinedOutput()
		if err != nil {
			// Fall back to cp when rsync is unavailable.
			_, _ = exec.Command("cp", "-af", extractedHome+"/.", "/home/"+systemUser+"/").CombinedOutput()
			_ = out
		}
		_, _ = exec.Command("chown", "-R", systemUser+":"+systemUser, "/home/"+systemUser).CombinedOutput()
		_, _ = exec.Command("restorecon", "-R", "/home/"+systemUser).CombinedOutput()
	}

	// Import the database dump when present.
	dumpPath := filepath.Join(tmpDir, "dump.sql")
	dbName := systemUser + "_main"
	databaseImport := "not_present"
	if _, err := os.Stat(dumpPath); err == nil {
		cmd := fmt.Sprintf("mysql %s < %s 2>&1", dbName, dumpPath)
		if _, err := exec.Command("bash", "-c", cmd).CombinedOutput(); err != nil {
			databaseImport = "failed"
		} else {
			databaseImport = "successful"
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"domain_name":     domainName,
		"file":            file,
		"database_import": databaseImport,
		"warning":         "Existing files were overwritten and database tables were recreated.",
	})
}
