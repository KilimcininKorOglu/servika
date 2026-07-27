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

	"servika/internal/credentials"
	"servika/internal/cron"
	"servika/internal/domains"
	"servika/internal/httpx"
	"servika/internal/mail"

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
	Mail    *mail.Handlers
	Cron    *cron.Handlers
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
	OK         bool             `json:"ok"`
	DomainID   int64            `json:"domain_id"`
	Domain     string           `json:"domain"`
	SystemUser string           `json:"system_user"`
	WebFiles   int              `json:"web_files"`
	Databases  []DBMap          `json:"databases"`
	Mailboxes  []MailCredential `json:"mailboxes"`
	Aliases    int              `json:"aliases"`
	CronJobs   int              `json:"cron_jobs"`
	Skipped    []string         `json:"skipped"`
	Source     Inventory        `json:"source"`
}

// MailCredential carries a newly provisioned mailbox address and its one-time
// password back to the client; the source cPanel password hash is never reused.
type MailCredential struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type DBMap struct {
	Source string `json:"source"`
	Target string `json:"target"`
	User   string `json:"user"`
}

// Import creates a new Servika domain and restores the web root plus the
// cPanel databases. Additional databases share the domain's default DB user,
// matching Servika's supported one-user-to-many-databases model.
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
	dbMaps := databaseMappings(inv.Databases, created.SystemUser, created.DBName, created.DBUser)
	for i, m := range dbMaps {
		// The first database reuses the domain's default DB (created by
		// domains.Create); each additional one is created and attached to the
		// same DB user, so rollback via domains.Delete drops them all.
		if i > 0 {
			if err := credentials.MySQLCreateDBForUser(h.DB, created.ID, m.Target, created.DBUser); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "could not create the additional database: "+err.Error())
				return
			}
		}
		if err := h.restoreDatabase(r.Context(), tmpPath, inv.ArchiveRoot, m.Source, m.Target); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not transfer the database: "+err.Error())
			return
		}
	}
	mailCreds, aliasCount, err := h.importMail(r, tmpPath, inv, created.ID, created.DomainName, created.SystemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not transfer email: "+err.Error())
		return
	}
	cronCount, err := h.importCron(r, inv, created.ID, created.SystemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not transfer cron jobs: "+err.Error())
		return
	}
	committed = true
	httpx.WriteJSON(w, http.StatusCreated, importResponse{
		OK: true, DomainID: created.ID, Domain: created.DomainName,
		SystemUser: created.SystemUser, WebFiles: inv.WebFiles,
		Databases: dbMaps, Mailboxes: mailCreds, Aliases: aliasCount, CronJobs: cronCount, Source: inv,
		Skipped: []string{"Source SSL certificates were only inventoried in this release."},
	})
}

