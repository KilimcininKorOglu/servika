package system

import (
	"log"
	"net/http"
	"os/exec"

	"servika/internal/httpx"
)

// Reboot requests a host reboot through systemd without exposing command output.
// `systemctl reboot` returns quickly (shutdown is async), so a non-zero exit is a
// genuine failure (polkit denial, dbus unavailable) and must not be reported as
// success — otherwise a pending reboot (quota activation, patched kernel) silently
// never happens while the operator believes it was issued.
func Reboot(w http.ResponseWriter, _ *http.Request) {
	if out, err := exec.Command("systemctl", "reboot").CombinedOutput(); err != nil {
		log.Printf("systemctl reboot failed: %v: %s", err, out)
		httpx.WriteError(w, http.StatusInternalServerError, "reboot could not be initiated")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}
