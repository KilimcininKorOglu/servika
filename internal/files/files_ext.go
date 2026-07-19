package files

// files_ext.go provides Write, Rename, Chmod, and a symlink-aware jail.
// The original jailJoin is in files.go; this file adds handlers and hardening.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"servika/internal/archivex"
	"servika/internal/httpx"
)

// jailJoinStrict is symlink-aware. It resolves the parent directory with EvalSymlinks,
// then joins the leaf and prevents escape through symlinks.
func jailJoinStrict(home, rel string) (string, error) {
	rel = filepath.Clean("/" + rel)
	wanted := filepath.Clean(filepath.Join(home, rel))

	// homeResolved
	homeResolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		homeResolved = home
	}

	// Find the resolvable portion of wanted.
	test := wanted
	for {
		if r, err := filepath.EvalSymlinks(test); err == nil {
			// The test path exists. Append and validate the remainder.
			rest := strings.TrimPrefix(wanted, test)
			full := filepath.Clean(filepath.Join(r, rest))
			if full == homeResolved || strings.HasPrefix(full, homeResolved+string(filepath.Separator)) {
				return full, nil
			}
			return "", errEscape
		}
		// Otherwise, ascend to the parent.
		parent := filepath.Dir(test)
		if parent == test {
			// The root has been reached.
			break
		}
		test = parent
	}
	// No ancestor resolved, which is rare. Fall back to a plain check.
	if wanted == homeResolved || strings.HasPrefix(wanted, homeResolved+string(filepath.Separator)) {
		return wanted, nil
	}
	return "", errEscape
}

// ----- Write (editor save) -----

type writeRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (h *Handlers) Write(w http.ResponseWriter, r *http.Request) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	var req writeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Content) > 5*1024*1024 {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "files over 5 MB cannot be saved with the editor")
		return
	}
	// Preserve permissions when the file already exists.
	mode := uint32(0644)
	if f, err := openAt2Beneath(home, req.Path, 0, 0); err == nil {
		if st, err2 := f.Stat(); err2 == nil {
			mode = uint32(st.Mode().Perm())
		}
		_ = f.Close()
	}
	if err := writeBeneath(home, req.Path, []byte(req.Content), mode, systemUser); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"path": req.Path,
		"size": len(req.Content),
	})
}

// ----- Rename / Move -----

type renameReq struct {
	Old string `json:"old"`
	New string `json:"new"`
}

func (h *Handlers) Rename(w http.ResponseWriter, r *http.Request) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	var req renameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Verify source exists under home (read-only check; the actual rename is symlink-safe).
	_, err = jailJoinStrict(home, req.Old)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if _, err := jailJoinStrict(home, req.New); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Old == "/" || req.New == "/" {
		httpx.WriteError(w, http.StatusBadRequest, "the home directory cannot be moved")
		return
	}
	// Ensure the target parent directory exists (symlink-safe).
	_ = mkdirAllBeneath(home, filepath.Dir(req.New), systemUser)
	if err := renameBeneath(home, req.Old, req.New, systemUser); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "old": req.Old, "new": req.New})
}

// ----- Chmod -----

type chmodReq struct {
	Path string `json:"path"`
	Mode string `json:"mode"` // Octal string such as "0644".
}

func (h *Handlers) Chmod(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	var req chmodReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mod := strings.TrimPrefix(req.Mode, "0")
	n, err := strconv.ParseUint(mod, 8, 32)
	if err != nil || n > 0o777 {
		httpx.WriteError(w, http.StatusBadRequest, "mode must be octal (0000-0777)")
		return
	}
	if err := chmodBeneath(home, req.Path, uint32(n)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "path": req.Path, "mode": req.Mode})
}

// ----- Extract ZIP, TAR, RAR, or compressed TAR -----

type extractReq struct {
	Path   string `json:"path"`   // Archive path.
	Target string `json:"target"` // Optional extraction directory. Defaults to the archive directory.
}

func newFileCommand(ctx context.Context, name string, arguments ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	return command
}

