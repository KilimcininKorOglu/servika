// Package redis manages isolated per-tenant Valkey and Redis caches.
// A single Valkey instance uses one ACL user per domain, restricted to the ~<system-user>:* key prefix
// with @dangerous and @admin denied, so sites cannot access each other's caches.
// ACLs are managed through valkey-cli without an additional Go dependency.
package redis

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/config"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handlers struct{ DB *sql.DB }

// systemUserPattern restricts system_user to a safe character set to prevent valkey-cli argument injection.
var systemUserPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

const (
	redisHost = "127.0.0.1"
	redisPort = 6379
)

func adminPass() string { return os.Getenv("SERVIKA_REDIS_ADMIN_PASS") }

// cli runs valkey-cli with the admin password in REDISCLI_AUTH rather than argv.
func cli(args ...string) (string, error) {
	cmd := exec.Command("valkey-cli", args...)
	cmd.Env = []string{
		"REDISCLI_AUTH=" + adminPass(),
		"LANG=C",
		"LC_ALL=C",
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func genPass() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// enableUser creates a tenant ACL limited to the <system-user>:* key and channel prefixes while denying
// dangerous and administrative commands. Isolation remains enforced because flushall, flushdb,
// keys, config, and swapdb return NOPERM. Read-only diagnostic commands required by the WordPress
// Redis Object Cache plugin are re-enabled. info and dbsize expose aggregate statistics but not
// another tenant's keys, which is acceptable for the shared cache.
func enableUser(systemUser, password string) error {
	if _, err := cli("ACL", "SETUSER", systemUser, "on", ">"+password,
		"resetkeys", "~"+systemUser+":*", "resetchannels", "&"+systemUser+":*",
		"+@all", "-@dangerous", "-@admin",
		"+info", "+dbsize", "+command", "+ping", "+echo", "+client|no-evict"); err != nil {
		return err
	}
	_, err := cli("ACL", "SAVE")
	return err
}

func disableUser(systemUser string) {
	_, _ = cli("ACL", "DELUSER", systemUser)
	_, _ = cli("ACL", "SAVE")
}

// ---- Automatic WordPress connection through wp-cli as the domain user ----

func wpBin() string { return config.WPCLIBin() }

func runWPCommand(systemUser string, args ...string) ([]byte, error) {
	full := append([]string{"-u", systemUser, "--", "env", "HOME=/home/" + systemUser,
		"/usr/bin/php", "-d", "memory_limit=512M", wpBin()}, args...)
	return exec.Command("runuser", full...).CombinedOutput()
}

// wpDirectories finds WordPress installations with wp-config.php in <system-user>/public_html or one level below.
func wpDirectories(systemUser string) []string {
	root := "/home/" + systemUser + "/public_html"
	candidates := []string{root}
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				candidates = append(candidates, filepath.Join(root, e.Name()))
			}
		}
	}
	var out []string
	for _, d := range candidates {
		if _, err := os.Stat(filepath.Join(d, "wp-config.php")); err == nil {
			out = append(out, d)
		}
	}
	return out
}

// connectWordPress automatically connects Redis to WordPress installations and returns the number connected.
// This is a best-effort operation. It does not use `wp redis enable` because that command invokes
// Predis and FLUSHDB, which the ACL correctly denies. Instead, the drop-in is copied directly,
// WP_REDIS_PASSWORD uses the [username, password] array format for ACL authentication, and
// selective flush implements runtime flushing with scan and unlink.
func connectWordPress(systemUser, password string) int {
	connected := 0
	for _, dir := range wpDirectories(systemUser) {
		set := func(key, value string, raw bool) {
			args := []string{"config", "set", key, value, "--type=constant", "--path=" + dir}
			if raw {
				args = append(args, "--raw")
			}
			_, _ = runWPCommand(systemUser, args...)
		}
		set("WP_REDIS_HOST", redisHost, false)
		set("WP_REDIS_PORT", strconv.Itoa(redisPort), true)
		// The drop-in authenticates ACL users when WP_REDIS_PASSWORD is an array of username and password.
		set("WP_REDIS_PASSWORD", "array('"+systemUser+"','"+password+"')", true)
		set("WP_REDIS_PREFIX", systemUser+":", false)
		set("WP_REDIS_SELECTIVE_FLUSH", "true", true)
		set("WP_REDIS_CLIENT", "phpredis", false)
		set("WP_CACHE", "true", true)
		// Remove obsolete single-string USERNAME or PASSWORD configuration.
		_, _ = runWPCommand(systemUser, "config", "delete", "WP_REDIS_USERNAME", "--path="+dir)

		if _, err := runWPCommand(systemUser, "plugin", "install", "redis-cache", "--activate", "--path="+dir); err != nil {
			continue
		}
		// Install the drop-in directly to avoid the FLUSHDB performed by wp redis enable.
		src := filepath.Join(dir, "wp-content/plugins/redis-cache/includes/object-cache.php")
		dst := filepath.Join(dir, "wp-content/object-cache.php")
		if _, err := exec.Command("runuser", "-u", systemUser, "--", "cp", "-f", src, dst).CombinedOutput(); err != nil {
			continue
		}
		// Treat a status containing "Connected" as a successful connection.
		if out, err := runWPCommand(systemUser, "redis", "status", "--path="+dir); err == nil && strings.Contains(string(out), "Connected") {
			connected++
		}
	}
	return connected
}

