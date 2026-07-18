// Package files provides a chrooted file manager API for domain home directories.
package files

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const (
	// MaxUploadBytes is the maximum accepted multipart request body size.
	MaxUploadBytes     = 2 * 1024 * 1024 * 1024
	maxMultipartMemory = 32 * 1024 * 1024
)

var errUploadTooLarge = errors.New("upload exceeds the size limit")

var managedSystemUserPattern = regexp.MustCompile(`^c_[A-Za-z0-9_]+$`)

type Handlers struct {
	DB *sql.DB
}

// home resolves a domain ID to /home/c_<user>.
func (h *Handlers) home(r *http.Request) (string, string, error) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var systemUser string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, is_demo FROM domains WHERE id=?`, id).
		Scan(&systemUser, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", os.ErrNotExist
	}
	if err != nil {
		return "", "", err
	}
	if isDemo == 1 {
		return "", "", errDemo
	}
	if !managedSystemUserPattern.MatchString(systemUser) {
		return "", "", errBadUser
	}
	return "/home/" + systemUser, systemUser, nil
}

var (
	errDemo    = errors.New("files cannot be managed for a demo subscription")
	errBadUser = errors.New("security: invalid system user")
	errEscape  = errors.New("security: escape from home directory blocked")
)

type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"` // Relative to home for the panel UI.
	Type      string `json:"type"` // "folder" | "file" | "symlink"
	SizeBytes int64  `json:"size_b"`
	Mode      string `json:"mode"`    // "0644"
	Changed   string `json:"changed"` // RFC3339
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		rel = "/"
	}
	abs, err := jailJoinStrict(home, rel)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	dir, err := os.ReadDir(abs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	out := make([]Entry, 0, len(dir))
	for _, e := range dir {
		info, err := e.Info()
		if err != nil {
			continue
		}
		ftype := "file"
		if info.IsDir() {
			ftype = "folder"
		} else if info.Mode()&os.ModeSymlink != 0 {
			ftype = "symlink"
		}
		out = append(out, Entry{
			Name:      e.Name(),
			Path:      filepath.ToSlash(filepath.Join(rel, e.Name())),
			Type:      ftype,
			SizeBytes: info.Size(),
			Mode:      "0" + strconv.FormatInt(int64(info.Mode().Perm()), 8),
			Changed:   info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	// Sort folders first, then alphabetically.
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Type == "folder") != (out[j].Type == "folder") {
			return out[i].Type == "folder"
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"path":    filepath.ToSlash(rel),
		"content": out,
		"total":   len(out),
	})
}

// Download returns raw file content.
func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	rel := r.URL.Query().Get("path")
	abs, err := jailJoinStrict(home, rel)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if info.IsDir() {
		httpx.WriteError(w, http.StatusBadRequest, "directories cannot be downloaded")
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+info.Name()+"\"")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	_, _ = io.Copy(w, f)
}

// Read returns text file content for the editor.
func (h *Handlers) Read(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	rel := r.URL.Query().Get("path")
	abs, err := jailJoinStrict(home, rel)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if info.Size() > 2*1024*1024 {
		httpx.WriteError(w, http.StatusBadRequest, "file exceeds 2 MB and cannot be edited")
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"path":    rel,
		"content": string(data),
		"size":    info.Size(),
	})
}

type mkdirReq struct {
	Path string `json:"path"`
}

func (h *Handlers) Mkdir(w http.ResponseWriter, r *http.Request) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	var req mkdirReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	abs, err := jailJoinStrict(home, req.Path)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	chown(abs, systemUser)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "path": req.Path})
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	rel := r.URL.Query().Get("path")
	abs, err := jailJoinStrict(home, rel)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if abs == home {
		httpx.WriteError(w, http.StatusBadRequest, "the home directory cannot be deleted")
		return
	}
	if err := os.RemoveAll(abs); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": rel})
}

func (h *Handlers) Upload(w http.ResponseWriter, r *http.Request) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		rel = "/"
	}
	if err := parseMultipartUpload(w, r, MaxUploadBytes, maxMultipartMemory); err != nil {
		if errors.Is(err, errUploadTooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds the 2 GiB limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	file, fh, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	defer file.Close()
	if fh.Size > MaxUploadBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "file is too large (maximum 2 GiB)")
		return
	}
	abs, err := jailJoinStrict(home, filepath.Join(rel, fh.Filename))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	out, err := os.Create(abs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	defer out.Close()
	written, err := io.Copy(out, file)
	if err != nil {
		_ = os.Remove(abs)
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	chown(abs, systemUser)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":   true,
		"path": filepath.ToSlash(filepath.Join(rel, fh.Filename)),
		"size": written,
		"name": fh.Filename,
	})
}

func parseMultipartUpload(w http.ResponseWriter, r *http.Request, maxBytes int64, maxMemory int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return errUploadTooLarge
		}
		return err
	}
	return nil
}

func statusFromErr(err error) int {
	switch err {
	case os.ErrNotExist:
		return http.StatusNotFound
	case errDemo:
		return http.StatusForbidden
	case errBadUser, errEscape:
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// chown restores ownership and the SELinux context for a managed path.
func chown(path, systemUser string) {
	if uu, err := userLookup(systemUser); err == nil {
		_ = osChown(path, uu.UID, uu.GID)
	}
	_, _ = exec.Command("restorecon", path).CombinedOutput()
}
