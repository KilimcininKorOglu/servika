package transfers

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"servika/internal/domains"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const MaxUploadBytes = int64(20 << 30)

// commandPath mirrors the restore path; transfer subprocesses run with an
// explicit, minimal environment so panel secrets from os.Environ() are never
// inherited by mysql/tar/find.
const commandPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

func newTransferCommand(ctx context.Context, name string, arguments ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Env = []string{"PATH=" + commandPath, "HOME=/root"}
	return command
}

type Handlers struct {
	DB      *sql.DB
	Domains *domains.Handlers
}

// Analyze accepts a cPanel full backup and returns an inventory. It never
// extracts or persists archive contents.
func (h *Handlers) Analyze(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read the upload or the size limit was exceeded")
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	f, hdr, err := r.FormFile("archive")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "a cPanel .tar.gz backup is required in the archive field")
		return
	}
	defer func() { _ = f.Close() }()
	low := strings.ToLower(hdr.Filename)
	if !strings.HasSuffix(low, ".tar.gz") && !strings.HasSuffix(low, ".tgz") {
		httpx.WriteError(w, http.StatusBadRequest, "the first release only supports cPanel .tar.gz/.tgz full backups")
		return
	}
	inv, err := AnalyzeCPanel(f)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrArchiveTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		httpx.WriteError(w, status, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inv)
}

type importResponse struct {
	OK         bool      `json:"ok"`
	DomainID   int64     `json:"domain_id"`
	Domain     string    `json:"domain"`
	SystemUser string    `json:"system_user"`
	WebFiles   int       `json:"web_files"`
	Database   string    `json:"database,omitempty"`
	Skipped    []string  `json:"skipped"`
	Source     Inventory `json:"source"`
}

// Import creates a new Servika domain and restores the web root plus a single
// cPanel database. Unsupported multi-database accounts are rejected before
// provisioning, so the operation cannot silently lose data.
func (h *Handlers) Import(w http.ResponseWriter, r *http.Request) {
	if h.Domains == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain provider is not ready")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read the upload or the size limit was exceeded")
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	f, _, err := r.FormFile("archive")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "the archive field is required")
		return
	}
	defer func() { _ = f.Close() }()

	tmp, err := os.CreateTemp("", "servika-cpanel-*.tar.gz")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create a temporary archive")
		return
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := io.Copy(tmp, f); err != nil || tmp.Close() != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not save the archive")
		return
	}
	src, err := os.Open(tmpPath)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not open the archive")
		return
	}
	inv, err := AnalyzeCPanel(src)
	_ = src.Close()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(inv.Databases) > 1 {
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"this account has more than one database; lossless multi-database conversion is not supported yet")
		return
	}

	created, ok := h.provisionDomain(w, r, inv)
	if !ok {
		return
	}
	committed := false
	defer func() {
		if !committed {
			h.rollbackDomain(r, created.ID)
		}
	}()

	if err := h.restoreWeb(r.Context(), tmpPath, inv.ArchiveRoot, created.SystemUser); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not transfer web files: "+err.Error())
		return
	}
	if len(inv.Databases) == 1 {
		if err := h.restoreDatabase(r.Context(), tmpPath, inv.ArchiveRoot, inv.Databases[0], created.DBName); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not transfer the database: "+err.Error())
			return
		}
	}
	committed = true
	httpx.WriteJSON(w, http.StatusCreated, importResponse{
		OK: true, DomainID: created.ID, Domain: created.DomainName,
		SystemUser: created.SystemUser, WebFiles: inv.WebFiles,
		Database: created.DBName, Source: inv,
		Skipped: []string{"Email mailboxes, cron jobs and source SSL certificates were only inventoried in this first release."},
	})
}

type createdDomain struct {
	ID         int64  `json:"id"`
	DomainName string `json:"domain_name"`
	SystemUser string `json:"system_user"`
	DBName     string `json:"db_name"`
}