// importCron recreates each supported cPanel cron job through the panel's own
// create path, rewriting the source home prefix onto the target tenant's home.
// It runs after the web/database/mail restore so a failure still rolls the whole
// domain back via the caller's deferred rollbackDomain.
func (h *Handlers) importCron(r *http.Request, inv Inventory, domainID int64, targetUser string) (int, error) {
	if len(inv.CronJobs) == 0 {
		return 0, nil
	}
	if h.Cron == nil {
		return 0, errors.New("cron provider is not ready")
	}
	created := 0
	for _, job := range inv.CronJobs {
		command := job.Command
		if inv.Username != "" {
			command = strings.ReplaceAll(command, "/home/"+inv.Username+"/", "/home/"+targetUser+"/")
		}
		body, _ := json.Marshal(map[string]string{
			"minute": job.Minute, "hour": job.Hour, "day": job.Day,
			"month": job.Month, "week": job.Weekday,
			"command": command, "comment": job.Comment,
		})
		req := domainRequest(r, http.MethodPost, "/cron", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Cron.Create(rr, req)
		if rr.Code != http.StatusCreated {
			return 0, fmt.Errorf("job %d: %s", created+1, strings.TrimSpace(rr.Body.String()))
		}
		created++
	}
	return created, nil
}

type createdDomain struct {
	ID         int64  `json:"id"`
	DomainName string `json:"domain_name"`
	SystemUser string `json:"system_user"`
	DBName     string `json:"db_name"`
	DBUser     string `json:"db_user"`
}

// databaseMappings maps each source cPanel database name to a Servika target.
// The first source reuses the domain's default database; each additional source
// is namespaced as "<system_user>_<sanitized-suffix>", truncated to MySQL's
// 64-char identifier limit and de-duplicated with a numeric tail.
func databaseMappings(sources []string, sk, defaultDB, dbUser string) []DBMap {
	out := make([]DBMap, 0, len(sources))
	used := map[string]bool{defaultDB: true}
	for i, source := range sources {
		target := defaultDB
		if i > 0 {
			suffix := dbSuffix(source)
			maxSuffix := 64 - len(sk) - 1
			if maxSuffix < 1 {
				maxSuffix = 1
			}
			if len(suffix) > maxSuffix {
				suffix = suffix[:maxSuffix]
			}
			target = sk + "_" + suffix
			base := target
			for n := 2; used[target]; n++ {
				tail := "_" + strconv.Itoa(n)
				limit := 64 - len(tail)
				if len(base) > limit {
					base = base[:limit]
				}
				target = base + tail
			}
		}
		used[target] = true
		out = append(out, DBMap{Source: source, Target: target, User: dbUser})
	}
	return out
}

func dbSuffix(source string) string {
	s := strings.ToLower(source)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	s = strings.Trim(b.String(), "_")
	if s == "" {
		return "db"
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return strings.TrimRight(s, "_")
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

// importMail provisions the domain's mail infrastructure, recreates each source
// mailbox with a fresh password (the cPanel hash is never reused), restores its
// Maildir, and recreates forwarders. It runs after the web/database restore so a
// mail failure still rolls the whole domain back via the caller's deferred
// rollbackDomain.
func (h *Handlers) importMail(r *http.Request, archivePath string, inv Inventory, domainID int64, targetDomain, sk string) ([]MailCredential, int, error) {
	if len(inv.Mailboxes) == 0 && inv.AliasCount == 0 && inv.MailFiles == 0 {
		return []MailCredential{}, 0, nil
	}
	if h.Mail == nil {
		return nil, 0, errors.New("mail provider is not ready")
	}
	if err := mail.EnableDomain(r.Context(), h.DB, domainID); err != nil {
		return nil, 0, err
	}
	creds := make([]MailCredential, 0, len(inv.Mailboxes))
	for _, local := range inv.Mailboxes {
		body, _ := json.Marshal(map[string]string{"local_part": local})
		req := domainRequest(r, http.MethodPost, "/mail", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Mail.Create(rr, req)
		if rr.Code != http.StatusCreated {
			return nil, 0, fmt.Errorf("mailbox %s: %s", local, strings.TrimSpace(rr.Body.String()))
		}
		var result struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			return nil, 0, err
		}
		creds = append(creds, MailCredential{Email: result.Email, Password: result.Password})
		if inv.PrimaryDomain != "" {
			if err := h.restoreMailbox(r.Context(), archivePath, inv.ArchiveRoot, inv.PrimaryDomain, local, sk); err != nil {
				return nil, 0, fmt.Errorf("mailbox %s messages: %w", local, err)
			}
		}
	}

	aliases, err := readAliases(archivePath, inv.ArchiveRoot, inv.PrimaryDomain, targetDomain)
	if err != nil {
		return nil, 0, err
	}
	created := 0
	for _, a := range aliases {
		body, _ := json.Marshal(map[string]string{"local_part": a.Local, "destination": a.Destination})
		req := domainRequest(r, http.MethodPost, "/mail/aliases", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Mail.CreateAlias(rr, req)
		if rr.Code == http.StatusCreated {
			created++
			continue
		}
		return nil, 0, fmt.Errorf("alias %s: %s", a.Local, strings.TrimSpace(rr.Body.String()))
	}
	return creds, created, nil
}

// domainRequest builds an in-process request carrying the chi URL param `id`
// that the mail handlers read to resolve the target domain.
func domainRequest(parent *http.Request, method, url string, domainID int64, body io.Reader) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", strconv.FormatInt(domainID, 10))
	ctx := context.WithValue(parent.Context(), chi.RouteCtxKey, rc)
	req := httptest.NewRequest(method, url, body).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// restoreMailbox extracts a single source Maildir into the tenant's mail root as
// root, then fixes SELinux context. Subprocesses run with a minimal environment.
func (h *Handlers) restoreMailbox(ctx context.Context, archivePath, root, sourceDomain, local, sk string) error {
	if !strings.HasPrefix(sk, "c_") || root == "" {
		return errors.New("unsafe target")
	}
	target := "/home/" + sk + "/mail/" + local
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	member := root + "/homedir/mail/" + sourceDomain + "/" + local
	cmd := newTransferCommand(ctx, "tar", "-xz", "-f", "-", "-C", target, "--strip-components=5", member)
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		// A mailbox present only in metadata may have no Maildir in the archive.
		if strings.Contains(string(out), "Not found in archive") || strings.Contains(string(out), "Not found") {
			return nil
		}
		return fmt.Errorf("tar: %s", strings.TrimSpace(string(out)))
	}
	if out, err := newTransferCommand(ctx, "chown", "-R", sk+":"+sk, target).CombinedOutput(); err != nil {
		return fmt.Errorf("chown: %s", strings.TrimSpace(string(out)))
	}
	_, _ = newTransferCommand(ctx, "restorecon", "-RF", target).CombinedOutput()
	return nil
}

type aliasImport struct {
	Local       string
	Destination string
}

// readAliases parses the source valias file and rewrites each forwarder onto the
// target domain, dropping pipe/include destinations that Servika cannot host.
func readAliases(archivePath, root, sourceDomain, targetDomain string) ([]aliasImport, error) {
	if sourceDomain == "" {
		return []aliasImport{}, nil
	}
	body, err := readSmallTarMember(archivePath, root+"/va/"+sourceDomain)
	if errors.Is(err, errMemberNotFound) {
		return []aliasImport{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []aliasImport{}
	for _, line := range strings.Split(string(body), "\n") {
		p := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(p) != 2 {
			continue
		}
		source := strings.TrimSpace(p[0])
		destRaw := strings.TrimSpace(p[1])
		if source == "" || destRaw == "" || strings.HasPrefix(destRaw, ":") || strings.HasPrefix(destRaw, "|") {
			continue
		}
		local := strings.TrimSuffix(strings.ToLower(source), "@"+strings.ToLower(sourceDomain))
		if local == "*" {
			local = ""
		}
		if local != "" && !localPartRE.MatchString(local) {
			continue
		}
		var dests []string
		for _, d := range strings.Split(destRaw, ",") {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" {
				continue
			}
			if !strings.Contains(d, "@") && localPartRE.MatchString(d) {
				d += "@" + targetDomain
			}
			d = strings.ReplaceAll(d, "@"+strings.ToLower(sourceDomain), "@"+targetDomain)
			if strings.Contains(d, "@") {
				dests = append(dests, d)
			}
		}
		if len(dests) > 0 {
			out = append(out, aliasImport{Local: local, Destination: strings.Join(dests, ",")})
		}
	}
	return out, nil
}

var errMemberNotFound = errors.New("archive member not found")

// readSmallTarMember returns a single small metadata member from the archive,
// guarding against oversized entries.
func readSmallTarMember(archivePath, want string) ([]byte, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, errMemberNotFound
		}
		if err != nil {
			return nil, err
		}
		if path.Clean(hdr.Name) == path.Clean(want) {
			if hdr.Size > maxMetadataBytes {
				return nil, ErrArchiveTooLarge
			}
			return io.ReadAll(io.LimitReader(tr, maxMetadataBytes))
		}
	}
}
