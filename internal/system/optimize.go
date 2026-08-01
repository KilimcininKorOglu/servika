// Package system provides server-level operations: usage, services, updates, and optimization.
package system

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"servika/internal/config"
	"servika/internal/httpx"
)

const optimizeUnit = "servika-optimize-run"

func optimizeLogPath() string { return filepath.Join(config.LogDir(), "optimize.log") }

func optimizeWrapper() string { return filepath.Join(config.LogDir(), "optimize-run.sh") }

// optimizeWrapperContent renders a FIXED wrapper script (no user input enters argv):
// dnf/yum -y update + servika-optimize, each step idempotent and safe. It runs as a
// separate systemd-run process with no DB access, so the language is fixed at write
// time and passed to servika-optimize via PANEL_LANG. English is the primary language.
func optimizeWrapperContent(lang string) string {
	if lang == "tr" {
		return `#!/usr/bin/env bash
set -uo pipefail
echo "========== Sunucu Optimizasyonu -- $(date "+%Y-%m-%d %H:%M:%S") =========="
echo
echo "1/2 Sistem paket güncellemesi (AlmaLinux)"
if command -v dnf >/dev/null 2>&1; then
  dnf -y update
elif command -v yum >/dev/null 2>&1; then
  yum -y update
else
  echo "  (dnf/yum bulunamadı -- paket güncellemesi atlandı)"
fi
echo
echo "2/2 MariaDB / nginx / PHP performans ayarı"
if command -v servika-optimize >/dev/null 2>&1; then
  PANEL_LANG=tr servika-optimize
else
  echo "  (servika-optimize bulunamadı -- ayarlama atlandı)"
fi
echo
echo "========== Optimizasyon tamamlandı =========="
`
	}
	return `#!/usr/bin/env bash
set -uo pipefail
echo "========== Server Optimization -- $(date "+%Y-%m-%d %H:%M:%S") =========="
echo
echo "1/2 System package update (AlmaLinux)"
if command -v dnf >/dev/null 2>&1; then
  dnf -y update
elif command -v yum >/dev/null 2>&1; then
  yum -y update
else
  echo "  (dnf/yum not found -- package update skipped)"
fi
echo
echo "2/2 MariaDB / nginx / PHP performance tuning"
if command -v servika-optimize >/dev/null 2>&1; then
  PANEL_LANG=en servika-optimize
else
  echo "  (servika-optimize not found -- tuning skipped)"
fi
echo
echo "========== Optimization complete =========="
`
}

// optimizeRunning checks if the transient unit is still active.
func optimizeRunning() (bool, string) {
	d := strings.TrimSpace(runOutput("systemctl", "is-active", optimizeUnit))
	return d == "active" || d == "activating", d
}

func runOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

// writeOptimizeWrapper atomically writes the wrapper script (0700).
func writeOptimizeWrapper(lang string) error {
	wrapper := optimizeWrapper()
	tmp := wrapper + ".tmp"
	if err := os.WriteFile(tmp, []byte(optimizeWrapperContent(lang)), 0o700); err != nil {
		return err
	}
	return os.Rename(tmp, wrapper)
}

// OptimizeStatus returns GET /system/optimize.
func OptimizeStatus(w http.ResponseWriter, r *http.Request) {
	running, status := optimizeRunning()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"running": running,
		"status":  status,
	})
}

// OptimizeStart starts POST /system/optimize/start.
func OptimizeStart(w http.ResponseWriter, r *http.Request) {
	running, _ := optimizeRunning()
	if running {
		httpx.WriteError(w, http.StatusConflict, t("optimization is already running", "optimizasyon zaten sürüyor"))
		return
	}
	if err := writeOptimizeWrapper(panelLang()); err != nil {
		log.Printf("optimize: write wrapper: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, t("could not start optimization", "optimizasyon başlatılamadı"))
		return
	}
	logPath := optimizeLogPath()
	wrapper := optimizeWrapper()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		log.Printf("optimize: prepare log directory %s: %v", filepath.Dir(logPath), err)
		httpx.WriteError(w, http.StatusInternalServerError, t("could not start optimization", "optimizasyon başlatılamadı"))
		return
	}
	header := fmt.Sprintf("%s\n", t(
		fmt.Sprintf("=== Optimization started: %s ===", time.Now().Format("2006-01-02 15:04:05")),
		fmt.Sprintf("=== Optimizasyon başlatıldı: %s ===", time.Now().Format("2006-01-02 15:04:05"))))
	if err := os.WriteFile(logPath, []byte(header), 0o640); err != nil {
		log.Printf("optimize: open log %s: %v", logPath, err)
		httpx.WriteError(w, http.StatusInternalServerError, t("could not start optimization", "optimizasyon başlatılamadı"))
		return
	}
	// systemd-run: transient unit under PID 1; output via append: to log file.
	cmd := exec.Command("systemd-run",
		"--collect",
		"--unit", optimizeUnit,
		"--description", "Servika server optimization",
		"-p", "StandardOutput=append:"+logPath,
		"-p", "StandardError=append:"+logPath,
		wrapper)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("optimize: systemd-run start: %v: %s", err, strings.TrimSpace(string(out)))
		httpx.WriteError(w, http.StatusInternalServerError, t("could not start optimization", "optimizasyon başlatılamadı"))
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"started": true})
}

// OptimizeLog returns GET /system/optimize/log.
func OptimizeLog(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(optimizeLogPath())
	if err != nil {
		b = nil
	}
	s := string(b)
	const maxLog = 60000
	if len(s) > maxLog {
		s = s[len(s)-maxLog:]
	}
	running, status := optimizeRunning()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"log":     s,
		"running": running,
		"status":  status,
	})
}
