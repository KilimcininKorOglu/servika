// Package wordpress provides one-click WordPress installation and management through wp-cli.
// Commands run as the c_<slug> domain user, paths remain restricted to public_html, and root-site deletion is prohibited.
package wordpress

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"servika/internal/credentials"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handlers struct{ DB *sql.DB }

const wpBin = "/usr/local/bin/wp"

var (
	subdirectoryPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,30}[a-z0-9])?$`)
	reAdmin             = regexp.MustCompile(`^[A-Za-z0-9._@-]{3,60}$`)
	reEmail             = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reDBName            = regexp.MustCompile(`define\(\s*['"]DB_NAME['"]\s*,\s*['"]([^'"]+)['"]`)
	reManagedDBName     = regexp.MustCompile(`^wp_([a-f0-9]{8})$`)
)

type Installation struct {
	Dir      string `json:"dir"`
	SiteURL  string `json:"site_url"`
	AdminURL string `json:"admin_url"`
	Version  string `json:"version"`
}

func (h *Handlers) domain(r *http.Request) (id int64, systemUser, domainName string, ssl, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var cert string
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, domain_name, COALESCE(cert_path,''), COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&systemUser, &domainName, &cert, &isDemo); err != nil {
		return id, "", "", false, false, false
	}
	return id, systemUser, domainName, cert != "", isDemo == 1, true
}

// runWP runs wp-cli as the domain user with HOME set and without a shell.
// It invokes PHP directly with a 512 MB memory limit because the raw .phar shebang does not read
// WP_CLI_PHP_ARGS and archive extraction can exceed the default 128 MB limit.
func runWP(systemUser string, args ...string) ([]byte, error) {
	full := append([]string{"-u", systemUser, "--", "env", "HOME=/home/" + systemUser,
		"/usr/bin/php", "-d", "memory_limit=512M", wpBin}, args...)
	cmd := exec.Command("runuser", full...)
	return cmd.CombinedOutput()
}

func (h *Handlers) scheme(ssl bool) string {
	if ssl {
		return "https://"
	}
	return "http://"
}

// GET /domains/{id}/wordpress discovers installations in public_html and one directory level below.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	_, systemUser, _, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	root := "/home/" + systemUser + "/public_html"
	out := []Installation{}
	candidates := []string{root}
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				candidates = append(candidates, filepath.Join(root, e.Name()))
			}
		}
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "wp-config.php")); err != nil {
			continue
		}
		installation := Installation{Dir: "/" + strings.TrimPrefix(strings.TrimPrefix(dir, root), "/")}
		if installation.Dir == "/" {
			installation.Dir = "/ (root)"
		}
		if b, err := runWP(systemUser, "core", "version", "--path="+dir); err == nil {
			installation.Version = strings.TrimSpace(string(b))
		}
		if b, err := runWP(systemUser, "option", "get", "siteurl", "--path="+dir); err == nil {
			installation.SiteURL = strings.TrimSpace(string(b))
			installation.AdminURL = installation.SiteURL + "/wp-admin"
		}
		out = append(out, installation)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// AllInstallation summarizes one WordPress installation for the aggregate table.
type AllInstallation struct {
	DomainID    int64  `json:"domain_id"`
	DomainName  string `json:"domain_name"`
	Dir         string `json:"dir"`
	Version     string `json:"version"`
	LastVersion string `json:"last_version"` // Target version when an update exists.
	Status      string `json:"status"`       // WordPress update status.
	InstallDate string `json:"install_date"` // wp-config.php mtime, YYYY-MM-DD
	SiteURL     string `json:"site_url"`
	AdminURL    string `json:"admin_url"`
}

type wpCandidate struct {
	domainID               int64
	systemUser, domainName string
	ssl                    bool
	dir, root              string
}

