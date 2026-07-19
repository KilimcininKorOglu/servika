package backups

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"servika/internal/archivex"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const restoreCommandPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

func newRestoreCommand(ctx context.Context, name string, arguments ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Env = []string{"PATH=" + restoreCommandPath, "HOME=/root"}
	return command
}

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
	if !validSystemUser(systemUser) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid system user")
		return
	}
	if file == "" || filepath.Base(file) != file {
		httpx.WriteError(w, http.StatusBadRequest, "invalid backup file")
		return
	}

	abs := filepath.Join(BackupRoot, systemUser, file)
	archiveType := archivex.DetectType(abs)
	if archiveType == archivex.TypeUnknown || archiveType == archivex.TypeRAR {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported backup archive")
		return
	}
	archiveInfo, err := os.Lstat(abs)
	if err != nil || !archiveInfo.Mode().IsRegular() {
		httpx.WriteError(w, http.StatusNotFound, "backup file not found")
		return
	}

	// Extract into an isolated staging directory owned by the tenant.
	tmpDir, err := os.MkdirTemp("", "servika-restore-*")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not prepare backup restore")
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	if _, err := newRestoreCommand(r.Context(), "chown", systemUser+":"+systemUser, tmpDir).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not prepare backup restore")
		return
	}
	if _, err := archivex.Extract(r.Context(), abs, tmpDir, systemUser); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid backup archive")
		return
	}

	// Replace the existing home only when the staged entry is a real directory.
	extractedHome := filepath.Join(tmpDir, systemUser)
	if extractedInfo, err := os.Lstat(extractedHome); err == nil && extractedInfo.IsDir() {
		homeTarget := "/home/" + systemUser + "/"
		if _, err := newRestoreCommand(r.Context(), "rsync", "-a", "--delete", extractedHome+"/", homeTarget).CombinedOutput(); err != nil {
			if !errors.Is(err, exec.ErrNotFound) {
				httpx.WriteError(w, http.StatusInternalServerError, "could not restore backup files")
				return
			}
			if _, err := newRestoreCommand(r.Context(), "cp", "-af", extractedHome+"/.", homeTarget).CombinedOutput(); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "could not restore backup files")
				return
			}
		}
		if _, err := newRestoreCommand(r.Context(), "chown", "-R", systemUser+":"+systemUser, "/home/"+systemUser).CombinedOutput(); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not restore backup ownership")
			return
		}
		if _, err := newRestoreCommand(r.Context(), "restorecon", "-R", "/home/"+systemUser).CombinedOutput(); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not restore backup security context")
			return
		}
	}

	// Import the database dump without a shell or inherited panel secrets.
	dumpPath := filepath.Join(tmpDir, "dump.sql")
	dbName := systemUser + "_main"
	databaseImport := "not_present"
	if dumpInfo, err := os.Lstat(dumpPath); err == nil && dumpInfo.Mode().IsRegular() {
		dump, err := os.Open(dumpPath)
		if err != nil {
			databaseImport = "failed"
		} else {
			command := newRestoreCommand(r.Context(), "mysql", dbName)
			command.Stdin = dump
			_, commandErr := command.CombinedOutput()
			_ = dump.Close()
			if commandErr != nil {
				databaseImport = "failed"
			} else {
				databaseImport = "successful"
			}
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