func (h *Handlers) Extract(w http.ResponseWriter, r *http.Request) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	var req extractReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	abs, err := jailJoinStrict(home, req.Path)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() {
		httpx.WriteError(w, http.StatusBadRequest, "file not found or path is not a regular file")
		return
	}

	target := req.Target
	if target == "" {
		target = filepath.Dir(req.Path)
	}
	targetAbs, err := jailJoinStrict(home, target)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := os.MkdirAll(targetAbs, 0755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	if _, err := newFileCommand(r.Context(), "chown", systemUser+":"+systemUser, targetAbs).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}

	lowerPath := strings.ToLower(abs)
	if strings.HasSuffix(lowerPath, ".gz") && archivex.DetectType(lowerPath) == archivex.TypeUnknown {
		gzipRelative := filepath.Join(target, strings.TrimSuffix(filepath.Base(abs), ".gz"))
		gzipTarget, err := jailJoinStrict(home, gzipRelative)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid extraction target")
			return
		}
		gzipOutput, err := os.OpenFile(gzipTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
			return
		}
		command := newFileCommand(r.Context(), "gunzip", "-k", "-c", abs)
		command.Stdout = gzipOutput
		runErr := command.Run()
		closeErr := gzipOutput.Close()
		if runErr != nil || closeErr != nil {
			_ = os.Remove(gzipTarget)
			httpx.WriteError(w, http.StatusBadRequest, "invalid gzip file")
			return
		}
	} else {
		if archivex.DetectType(lowerPath) == archivex.TypeUnknown {
			httpx.WriteError(w, http.StatusBadRequest, "unsupported format (zip, rar, tar, tar.gz/tgz, tar.bz2, tar.xz, gz)")
			return
		}
		if _, err := archivex.Extract(r.Context(), abs, targetAbs, systemUser); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid archive")
			return
		}
	}

	if _, err := newFileCommand(r.Context(), "chown", "-R", systemUser+":"+systemUser, targetAbs).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	if _, err := newFileCommand(r.Context(), "restorecon", "-R", targetAbs).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"path":   req.Path,
		"target": target,
	})
}

// ----- Bulk copy and move -----

type bulkMoveCopyReq struct {
	Sources []string `json:"sources"`
	Target  string   `json:"target"` // Target folder that receives the sources.
}

func (h *Handlers) Copy(w http.ResponseWriter, r *http.Request) {
	h.bulkMoveCopy(w, r, false)
}

func (h *Handlers) Move(w http.ResponseWriter, r *http.Request) {
	h.bulkMoveCopy(w, r, true)
}

func (h *Handlers) bulkMoveCopy(w http.ResponseWriter, r *http.Request, move bool) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	var req bulkMoveCopyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Verify the target is a directory under home (symlink-safe).
	targetDir, err := isDirBeneath(home, req.Target)
	if err != nil || !targetDir {
		httpx.WriteError(w, http.StatusBadRequest, "target is not a directory")
		return
	}

	successful := 0
	errorsList := []string{}
	for _, source := range req.Sources {
		destination := filepath.Join(req.Target, filepath.Base(source))
		if destination == source {
			errorsList = append(errorsList, source+": source and target are identical")
			continue
		}
		var operationErr error
		if move {
			operationErr = renameBeneath(home, source, destination, systemUser)
		} else {
			operationErr = copyTreeBeneath(home, source, destination, systemUser)
		}
		if operationErr != nil {
			errorsList = append(errorsList, source+": operation failed")
			continue
		}
		successful++
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": len(errorsList) == 0, "successful": successful, "errors": errorsList,
	})
}


// ----- Archive selected files -----

type archiveReq struct {
	Resources  []string `json:"resources"`
	OutputPath string   `json:"output_path"` // Example: /public_html/backup.zip.
	Format     string   `json:"format"`      // zip | tar.gz
}

