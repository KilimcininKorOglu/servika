package system

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"servika/internal/httpx"
)

const (
	updateScript  = "/usr/local/bin/servika-update"
	updateRawURL  = "https://raw.githubusercontent.com/servika/servika/main/assets/ops/servika-update"
	updateLogPath = "/opt/servika/logs/update.log"
	updateUnit    = "servika-update"
)

func updateRunning() bool {
	output, _ := exec.Command("systemctl", "is-active", updateUnit).CombinedOutput()
	state := strings.TrimSpace(string(output))
	return state == "active" || state == "activating"
}

// UpdateStatus reports whether the update tool exists and an update is running.
func UpdateStatus(w http.ResponseWriter, _ *http.Request) {
	_, statError := os.Stat(updateScript)
	running := updateRunning()
	status := "idle"
	if running {
		status = "running"
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"tool_available": statError == nil,
		"running":        running,
		"status":         status,
	})
}

func downloadUpdateTool() error {
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Get(updateRawURL)
	if err != nil {
		return fmt.Errorf("download update tool: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download update tool: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read update tool: %w", err)
	}
	if !strings.HasPrefix(string(body), "#!") {
		return fmt.Errorf("download update tool: unexpected content")
	}

	temporaryPath := updateScript + ".tmp"
	if err := os.WriteFile(temporaryPath, body, 0o755); err != nil {
		return fmt.Errorf("write update tool: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o755); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("make update tool executable: %w", err)
	}
	if err := os.Rename(temporaryPath, updateScript); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("install update tool: %w", err)
	}
	return nil
}

// StartUpdate bootstraps the update tool when needed and starts it in a transient systemd unit.
func StartUpdate(w http.ResponseWriter, _ *http.Request) {
	if updateRunning() {
		httpx.WriteError(w, http.StatusConflict, "an update is already running")
		return
	}

	toolDownloaded := false
	if _, err := os.Stat(updateScript); err != nil {
		if err := downloadUpdateTool(); err != nil {
			httpx.WriteError(w, http.StatusBadGateway, "the update tool could not be downloaded")
			return
		}
		toolDownloaded = true
	}

	if err := os.MkdirAll("/opt/servika/logs", 0o750); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "the update log could not be prepared")
		return
	}
	logHeader := fmt.Sprintf("=== Update started: %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
	if toolDownloaded {
		logHeader += "(The missing update tool was downloaded.)\n"
	}
	if err := os.WriteFile(updateLogPath, []byte(logHeader), 0o640); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "the update log could not be prepared")
		return
	}

	command := exec.Command("systemd-run",
		"--collect",
		"--unit", updateUnit,
		"--description", "Servika update",
		"/bin/bash", "-lc", fmt.Sprintf("%s >>%s 2>&1", updateScript, updateLogPath))
	if _, err := command.CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "the update could not be started")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"started":         true,
		"tool_downloaded": toolDownloaded,
	})
}

// UpdateLog returns the tail of the update log and the normalized execution state.
func UpdateLog(w http.ResponseWriter, _ *http.Request) {
	body, err := os.ReadFile(updateLogPath)
	if err != nil {
		body = nil
	}
	logText := string(body)
	if len(logText) > 60000 {
		logText = logText[len(logText)-60000:]
	}
	running := updateRunning()
	status := "idle"
	if running {
		status = "running"
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"log":     logText,
		"running": running,
		"status":  status,
	})
}
