package system

import (
	"net/http"

	"servika/internal/httpx"
)

// Reboot requests a host reboot through systemd without exposing command output.
func Reboot(w http.ResponseWriter, _ *http.Request) {
	runSystemctl("reboot")
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}