// provisionDomain drives domains.Create in-process so the full provisioning
// side effects (system user, vhost, FTP, database, php-fpm pool) are reused.
// On any non-201 it copies domains.Create's own error response to the client
// and returns ok=false.
func (h *Handlers) provisionDomain(w http.ResponseWriter, r *http.Request, inv Inventory) (createdDomain, bool) {
	domain := strings.ToLower(strings.TrimSpace(r.FormValue("domain")))
	if domain == "" {
		domain = inv.PrimaryDomain
	}
	if domain == "" {
		httpx.WriteError(w, http.StatusBadRequest, "the primary domain could not be determined")
		return createdDomain{}, false
	}
	customerID, err := requiredInt64(r.FormValue("customer_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "customer_id is required")
		return createdDomain{}, false
	}
	phpVersion := strings.TrimSpace(r.FormValue("php_version"))
	if phpVersion == "" {
		phpVersion = "8.3"
	}
	var planID *int64
	if s := strings.TrimSpace(r.FormValue("plan_id")); s != "" {
		v, e := requiredInt64(s)
		if e != nil {
			httpx.WriteError(w, http.StatusBadRequest, "plan_id is invalid")
			return createdDomain{}, false
		}
		planID = &v
	}

	createBody, _ := json.Marshal(map[string]any{
		"domain_name": domain, "php_version": phpVersion,
		"customer_id": customerID, "plan_id": planID,
	})
	cr := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(createBody)).
		WithContext(r.Context())
	cr.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	h.Domains.Create(cw, cr)
	if cw.Code != http.StatusCreated {
		copyRecorded(w, cw)
		return createdDomain{}, false
	}
	var created createdDomain
	if err := json.Unmarshal(cw.Body.Bytes(), &created); err != nil || created.ID <= 0 {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read the created domain response")
		return createdDomain{}, false
	}
	return created, true
}

func requiredInt64(s string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("invalid number")
	}
	return v, nil
}

func copyRecorded(w http.ResponseWriter, rr *httptest.ResponseRecorder) {
	for k, values := range rr.Header() {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rr.Code)
	_, _ = w.Write(rr.Body.Bytes())
}

func (h *Handlers) rollbackDomain(r *http.Request, id int64) {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", strconv.FormatInt(id, 10))
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+strconv.FormatInt(id, 10), nil).
		WithContext(ctx)
	h.Domains.Delete(httptest.NewRecorder(), req)
}

// restoreWeb extracts the archive's public_html subtree into the freshly
// provisioned tenant home as root, then fixes ownership and SELinux context —
// the same canonical pattern backups.Restore uses. Subprocesses run with a
// minimal environment (no inherited panel secrets).
func (h *Handlers) restoreWeb(ctx context.Context, archivePath, root, sk string) error {
	if !strings.HasPrefix(sk, "c_") || root == "" {
		return errors.New("unsafe target")
	}
	target := "/home/" + sk + "/public_html"
	if out, err := newTransferCommand(ctx, "find", target, "-mindepth", "1", "-delete").CombinedOutput(); err != nil {
		return fmt.Errorf("clearing target: %s", strings.TrimSpace(string(out)))
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	member := root + "/homedir/public_html"
	cmd := newTransferCommand(ctx, "tar", "-xz", "-f", "-", "-C", target, "--strip-components=3", member)
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %s", strings.TrimSpace(string(out)))
	}
	if out, err := newTransferCommand(ctx, "chown", "-R", sk+":"+sk, target).CombinedOutput(); err != nil {
		return fmt.Errorf("chown: %s", strings.TrimSpace(string(out)))
	}
	_, _ = newTransferCommand(ctx, "restorecon", "-RF", target).CombinedOutput()
	return nil
}

// restoreDatabase streams the single cPanel SQL dump into the target database,
// dropping CREATE DATABASE / USE statements so the dump lands in Servika's own
// database rather than the source's. mysql runs with a minimal environment.
func (h *Handlers) restoreDatabase(ctx context.Context, archivePath, root, sourceDB, targetDB string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	want := root + "/mysql/" + sourceDB + ".sql"
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return errors.New("the SQL dump was not found in the archive")
		}
		if err != nil {
			return err
		}
		if path.Clean(hdr.Name) != want {
			continue
		}
		return pipeDumpToMySQL(ctx, tr, targetDB)
	}
}

func pipeDumpToMySQL(ctx context.Context, dump io.Reader, targetDB string) error {
	cmd := newTransferCommand(ctx, "mysql", targetDB)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	bw := bufio.NewWriter(stdin)
	br := bufio.NewReader(dump)
	for {
		line, readErr := br.ReadString('\n')
		upper := strings.ToUpper(strings.TrimSpace(line))
		if !strings.HasPrefix(upper, "CREATE DATABASE ") && !strings.HasPrefix(upper, "USE ") {
			if _, err := bw.WriteString(line); err != nil {
				_ = stdin.Close()
				_ = cmd.Wait()
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = stdin.Close()
			_ = cmd.Wait()
			return readErr
		}
	}
	_ = bw.Flush()
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("mysql: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}
