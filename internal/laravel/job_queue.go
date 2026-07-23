package laravel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"servika/internal/httpx"
)

func queueProps(unit string) map[string]string {
	props := map[string]string{}
	out, err := exec.Command("systemctl", "show", "-p", "ActiveState", "-p", "SubState", "-p", "NRestarts", unit).Output()
	if err != nil {
		return props
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if i := strings.IndexByte(line, '='); i > 0 {
			props[line[:i]] = strings.TrimSpace(line[i+1:])
		}
	}
	return props
}

func monitorQueue(unit string) (string, bool) {
	time.Sleep(3 * time.Second)
	props := queueProps(unit)
	if props["ActiveState"] == "failed" || props["SubState"] == "failed" {
		return "worker failed shortly after startup", false
	}
	return "", true
}

func ensureLogDir(systemUser string) {
	dir := "/home/" + systemUser + "/" + logSubdir
	if _, err := os.Stat(dir); err != nil {
		_ = os.MkdirAll(dir, 0750)
		_ = exec.Command("chown", systemUser+":"+systemUser, dir).Run()
	}
}

func cronPath(id int64) string { return cronDir + "/servika-laravel-" + fmt.Sprint(id) }

func (h *Handlers) Schedule(w http.ResponseWriter, r *http.Request) {
	id, systemUser, phpVersion, demo, ok := h.lookup(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "schedule cannot be managed for demo subscriptions")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	appDir, err := h.appDir(r, id, systemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid application directory")
		return
	}
	path := cronPath(id)
	if req.Enabled {
		ensureLogDir(systemUser)
		logFile := "/home/" + systemUser + "/" + logSubdir + "/laravel-schedule.log"
		line := fmt.Sprintf("* * * * * %s %s %s/artisan schedule:run >> %s 2>&1\n", systemUser, phpBin(phpVersion), appDir, logFile)
		body := "# Servika Laravel Toolkit schedule for domain " + fmt.Sprint(id) + "\nPATH=" + systemPath + "\n" + line
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "cron write failed")
			return
		}
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE cp_laravel_apps SET schedule_enabled=1 WHERE domain_id=?`, id)
	} else {
		_ = os.Remove(path)
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE cp_laravel_apps SET schedule_enabled=0 WHERE domain_id=?`, id)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "schedule_enabled": req.Enabled})
}

var reConn = regexp.MustCompile(`^[a-z0-9_]+$`)

func queueUnitName(id int64) string { return "servika-laravel-queue-" + fmt.Sprint(id) }
func queueUnitPath(id int64) string { return "/etc/systemd/system/" + queueUnitName(id) + ".service" }

func queueUnit(id int64, systemUser, appDir, php, conn string, timeout, maxJobs int) string {
	return "[Unit]\n" +
		"Description=Servika Laravel queue worker for domain " + fmt.Sprint(id) + "\n" +
		"After=network.target mariadb.service\n\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"User=" + systemUser + "\nGroup=" + systemUser + "\n" +
		"WorkingDirectory=" + appDir + "\n" +
		"Environment=HOME=/home/" + systemUser + "\n" +
		fmt.Sprintf("ExecStart=%s %s/artisan queue:work %s --sleep=3 --tries=3 --timeout=%d --max-jobs=%d --max-time=3600\n", php, appDir, conn, timeout, maxJobs) +
		"Restart=always\nRestartSec=5\n" +
		"StartLimitIntervalSec=300\nStartLimitBurst=10\n" +
		"Slice=servika-" + systemUser + ".slice\n" +
		"NoNewPrivileges=yes\n" +
		"ProtectSystem=strict\n" +
		"ReadWritePaths=/home/" + systemUser + "\n" +
		"PrivateTmp=yes\n" +
		"ProtectControlGroups=yes\n" +
		"ProtectKernelTunables=yes\n" +
		"RestrictSUIDSGID=yes\n\n" +
		"[Install]\nWantedBy=multi-user.target\n"
}

func (h *Handlers) Queue(w http.ResponseWriter, r *http.Request) {
	id, systemUser, phpVersion, demo, ok := h.lookup(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "queue worker cannot be managed for demo subscriptions")
		return
	}
	var req struct {
		Enabled    bool   `json:"enabled"`
		Timeout    int    `json:"timeout"`
		MaxJobs    int    `json:"max_jobs"`
		Connection string `json:"connection"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	defer lockDomain(id)()
	unit := queueUnitName(id) + ".service"
	if !req.Enabled {
		_ = exec.Command("systemctl", "disable", "--now", unit).Run()
		_ = os.Remove(queueUnitPath(id))
		_ = exec.Command("systemctl", "daemon-reload").Run()
		_ = exec.Command("systemctl", "reset-failed", unit).Run()
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE cp_laravel_apps SET queue_enabled=0 WHERE domain_id=?`, id)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "queue_enabled": false})
		return
	}
	appDir, err := h.appDir(r, id, systemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid application directory")
		return
	}
	if req.Timeout < 5 || req.Timeout > 600 {
		req.Timeout = 60
	}
	if req.MaxJobs < 10 || req.MaxJobs > 100000 {
		req.MaxJobs = 1000
	}
	conn := strings.TrimSpace(req.Connection)
	if conn == "" {
		conn = "database"
	}
	if !reConn.MatchString(conn) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid connection name")
		return
	}
	if err := os.WriteFile(queueUnitPath(id), []byte(queueUnit(id, systemUser, appDir, phpBin(phpVersion), conn, req.Timeout, req.MaxJobs)), 0644); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "unit write failed")
		return
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	if err := exec.Command("systemctl", "enable", "--now", unit).Run(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "worker start failed")
		return
	}
	warning, healthy := monitorQueue(unit)
	_, _ = h.DB.ExecContext(r.Context(), `UPDATE cp_laravel_apps SET queue_enabled=1, queue_timeout=?, queue_max_jobs=?, queue_connection=? WHERE domain_id=?`, req.Timeout, req.MaxJobs, conn, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": healthy, "queue_enabled": true, "healthy": healthy, "warning": warning})
}

func (h *Handlers) QueueStatus(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, ok := h.lookup(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	props := queueProps(queueUnitName(id) + ".service")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"active_state": props["ActiveState"], "sub_state": props["SubState"], "restarts": props["NRestarts"]})
}
