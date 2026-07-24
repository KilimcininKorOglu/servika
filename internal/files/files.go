// Package files provides a chrooted file manager API for domain home directories.
package files

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const (
	// MaxUploadBytes is the maximum accepted multipart request body size.
	MaxUploadBytes     = 10 * 1024 * 1024 * 1024
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
	errDemo       = errors.New("files cannot be managed for a demo subscription")
	errBadUser    = errors.New("security: invalid system user")
	errEscape     = errors.New("security: escape from home directory blocked")
	errNotRegular = errors.New("not a regular file")
	errTooLarge   = errors.New("file exceeds the size limit")
)

type Entry struct {
	Name        string `json:"name"`
	Path        string `json:"path"` // Relative to home for the panel UI.
	Type        string `json:"type"` // "folder" | "file" | "symlink"
	SizeBytes   int64  `json:"size_b"`
	Mode        string `json:"mode"`        // "0644"
	Permissions string `json:"permissions"` // "-rw-r--r--"
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	Changed     string `json:"changed"` // RFC3339
}

func fileMetadata(info os.FileInfo) (mode, permissions, owner, group string) {
	mode = "0" + strconv.FormatInt(int64(info.Mode().Perm()), 8)
	permissions = info.Mode().String()
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return mode, permissions, "", ""
	}

	owner = strconv.FormatUint(uint64(stat.Uid), 10)
	if account, err := user.LookupId(owner); err == nil {
		owner = account.Username
	}
	group = strconv.FormatUint(uint64(stat.Gid), 10)
	if accountGroup, err := user.LookupGroupId(group); err == nil {
		group = accountGroup.Name
	}
	return mode, permissions, owner, group
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), messageFromErr(err))
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		rel = "/"
	}
	// Symlink-safe listing: openat2 rejects any symlink component, so a tenant cannot
	// race a directory into a symlink between validation and the root-side read.
	dir, err := readDirBeneath(home, rel)
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
		mode, permissions, owner, group := fileMetadata(info)
		out = append(out, Entry{
			Name:        e.Name(),
			Path:        filepath.ToSlash(filepath.Join(rel, e.Name())),
			Type:        ftype,
			SizeBytes:   info.Size(),
			Mode:        mode,
			Permissions: permissions,
			Owner:       owner,
			Group:       group,
			Changed:     info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
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
		httpx.WriteError(w, statusFromErr(err), messageFromErr(err))
		return
	}
	rel := r.URL.Query().Get("path")
	// Symlink-safe open: the fd is resolved beneath home following no symlinks, so a
	// tenant cannot race the path into a symlink and have root read a host file.
	f, err := openReadBeneath(home, rel)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	if info.IsDir() {
		httpx.WriteError(w, http.StatusBadRequest, "directories cannot be downloaded")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+info.Name()+"\"")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	_, _ = io.Copy(w, f)
}

// Read returns text file content for the editor.
func (h *Handlers) Read(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), messageFromErr(err))
		return
	}
	rel := r.URL.Query().Get("path")
	// Symlink-safe read with an inline 2 MB cap (single fd, no separate racy stat).
	const maxEditBytes = 2 * 1024 * 1024
	data, info, err := readFileBeneath(home, rel, maxEditBytes)
	if errors.Is(err, errTooLarge) {
		httpx.WriteError(w, http.StatusBadRequest, "file exceeds 2 MB and cannot be edited")
		return
	}
	if errors.Is(err, errNotRegular) {
		httpx.WriteError(w, http.StatusBadRequest, "not a regular file")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
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
		httpx.WriteError(w, statusFromErr(err), messageFromErr(err))
		return
	}
	var req mkdirReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := mkdirAllBeneath(home, req.Path, systemUser); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "path": req.Path})
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), messageFromErr(err))
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" || rel == "/" {
		httpx.WriteError(w, http.StatusBadRequest, "the home directory cannot be deleted")
		return
	}
	if err := removeAllBeneath(home, rel); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": rel})
}

func (h *Handlers) Upload(w http.ResponseWriter, r *http.Request) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), messageFromErr(err))
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
	defer func() { _ = file.Close() }()
	if fh.Size > MaxUploadBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "file is too large (maximum 10 GiB)")
		return
	}
	uploadPath := filepath.Join(rel, fh.Filename)
	written, err := copyStreamBeneath(home, uploadPath, file, systemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":   true,
		"path": filepath.ToSlash(uploadPath),
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

// messageFromErr returns a human-readable error message for filesystem-related
// sentinel errors, or a generic fallback for unknown errors.
func messageFromErr(err error) string {
	switch err {
	case os.ErrNotExist:
		return "not found"
	case errDemo:
		return "not available for demo subscriptions"
	case errBadUser, errEscape:
		return "invalid path"
	}
	return "operation failed"
}