// GET /wordpress/all scans installations across all domains for versions, updates, and installation dates.
// The AdminOnly endpoint runs wp-cli calls through a four-worker pool with per-call context timeouts.
func (h *Handlers) ListAll(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, system_user, domain_name, COALESCE(cert_path,'') FROM domains ORDER BY domain_name`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list domains")
		return
	}
	var candidates []wpCandidate
	for rows.Next() {
		var id int64
		var systemUser, domainName, cert string
		if err := rows.Scan(&id, &systemUser, &domainName, &cert); err != nil {
			continue
		}
		if !strings.HasPrefix(systemUser, "c_") {
			continue
		}
		root := "/home/" + systemUser + "/public_html"
		directories := []string{root}
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					directories = append(directories, filepath.Join(root, e.Name()))
				}
			}
		}
		for _, dir := range directories {
			if _, err := os.Stat(filepath.Join(dir, "wp-config.php")); err != nil {
				continue
			}
			candidates = append(candidates, wpCandidate{id, systemUser, domainName, cert != "", dir, root})
		}
	}
	_ = rows.Err()
	_ = rows.Close()

	out := make([]AllInstallation, len(candidates))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, a wpCandidate) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = h.inspectInstallation(r.Context(), a)
		}(i, candidates[i])
	}
	wg.Wait()
	httpx.WriteJSON(w, http.StatusOK, out)
}

// inspectInstallation collects version, update state, and installation date for one WordPress installation.
func (h *Handlers) inspectInstallation(ctx context.Context, a wpCandidate) AllInstallation {
	directoryPath := strings.TrimPrefix(strings.TrimPrefix(a.dir, a.root), "/")
	base := h.scheme(a.ssl) + a.domainName
	if directoryPath != "" {
		base += "/" + directoryPath
	}
	directoryLabel := "/" + directoryPath
	if directoryLabel == "/" {
		directoryLabel = "/ (root)"
	}
	installation := AllInstallation{
		DomainID: a.domainID, DomainName: a.domainName, Dir: directoryLabel,
		SiteURL: base, AdminURL: base + "/wp-admin", Status: "unknown",
	}
	// Read the installed version.
	c1, cancel1 := context.WithTimeout(ctx, 15*time.Second)
	if b, err := wpStdout(c1, a.systemUser, "core", "version", "--path="+a.dir); err == nil {
		installation.Version = strings.TrimSpace(string(b))
	}
	cancel1()
	// Check for updates with a timeout because wp-cli calls the wordpress.org API.
	c2, cancel2 := context.WithTimeout(ctx, 25*time.Second)
	if b, err := wpStdout(c2, a.systemUser, "core", "check-update", "--path="+a.dir, "--format=json"); err == nil {
		bt := bytes.TrimSpace(b)
		if len(bt) == 0 || string(bt) == "[]" {
			installation.Status = "current"
		} else {
			var ups []struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(bt, &ups) == nil {
				if len(ups) > 0 {
					installation.Status = "outdated"
					installation.LastVersion = ups[0].Version
				} else {
					installation.Status = "current"
				}
			}
		}
	}
	cancel2()
	// Use the wp-config.php modification time as the installation date because it rarely changes.
	if fi, err := os.Stat(filepath.Join(a.dir, "wp-config.php")); err == nil {
		installation.InstallDate = fi.ModTime().Format("2006-01-02")
	}
	return installation
}

// wpStdout runs wp-cli as the domain user with a context timeout and returns only stdout.
// Discarding stderr prevents deprecation warnings from corrupting JSON output.
func wpStdout(ctx context.Context, systemUser string, args ...string) ([]byte, error) {
	full := append([]string{"-u", systemUser, "--", "env", "HOME=/home/" + systemUser,
		"/usr/bin/php", "-d", "memory_limit=512M", wpBin}, args...)
	cmd := exec.CommandContext(ctx, "runuser", full...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// wpInstallLock serializes concurrent WordPress installs per target path.
var wpInstallLock sync.Map

// installAlreadyExists checks if target dir already has WordPress content.
func installAlreadyExists(target string) (string, bool) {
	entries, err := os.ReadDir(target)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "wp-config.php" {
			return "WordPress is already installed in this directory", true
		}
		if name == "index.html" || name == "index.htm" || name == "index.php" ||
			name == "favicon.ico" || name == "favicon.png" ||
			name == ".htaccess" || name == ".htpasswd" ||
			name == "robots.txt" || name == "sitemap.xml" ||
			name == "cgi-bin" || name == ".well-known" ||
			strings.HasPrefix(name, ".") {
			continue
		}
		return "Target directory already contains content", true
	}
	return "", false
}

// POST /domains/{id}/wordpress installs WordPress.
func (h *Handlers) Install(w http.ResponseWriter, r *http.Request) {
	id, systemUser, domainName, ssl, demo, ok := h.domain(r)
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
		SubDir     string `json:"sub_dir"`
		SiteTitle  string `json:"site_title"`
		AdminUser  string `json:"admin_user"`
		AdminEmail string `json:"admin_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SubDir = strings.Trim(strings.TrimSpace(req.SubDir), "/")
	req.SiteTitle = strings.TrimSpace(req.SiteTitle)
	req.AdminUser = strings.TrimSpace(req.AdminUser)
	req.AdminEmail = strings.TrimSpace(req.AdminEmail)
	if req.SiteTitle == "" || len(req.SiteTitle) > 120 {
		httpx.WriteError(w, http.StatusBadRequest, "site title is required (maximum 120 characters)")
		return
	}
	if !reAdmin.MatchString(req.AdminUser) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid administrator username")
		return
	}
	if !reEmail.MatchString(req.AdminEmail) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	if req.SubDir != "" && !subdirectoryPattern.MatchString(req.SubDir) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid subdirectory (lowercase letters, digits, and hyphens only)")
		return
	}
	root := "/home/" + systemUser + "/public_html"
	target := root
	if req.SubDir != "" {
		target = filepath.Join(root, req.SubDir)
	}
	// Lock to serialize concurrent installs to the same target.
	if _, loaded := wpInstallLock.LoadOrStore(target, true); loaded {
		httpx.WriteError(w, http.StatusConflict, "WordPress installation is already in progress for this directory")
		return
	}
	defer wpInstallLock.Delete(target)
	if msg, ok := installAlreadyExists(target); ok {
		httpx.WriteError(w, http.StatusConflict, msg)
		return
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create target directory")
		return
	}
	_ = exec.Command("chown", "-R", systemUser+":"+systemUser, target).Run()
	_ = exec.Command("restorecon", "-R", target).Run()

	// Create the database.
	slug := randSlug()
	dbName := "wp_" + slug
	dbUser := "wpu_" + slug
	dbPass := credentials.RandomPassword(24)
	if err := credentials.MySQLCreateDB(h.DB, id, dbName, dbUser, dbPass); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	fail := func(stage string, out []byte) {
		_ = credentials.MySQLDropDB(h.DB, dbName, dbUser)
		if req.SubDir != "" { // Remove only the subdirectory created by this operation.
			_ = os.RemoveAll(target)
		}
		msg := strings.TrimSpace(string(out))
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		httpx.WriteError(w, http.StatusInternalServerError, stage+" failed: "+msg)
	}

	if out, err := runWP(systemUser, "core", "download", "--path="+target, "--locale=en_US"); err != nil {
		fail("WordPress download", out)
		return
	}
	if out, err := runWP(systemUser, "config", "create", "--dbname="+dbName, "--dbuser="+dbUser,
		"--dbpass="+dbPass, "--dbhost=localhost", "--locale=en_US", "--path="+target, "--skip-check"); err != nil {
		fail("wp-config creation", out)
		return
	}
	url := h.scheme(ssl) + domainName
	if req.SubDir != "" {
		url += "/" + req.SubDir
	}
	adminPassword := randomPassword()
	if out, err := runWP(systemUser, "core", "install", "--url="+url, "--title="+req.SiteTitle,
		"--admin_user="+req.AdminUser, "--admin_password="+adminPassword,
		"--admin_email="+req.AdminEmail, "--skip-email", "--path="+target); err != nil {
		fail("WordPress installation", out)
		return
	}
	_ = exec.Command("chown", "-R", systemUser+":"+systemUser, target).Run()
	_ = exec.Command("restorecon", "-R", target).Run()

	version := ""
	if b, err := runWP(systemUser, "core", "version", "--path="+target); err == nil {
		version = strings.TrimSpace(string(b))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "site_url": url, "admin_url": url + "/wp-admin",
		"admin_user": req.AdminUser, "admin_password": adminPassword,
		"version": version, "db_name": dbName,
	})
}