func (h *Handlers) Archive(w http.ResponseWriter, r *http.Request) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	var req archiveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Resources) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "source missing")
		return
	}
	if req.Format == "" {
		req.Format = "zip"
	}
	outputAbs, err := jailJoinStrict(home, req.OutputPath)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	_ = os.MkdirAll(filepath.Dir(outputAbs), 0755)

	// Resolve all resources beneath home and archive them with relative names.
	// Find their common parent and run from there.
	var args []string
	if req.Format == "zip" {
		args = []string{"-r", "-q", outputAbs}
		for _, k := range req.Resources {
			kAbs, err := jailJoinStrict(home, k)
			if err != nil {
				continue
			}
			// Change directory and use a relative name.
			args = append(args, kAbs)
		}
		_, err := newFileCommand(r.Context(), "zip", args...).CombinedOutput()
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
			return
		}
	} else { // tar.gz
		args = []string{"-czf", outputAbs}
		for _, k := range req.Resources {
			kAbs, err := jailJoinStrict(home, k)
			if err != nil {
				continue
			}
			args = append(args, kAbs)
		}
		_, err := newFileCommand(r.Context(), "tar", args...).CombinedOutput()
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
			return
		}
	}
	_, _ = newFileCommand(r.Context(), "chown", systemUser+":"+systemUser, outputAbs).CombinedOutput()
	_, _ = newFileCommand(r.Context(), "restorecon", outputAbs).CombinedOutput()

	info, _ := os.Stat(outputAbs)
	var size int64
	if info != nil {
		size = info.Size()
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "output_path": req.OutputPath, "size": size,
	})
}

// ----- New empty file -----

type newFileRequest struct {
	Path string `json:"path"`
}

func (h *Handlers) NewFile(w http.ResponseWriter, r *http.Request) {
	home, systemUser, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	var req newFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := createExclBeneath(home, req.Path, systemUser); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "path": req.Path})
}

// ----- Calculate size with du -sb -----

func (h *Handlers) CalculateSize(w http.ResponseWriter, r *http.Request) {
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
	out, err := newFileCommand(r.Context(), "du", "-sb", abs).CombinedOutput()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	parts := strings.Fields(string(out))
	if len(parts) < 1 {
		httpx.WriteError(w, http.StatusInternalServerError, "could not parse du output")
		return
	}
	var b int64
	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			break
		}
		b = b*10 + int64(c-'0')
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"path":   rel,
		"size_b": b,
	})
}

// ----- Recursive search by name pattern -----

func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	home, _, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), "operation failed")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"content": []any{}, "total": 0})
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		rel = "/"
	}
	baseAbs, err := jailJoinStrict(home, rel)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Security: q is only a file-name pattern. Use iname without a shell to prevent injection.
	q = strings.ReplaceAll(q, "*", "")
	q = strings.ReplaceAll(q, "?", "")
	pattern := "*" + q + "*"

	out, _ := newFileCommand(r.Context(), "find", baseAbs, "-iname", pattern, "-printf", "%p\t%s\t%y\t%T@\n").Output()
	results := []Entry{}
	for _, ln := range strings.Split(string(out), "\n") {
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		absp := parts[0]
		size := int64(0)
		for _, c := range parts[1] {
			if c < '0' || c > '9' {
				break
			}
			size = size*10 + int64(c-'0')
		}
		ftype := "file"
		switch parts[2] {
		case "d":
			ftype = "folder"
		case "l":
			ftype = "symlink"
		}
		// Make the path relative to home.
		relativePath := strings.TrimPrefix(absp, home)
		if relativePath == "" {
			relativePath = "/"
		}
		info, _ := os.Lstat(absp)
		mode, permissions, owner, group := "", "", "", ""
		var changedAt string
		if info != nil {
			mode, permissions, owner, group = fileMetadata(info)
			changedAt = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
		}
		results = append(results, Entry{
			Name: filepath.Base(absp), Path: filepath.ToSlash(relativePath),
			Type: ftype, SizeBytes: size, Mode: mode, Permissions: permissions,
			Owner: owner, Group: group, Changed: changedAt,
		})
		if len(results) >= 500 {
			break
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"content": results, "total": len(results), "q": q,
	})
}
