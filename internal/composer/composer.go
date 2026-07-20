// Package composer: per-domain PHP Composer execution (whitelist + as the domain user).
package composer

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB *sql.DB
}

const composerBin = "/usr/local/bin/composer"

var rePkg = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]*)/[a-z0-9]([a-z0-9._-]*)(:[\^~<>=0-9.* |,-]+)?$`)

func (h *Handlers) load(r *http.Request) (id int64, systemUser string, demo bool, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, is_demo FROM domains WHERE id=?`, id).Scan(&systemUser, &isDemo); err != nil {
		return id, "", false, false
	}
	return id, systemUser, isDemo == 1, true
}

// GET /domains/{id}/composer, status (is composer installed, does composer.json exist)
func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	_, systemUser, _, ok := h.load(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	var version string
	installed := false
	vc := exec.Command(composerBin, "--version", "--no-ansi")
	vc.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/tmp",
		"COMPOSER_HOME=/tmp",
	}
	if out, err := vc.Output(); err == nil { // stdout-only: exclude the stderr plugin warning
		installed = true
		version = strings.TrimSpace(string(out))
	}
	_, jErr := os.Stat("/home/" + systemUser + "/public_html/composer.json")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"installed":     installed,
		"version":       version,
		"composer_json": jErr == nil,
		"username":      systemUser,
		"dir":           "/home/" + systemUser + "/public_html",
	})
}

// POST /domains/{id}/composer  body {"command":"install|update|dump-autoload|validate|require|remove","package":"vendor/pkg"}
func (h *Handlers) Run(w http.ResponseWriter, r *http.Request) {
	_, systemUser, demo, ok := h.load(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "composer cannot run for a demo subscription")
		return
	}
	if !strings.HasPrefix(systemUser, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user")
		return
	}
	if _, err := os.Stat(composerBin); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "composer is not installed on the server")
		return
	}
	var req struct {
		Command string `json:"command"`
		Package string `json:"package"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	allowed := map[string]bool{"install": true, "update": true, "dump-autoload": true, "validate": true, "require": true, "remove": true, "show": true}
	if !allowed[req.Command] {
		httpx.WriteError(w, http.StatusBadRequest, "command is not allowed")
		return
	}
	directory := "/home/" + systemUser + "/public_html"
	// Pass arguments explicitly without a shell to prevent command injection.
	args := []string{"-u", systemUser, "--", composerBin, req.Command, "--no-interaction", "--no-ansi", "-d", directory}
	if req.Command == "install" || req.Command == "update" {
		args = append(args, "--no-scripts", "--no-plugins")
	}
	if req.Command == "require" || req.Command == "remove" {
		pkg := strings.TrimSpace(req.Package)
		if !rePkg.MatchString(pkg) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid package name (expected vendor/package[:version])")
			return
		}
		args = append(args, pkg)
	}
	cmd := exec.Command("runuser", args...)
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/home/" + systemUser,
		"COMPOSER_HOME=/home/" + systemUser + "/.composer",
		"COMPOSER_ALLOW_SUPERUSER=0",
	}
	out, err := cmd.CombinedOutput()
	output := string(out)
	if len(output) > 20000 {
		output = output[len(output)-20000:]
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      err == nil,
		"command": req.Command,
		"output":  output,
	})
}
