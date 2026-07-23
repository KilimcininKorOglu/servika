package laravel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"servika/internal/httpx"
)

func deployUnit(id int64) string       { return fmt.Sprintf("servika-laravel-deploy-%d", id) }
func deployLog(id int64) string        { return fmt.Sprintf("%s/deploy-%d.log", logRootDir(), id) }
func deployScriptPath(id int64) string { return fmt.Sprintf("/run/servika-laravel-deploy-%d.sh", id) }

func deployScript(appDir, php, nodeDir string, migrate, npmBuild bool) string {
	var b strings.Builder
	b.WriteString("#!/bin/bash\n")
	fmt.Fprintf(&b, "export PATH=%s:%s\n", nodeDir, systemPath)
	fmt.Fprintf(&b, "cd %s || exit 1\n", shellQuote(appDir))
	b.WriteString("echo '== enable maintenance mode =='\n")
	fmt.Fprintf(&b, "%s artisan down || true\n", php)
	b.WriteString("echo '== git pull =='\n")
	b.WriteString("if [ -d .git ]; then git pull --ff-only 2>&1 || git pull 2>&1 || true; else echo '(not a git repository, skipped)'; fi\n")
	b.WriteString("echo '== composer install (--no-dev) =='\n")
	fmt.Fprintf(&b, "%s %s install --no-interaction --prefer-dist --no-dev 2>&1 || true\n", php, shellQuote(composerBin()))
	if npmBuild {
		b.WriteString("echo '== npm ci + build =='\n")
		fmt.Fprintf(&b, "%s/npm ci --prefix %s --no-fund --no-audit 2>&1 || %s/npm install --prefix %s 2>&1 || true\n", nodeDir, shellQuote(appDir), nodeDir, shellQuote(appDir))
		fmt.Fprintf(&b, "%s/npm run build --prefix %s 2>&1 || true\n", nodeDir, shellQuote(appDir))
	}
	if migrate {
		b.WriteString("echo '== migrate --force =='\n")
		fmt.Fprintf(&b, "%s artisan migrate --force 2>&1 || true\n", php)
	}
	b.WriteString("echo '== cache =='\n")
	fmt.Fprintf(&b, "%s artisan config:cache 2>&1 || true\n", php)
	fmt.Fprintf(&b, "%s artisan route:cache 2>&1 || true\n", php)
	b.WriteString("echo '== disable maintenance mode =='\n")
	fmt.Fprintf(&b, "%s artisan up || true\n", php)
	b.WriteString("echo '== DEPLOY COMPLETE =='\n")
	return b.String()
}

func (h *Handlers) Deploy(w http.ResponseWriter, r *http.Request) {
	id, systemUser, phpVersion, demo, ok := h.lookup(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "deploy cannot be run for demo subscriptions")
		return
	}
	defer lockDomain(id)()
	var currentStatus string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT COALESCE(last_deploy_status,'') FROM cp_laravel_apps WHERE domain_id=?`, id).Scan(&currentStatus)
	if currentStatus == "installing" || currentStatus == "running" {
		httpx.WriteError(w, http.StatusConflict, "an install or deploy operation is already running for this domain")
		return
	}
	var req struct {
		Migrate     bool   `json:"migrate"`
		NpmBuild    bool   `json:"npm_build"`
		NodeVersion string `json:"node_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.NodeVersion != "" && req.NodeVersion != "system" && !reNodeVersion.MatchString(req.NodeVersion) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid node version")
		return
	}
	appDir, err := h.appDir(r, id, systemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid application directory")
		return
	}
	nodeDir := nodeBinDir(req.NodeVersion)
	script := deployScript(appDir, phpBin(phpVersion), nodeDir, req.Migrate, req.NpmBuild)
	path := deployScriptPath(id)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "deploy script write failed")
		return
	}
	_ = exec.Command("systemctl", "reset-failed", deployUnit(id)+".service").Run()
	if err := systemdRunDetached(systemUser, appDir, deployUnit(id), deployLog(id), "/bin/bash", path); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "deploy start failed")
		return
	}
	_, _ = h.DB.ExecContext(r.Context(), `UPDATE cp_laravel_apps SET last_deploy_status='running', last_deploy_at=NOW() WHERE domain_id=?`, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "unit": deployUnit(id)})
}

func (h *Handlers) DeployStatus(w http.ResponseWriter, r *http.Request) {
	id, systemUser, _, _, ok := h.lookup(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	unit := deployUnit(id) + ".service"
	status := unitStatus(unit)
	running := status == "activating" || status == "active" || status == "reloading"
	logTail := fileTail(deployLog(id), 16<<10)
	rec := h.getRecord(r.Context(), id)
	if !running && rec.LastDeployStatus == "running" {
		newStatus := "successful"
		if !strings.Contains(logTail, "DEPLOY COMPLETE") {
			newStatus = "failed"
		}
		appDir, _ := safeAppDir(systemUser, rec.AppRoot)
		lastCommit := ""
		if out, ok := TenantExec(r.Context(), systemUser, appDir, "/usr/bin/git", "-C", appDir, "rev-parse", "--short", "HEAD"); ok {
			lastCommit = strings.TrimSpace(out)
		}
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE cp_laravel_apps SET last_deploy_status=?, last_commit=? WHERE domain_id=?`, newStatus, lastCommit, id)
		_ = exec.Command("systemctl", "reset-failed", unit).Run()
		_ = os.Remove(deployScriptPath(id))
		rec.LastDeployStatus = newStatus
		rec.LastCommit = lastCommit
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"running": running, "status": rec.LastDeployStatus, "last_commit": rec.LastCommit, "log": logTail})
}