// POST /domains/{id}/wordpress/update updates an installation from {dir}.
func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	_, systemUser, _, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "not available for demo subscriptions")
		return
	}
	var updateRequest struct {
		Dir string `json:"dir"`
	}
	_ = json.NewDecoder(r.Body).Decode(&updateRequest)
	dir, err := resolveDirectory(systemUser, updateRequest.Dir)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	out1, e1 := runWP(systemUser, "core", "update", "--path="+dir)
	out2, _ := runWP(systemUser, "core", "update-db", "--path="+dir)
	if e1 != nil {
		log.Printf("wp core update failed for %s (dir=%s): %s", systemUser, dir, strings.TrimSpace(string(out1)))
		httpx.WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}
	version := ""
	if b, err := runWP(systemUser, "core", "version", "--path="+dir); err == nil {
		version = strings.TrimSpace(string(b))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version,
		"output": strings.TrimSpace(string(out1)) + "\n" + strings.TrimSpace(string(out2))})
}

// DELETE /domains/{id}/wordpress removes an installation from {dir, db_delete}.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, systemUser, _, _, demo, ok := h.domain(r)
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
	var deleteRequest struct {
		Dir      string `json:"dir"`
		DBDelete bool   `json:"delete_db"`
	}
	_ = json.NewDecoder(r.Body).Decode(&deleteRequest)
	dir, err := resolveDirectory(systemUser, deleteRequest.Dir)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	root := "/home/" + systemUser + "/public_html"
	// Protect the root site by refusing to remove public_html itself.
	if dir == root {
		httpx.WriteError(w, http.StatusBadRequest, "WordPress in the root directory cannot be removed from the panel because it would delete the entire site; use File Manager")
		return
	}
	if deleteRequest.DBDelete {
		if b, err := os.ReadFile(filepath.Join(dir, "wp-config.php")); err == nil {
			if m := reDBName.FindSubmatch(b); len(m) == 2 {
				dbName := string(m[1])
				// Cross-tenant guard: only drop databases that belong to this domain
				// AND carry the wp_ prefix (prevents arbitrary DB drop via payload).
				if h.dropAllowed(r, id, dbName) {
					if dbUser, ok := managedDBAccount(dbName); ok {
						_ = credentials.MySQLDropDB(h.DB, dbName, dbUser)
					}
				}
			}
		}
	}
	if err := os.RemoveAll(dir); err != nil { // The root path was rejected above, so this is a subdirectory.
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete record")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// resolveDirectory converts a directory value into a safe absolute path under public_html containing wp-config.php.
func resolveDirectory(systemUser, directoryValue string) (string, error) {
	root := "/home/" + systemUser + "/public_html"
	d := strings.TrimPrefix(strings.TrimSpace(directoryValue), "/ (root)")
	rel := strings.Trim(strings.TrimSpace(d), "/")
	dir := root
	if rel != "" && rel != "(root)" {
		dir = filepath.Join(root, rel)
	}
	clean := filepath.Clean(dir)
	if clean != root && !strings.HasPrefix(clean, root+"/") {
		return "", fmt.Errorf("path is outside the domain directory")
	}
	if _, err := os.Stat(filepath.Join(clean, "wp-config.php")); err != nil {
		return "", fmt.Errorf("WordPress was not found in this directory")
	}
	return clean, nil
}

// managedDBAccount validates a package-managed database name and derives its paired account.
func managedDBAccount(dbName string) (string, bool) {
	m := reManagedDBName.FindStringSubmatch(dbName)
	if len(m) != 2 {
		return "", false
	}
	return "wpu_" + m[1], true
}

// dbNameWPGuard validates that a database name is a syntactically valid MySQL identifier
// and carries the wp_ prefix used by managed WordPress installations. Rejects names that
// fail the identifier check (SQL injection / cross-tenant guard).
func dbNameWPGuard(dbAdi string) bool {
	if !credentials.ValidDBIdentifier(dbAdi) {
		return false
	}
	return strings.HasPrefix(dbAdi, "wp_")
}

// dbOwnedBy checks whether the authenticated domain owns the given WordPress database.
// The database must belong to a db_accounts row whose domain_id matches the request domain.
func (h *Handlers) dbOwnedBy(r *http.Request, domainID int64, dbAdi string) bool {
	var cnt int
	err := h.DB.QueryRowContext(r.Context(),
		"SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?", domainID, dbAdi).Scan(&cnt)
	return err == nil && cnt > 0
}

// dropAllowed gates database deletion for managed WordPress databases.
// A database may only be dropped when:
//   - It passes dbNameWPGuard (wp_ prefix + valid MySQL identifier)
//   - The authenticated domain owns it (dbOwnedBy)
//
// Without this gate a tenant could supply an arbitrary dbName in the delete payload
// and trick the panel into dropping another customer's database (cross-tenant DB drop).
func (h *Handlers) dropAllowed(r *http.Request, domainID int64, dbAdi string) bool {
	return dbNameWPGuard(dbAdi) && h.dbOwnedBy(r, domainID, dbAdi)
}

func randSlug() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) // Eight hexadecimal characters.
}

func randomPassword() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	out := make([]byte, 18)
	for i, c := range b {
		out[i] = alphabet[int(c)%len(alphabet)]
	}
	return string(out)
}
