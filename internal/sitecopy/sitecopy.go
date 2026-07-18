// Package sitecopy clones site files into timestamped staging copies under the same home directory.
// Copies are stored as public_html snapshots under ~/copies/copy_<ts>/ before changes and exclude databases.
// Security constraints prevent cross-user access, bind mounts, and fuser use. Deletion is restricted
// to /home/c_*/copies/ with both a name regular expression and a path-prefix guard.
package sitecopy

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handlers struct{ DB *sql.DB }

const maxCopyBytes = 3 * 1024 * 1024 * 1024 // Direct sites over 3 GB to the Backups feature.

var copyNamePattern = regexp.MustCompile(`^copy_[0-9]{8}_[0-9]{6}$`)

type Copy struct {
	Name   string `json:"name"`
	SizeMB int64  `json:"size_mb"`
	Date   string `json:"date"`
}

func (h *Handlers) domain(r *http.Request) (id int64, systemUser string, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, COALESCE(is_demo,0) FROM domains WHERE id=?`, id).Scan(&systemUser, &isDemo); err != nil {
		return id, "", false, false
	}
	return id, systemUser, isDemo == 1, true
}

// GET /domains/{id}/copy
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	_, systemUser, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	dir := "/home/" + systemUser + "/copies"
	out := []Copy{}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || !copyNamePattern.MatchString(e.Name()) {
				continue
			}
			copyItem := Copy{Name: e.Name()}
			if fi, err := e.Info(); err == nil {
				copyItem.Date = fi.ModTime().Format("2006-01-02 15:04")
			}
			copyItem.SizeMB = dirSizeMB(filepath.Join(dir, e.Name()))
			out = append(out, copyItem)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name }) // Sort newest first.
	httpx.WriteJSON(w, http.StatusOK, out)
}

// POST /domains/{id}/copy creates a staging copy.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	_, systemUser, demo, ok := h.domain(r)
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
	home := "/home/" + systemUser
	source := home + "/public_html"
	if _, err := os.Stat(source); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "public_html not found")
		return
	}
	if b := dirSizeBytes(source); b > maxCopyBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "site exceeds 3 GB; use the Backups tool")
		return
	}
	copyDir := home + "/copies"
	if err := os.MkdirAll(copyDir, 0o711); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create copies directory")
		return
	}
	_ = exec.Command("chown", systemUser+":"+systemUser, copyDir).Run() // Assign the copies directory to the customer.
	name := "copy_" + time.Now().Format("20060102_150405")
	target := copyDir + "/" + name + "/public_html"
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := os.MkdirAll(target, 0o755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create target")
		return
	}
	// The trailing slash makes rsync copy directory contents. Omit --delete to keep the operation non-destructive.
	if _, err := exec.CommandContext(ctx, "rsync", "-a", "--no-owner", "--no-group", source+"/", target+"/").CombinedOutput(); err != nil {
		_ = os.RemoveAll(copyDir + "/" + name)
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	// Assign ownership to the domain user.
	_ = exec.Command("chown", "-R", systemUser+":"+systemUser, copyDir+"/"+name).Run()
	_ = exec.Command("restorecon", "-R", copyDir+"/"+name).Run()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "size_mb": dirSizeMB(copyDir + "/" + name)})
}

// DELETE /domains/{id}/copy/{name}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	_, systemUser, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if !strings.HasPrefix(systemUser, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user")
		return
	}
	name := chi.URLParam(r, "name")
	if !copyNamePattern.MatchString(name) { // Strict name validation prevents path traversal.
		httpx.WriteError(w, http.StatusBadRequest, "invalid copy name")
		return
	}
	copyDir := "/home/" + systemUser + "/copies"
	target := filepath.Join(copyDir, name)
	// Layered guard verifies the prefix, cleaned path, and directory type.
	clean := filepath.Clean(target)
	if !strings.HasPrefix(clean, copyDir+"/") {
		httpx.WriteError(w, http.StatusBadRequest, "path is outside the copies directory")
		return
	}
	fi, err := os.Stat(clean)
	if err != nil || !fi.IsDir() {
		httpx.WriteError(w, http.StatusNotFound, "copy not found")
		return
	}
	if err := os.RemoveAll(clean); err != nil { // Remove only a regular directory without bind mounts or fuser.
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete record")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func dirSizeBytes(p string) int64 {
	out, err := exec.Command("du", "-sb", p).Output()
	if err != nil {
		return 0
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return 0
	}
	n, _ := strconv.ParseInt(f[0], 10, 64)
	return n
}

func dirSizeMB(p string) int64 {
	return dirSizeBytes(p) / (1024 * 1024)
}
