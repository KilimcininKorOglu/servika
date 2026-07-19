// Package passwordprotect manages directory-based .htpasswd authentication for nginx.
// Security relies on strict input validation, explicit exec arguments without a shell, and
// configuration rollback when vhost rendering fails, keeping customer sites operational.
package passwordprotect

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// Handlers provides HTTP handlers for directory password protection.
type Handlers struct {
	DB *sql.DB
}

const htpasswdDir = "/etc/nginx/htpasswd"

var (
	pathPattern = regexp.MustCompile(`^/[A-Za-z0-9._/-]{0,200}$`)
	reUser      = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)
)

// Record describes a user assigned to a protected directory.
type Record struct {
	ID        int64  `json:"id"`
	Path      string `json:"path"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

func (h *Handlers) domain(r *http.Request) (id int64, systemUser, version string, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, COALESCE(php_version,'8.3'), COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&systemUser, &version, &isDemo); err != nil {
		return id, "", "", false, false
	}
	return id, systemUser, version, isDemo == 1, true
}

// GET /domains/{id}/password-protection
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, path, username, created_at FROM protected_directories WHERE domain_id=? ORDER BY path, username`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list records")
		return
	}
	defer func() { _ = rows.Close() }()
	out := []Record{}
	for rows.Next() {
		var record Record
		if err := rows.Scan(&record.ID, &record.Path, &record.Username, &record.CreatedAt); err == nil {
			out = append(out, record)
		}
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listing failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// POST /domains/{id}/password-protection {path, username, password}
func (h *Handlers) Add(w http.ResponseWriter, r *http.Request) {
	id, systemUser, version, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "not available for demo subscriptions")
		return
	}
	if !strings.HasPrefix(systemUser, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user")
		return
	}
	var req struct {
		Path     string `json:"path"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	path := normalizePath(req.Path)
	if !pathPattern.MatchString(path) || strings.Contains(path, "..") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid path (example: /private)")
		return
	}
	if !reUser.MatchString(req.Username) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid username")
		return
	}
	if len(req.Password) < 4 || len(req.Password) > 128 {
		httpx.WriteError(w, http.StatusBadRequest, "password must contain 4 to 128 characters")
		return
	}
	if err := os.MkdirAll(htpasswdDir, 0o755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create htpasswd directory")
		return
	}
	file := htpasswdDir + "/d" + strconv.FormatInt(id, 10) + "_" + sanitize(path)
	flag := "-bB"
	if _, e := os.Stat(file); e != nil {
		flag = "-cbB" // Create a new file.
	}
	// Explicit arguments keep passwords and user names out of the shell.
	if _, err := exec.Command("htpasswd", flag, file, req.Username, req.Password).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	_ = exec.Command("restorecon", file).Run() // Apply the SELinux httpd_config_t context.
	_ = os.Chmod(file, 0o644)

	if _, err := h.DB.Exec(
		`INSERT INTO protected_directories (domain_id, path, username, htpasswd_file) VALUES (?,?,?,?)
		 ON DUPLICATE KEY UPDATE htpasswd_file=VALUES(htpasswd_file)`,
		id, path, req.Username, file); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not add record")
		return
	}

	if err := h.reRender(id, systemUser, version); err != nil {
		// Roll back the record and htpasswd entry when vhost validation fails, then render again.
		_, _ = h.DB.Exec(`DELETE FROM protected_directories WHERE domain_id=? AND path=? AND username=?`, id, path, req.Username)
		_ = exec.Command("htpasswd", "-D", file, req.Username).Run()
		if remaining := h.userCount(id, path); remaining == 0 {
			_ = os.Remove(file)
		}
		_ = h.reRender(id, systemUser, version)
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DELETE /domains/{id}/password-protection/{kid}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, systemUser, version, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	kid, _ := strconv.ParseInt(chi.URLParam(r, "kid"), 10, 64)
	var path, username, file string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT path, username, htpasswd_file FROM protected_directories WHERE id=? AND domain_id=?`, kid, id).
		Scan(&path, &username, &file); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "record not found")
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM protected_directories WHERE id=? AND domain_id=?`, kid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete record")
		return
	}
	_ = exec.Command("htpasswd", "-D", file, username).Run()
	if h.userCount(id, path) == 0 {
		_ = os.Remove(file) // Remove the location block when no user remains for this path.
	}
	if err := h.reRender(id, systemUser, version); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) userCount(id int64, path string) int {
	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM protected_directories WHERE domain_id=? AND path=?`, id, path).Scan(&n)
	return n
}

// reRender rebuilds the vhost and restores the backup when nginx validation fails.
func (h *Handlers) reRender(domainID int64, systemUser, version string) error {
	socket, err := provisioner.PHPSocketFor(systemUser, version)
	if err != nil {
		return fmt.Errorf("php socket: %w", err)
	}
	cfg := "/etc/nginx/conf.d/dom_" + systemUser + ".conf"
	backup, _ := os.ReadFile(cfg) // Nil when no backup exists.
	if err := provisioner.ApplyVhostForDomain(h.DB, domainID, socket, version); err != nil {
		if backup != nil {
			_ = os.WriteFile(cfg, backup, 0o644) // Restore the last known-good configuration.
			_ = exec.Command("nginx", "-t").Run()
		}
		return err
	}
	return nil
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	if path == "" {
		path = "/"
	}
	return path
}

var reNonAlnum = regexp.MustCompile(`[^A-Za-z0-9]+`)

func sanitize(s string) string {
	s = reNonAlnum.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		s = "root"
	}
	return s
}