// disconnectWordPress disables Redis in WordPress installations by removing the drop-in and constants.
// It does not use `wp redis disable`, which may attempt FLUSHDB, so the drop-in is removed directly.
func disconnectWordPress(systemUser string) {
	for _, dir := range wpDirectories(systemUser) {
		_, _ = exec.Command("runuser", "-u", systemUser, "--", "rm", "-f",
			filepath.Join(dir, "wp-content/object-cache.php")).CombinedOutput()
		for _, key := range []string{"WP_REDIS_HOST", "WP_REDIS_PORT", "WP_REDIS_USERNAME",
			"WP_REDIS_PASSWORD", "WP_REDIS_PREFIX", "WP_REDIS_SELECTIVE_FLUSH", "WP_REDIS_CLIENT", "WP_CACHE"} {
			_, _ = runWPCommand(systemUser, "config", "delete", key, "--path="+dir)
		}
	}
}

type statusResponse struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	Prefix      string `json:"prefix"`
	WPSnippet   string `json:"wp_snippet,omitempty"`
	WPConnected int    `json:"wp_connected,omitempty"`
}

func (h *Handlers) domainSystemUser(r *http.Request) (id int64, systemUser string, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user FROM domains WHERE id=?`, id).Scan(&systemUser); err != nil {
		return id, "", false
	}
	return id, systemUser, systemUserPattern.MatchString(systemUser)
}

func wpSnippet(systemUser, password string) string {
	return "// Redis object cache\n" +
		"define( 'WP_REDIS_HOST', '" + redisHost + "' );\n" +
		"define( 'WP_REDIS_PORT', " + strconv.Itoa(redisPort) + " );\n" +
		"define( 'WP_REDIS_PASSWORD', array( '" + systemUser + "', '" + password + "' ) );\n" +
		"define( 'WP_REDIS_PREFIX', '" + systemUser + ":' );\n" +
		"define( 'WP_REDIS_SELECTIVE_FLUSH', true );\n" +
		"define( 'WP_REDIS_CLIENT', 'phpredis' );\n" +
		"define( 'WP_CACHE', true );"
}

// GET /domains/{id}/redis
func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	id, systemUser, ok := h.domainSystemUser(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	var enabled int
	var password string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT enabled, redis_pass FROM domain_redis WHERE domain_id=?`, id).Scan(&enabled, &password)
	if err != nil || enabled == 0 {
		httpx.WriteJSON(w, http.StatusOK, statusResponse{Enabled: false, Host: redisHost, Port: redisPort, Username: systemUser, Prefix: systemUser + ":"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, statusResponse{
		Enabled: true, Host: redisHost, Port: redisPort, Username: systemUser, Password: "***",
		Prefix: systemUser + ":", WPSnippet: wpSnippet(systemUser, "***"),
	})
}

// POST /domains/{id}/redis enables the tenant cache.
func (h *Handlers) Open(w http.ResponseWriter, r *http.Request) {
	id, systemUser, ok := h.domainSystemUser(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if adminPass() == "" {
		httpx.WriteError(w, http.StatusServiceUnavailable, "redis is not configured (SERVIKA_REDIS_ADMIN_PASS is missing)")
		return
	}
	password := genPass()
	if err := enableUser(systemUser, password); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "redis ACL could not be created")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO domain_redis (domain_id, system_user, redis_pass, enabled) VALUES (?,?,?,1)
		 ON DUPLICATE KEY UPDATE system_user=VALUES(system_user), redis_pass=VALUES(redis_pass), enabled=1`,
		id, systemUser, password); err != nil {
		disableUser(systemUser) // Roll back the ACL if the database write fails.
		httpx.WriteError(w, http.StatusInternalServerError, "redis settings could not be saved")
		return
	}
	// Connect existing WordPress installations on a best-effort basis, retaining the manual snippet otherwise.
	connected := connectWordPress(systemUser, password)
	httpx.WriteJSON(w, http.StatusOK, statusResponse{
		Enabled: true, Host: redisHost, Port: redisPort, Username: systemUser, Password: password,
		Prefix: systemUser + ":", WPSnippet: wpSnippet(systemUser, password), WPConnected: connected,
	})
}

// DELETE /domains/{id}/redis disables the tenant cache.
func (h *Handlers) Close(w http.ResponseWriter, r *http.Request) {
	id, systemUser, ok := h.domainSystemUser(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	disconnectWordPress(systemUser) // Remove the WordPress drop-in while the credentials are still valid.
	disableUser(systemUser)
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM domain_redis WHERE domain_id=?`, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// CloseDomain removes the WordPress drop-in, Valkey ACL user, and domain_redis row when
// domains.Delete removes a domain. Explicit cleanup is required because domain_redis lacks
// an ON DELETE CASCADE foreign key and would otherwise retain an orphaned row.
func CloseDomain(db *sql.DB, id int64, systemUser string) {
	disconnectWordPress(systemUser)
	disableUser(systemUser)
	_, _ = db.Exec(`DELETE FROM domain_redis WHERE domain_id=?`, id)
}
