// Package sshaccess manages SSH access per hosting account.
// Each domain maps to a c_<slug> Linux user. Access is controlled by switching the user's login shell
// between /bin/bash and /usr/sbin/nologin. The server's sshd configuration has no AllowUsers or
// AllowGroups restrictions, so shell switching is sufficient and does not modify sshd_config or
// risk locking out root or unrelated accounts.
package sshaccess

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"servika/internal/config"
	"servika/internal/credentials"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const (
	enabledShell  = "/bin/bash"
	disabledShell = "/usr/sbin/nologin"
	sshPort       = 22
)

func servikaJailBin() string { return config.OpsTool("servika-jail") }

// Handlers provides HTTP handlers for per-domain SSH access.
type Handlers struct {
	DB   *sql.DB
	IPv4 string
}

type status struct {
	DomainName string `json:"domain_name"`
	Username   string `json:"username"`
	Enabled    bool   `json:"active"`
	Shell      string `json:"shell"`
	SSHHost    string `json:"ssh_host"`
	SSHPort    int    `json:"ssh_port"`
	HasKey     bool   `json:"has_key"`
	IsDemo     bool   `json:"is_demo"`
}

// validSystemUser restricts operations to panel-created c_<slug> users, preventing command injection
// and changes to unrelated accounts.
func validSystemUser(systemUser string) bool {
	if !strings.HasPrefix(systemUser, "c_") || len(systemUser) < 3 {
		return false
	}
	return !strings.ContainsAny(systemUser, "/ .;|&$`\n\r\t\"'")
}

func currentShell(systemUser string) string {
	out, err := exec.Command("getent", "passwd", systemUser).Output()
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(parts) >= 7 {
		return parts[6]
	}
	return ""
}

func hasKey(systemUser string) bool {
	st, err := os.Stat(filepath.Join("/home", systemUser, ".ssh", "authorized_keys"))
	return err == nil && st.Size() > 0
}

func (h *Handlers) load(r *http.Request) (id int64, systemUser, domainName string, demo bool, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, domain_name, is_demo FROM domains WHERE id=?`, id).
		Scan(&systemUser, &domainName, &isDemo)
	if err != nil {
		return id, "", "", false, false
	}
	return id, systemUser, domainName, isDemo == 1, true
}

// GET /domains/{id}/ssh returns the current SSH access state.
func (h *Handlers) Show(w http.ResponseWriter, r *http.Request) {
	_, systemUser, domainName, demo, ok := h.load(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	shell := currentShell(systemUser)
	httpx.WriteJSON(w, http.StatusOK, status{
		DomainName: domainName,
		Username:   systemUser,
		Enabled:    shell == enabledShell,
		Shell:      shell,
		SSHHost:    h.IPv4,
		SSHPort:    sshPort,
		HasKey:     hasKey(systemUser),
		IsDemo:     demo,
	})
}

// PUT /domains/{id}/ssh changes the login shell from {"active": true|false}.
func (h *Handlers) Configure(w http.ResponseWriter, r *http.Request) {
	id, systemUser, _, demo, ok := h.load(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "sSH access cannot be changed for a demo subscription")
		return
	}
	if !validSystemUser(systemUser) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid system user")
		return
	}
	var req struct {
		Enabled bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	shell := disabledShell
	if req.Enabled {
		shell = enabledShell
	}
	if _, err := exec.Command("usermod", "-s", shell, systemUser).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	if req.Enabled {
		// Prepare ~/.ssh for key uploads.
		dir := filepath.Join("/home", systemUser, ".ssh")
		_ = os.MkdirAll(dir, 0700)
		_ = exec.Command("chown", "-R", systemUser+":"+systemUser, dir).Run()
		_ = exec.Command("restorecon", "-R", dir).Run()
		// Synchronize the SSH password with the FTP password.
		_ = credentials.SyncSSHPassword(h.DB, systemUser)
		// Configure the chroot jail and add the user to the restricted SSH access group.
		_ = exec.Command(servikaJailBin(), "setup", systemUser).Run()
		_ = exec.Command("groupadd", "-f", "servika-ssh").Run()
		_ = exec.Command("gpasswd", "-a", systemUser, "servika-ssh").Run()
	} else {
		// When disabling SSH, remove the group membership and jail, then lock the password.
		_ = exec.Command("gpasswd", "-d", systemUser, "servika-ssh").Run()
		_ = exec.Command(servikaJailBin(), "teardown", systemUser).Run()
		_ = credentials.LockSSHPassword(systemUser)
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET ssh_access=? WHERE id=?`, boolToInt(req.Enabled), id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "active": req.Enabled, "shell": shell, "username": systemUser,
	})
}

// PUT /domains/{id}/ssh/key writes authorized_keys from {"key": "ssh-ed25519 ..."}.
func (h *Handlers) SaveKey(w http.ResponseWriter, r *http.Request) {
	_, systemUser, _, demo, ok := h.load(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "the SSH key cannot be changed for a demo subscription")
		return
	}
	if !validSystemUser(systemUser) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid system user")
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	key := strings.TrimSpace(req.Key)
	// Each non-comment line must start with ssh-, ecdsa-, or sk-. Empty input clears all keys.
	if key != "" {
		for line := range strings.SplitSeq(key, "\n") {
			l := strings.TrimSpace(line)
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			if !strings.HasPrefix(l, "ssh-") && !strings.HasPrefix(l, "ecdsa-") && !strings.HasPrefix(l, "sk-") {
				httpx.WriteError(w, http.StatusBadRequest, "invalid SSH key: every line must start with ssh-, ecdsa-, or sk-")
				return
			}
		}
	}
	dir := filepath.Join("/home", systemUser, ".ssh")
	if err := os.MkdirAll(dir, 0700); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	ak := filepath.Join(dir, "authorized_keys")
	body := ""
	if key != "" {
		body = key + "\n"
	}
	if err := os.WriteFile(ak, []byte(body), 0600); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	_ = exec.Command("chown", "-R", systemUser+":"+systemUser, dir).Run()
	_ = exec.Command("restorecon", "-R", dir).Run()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "has_key": key != ""})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// EnsureInfra prepares the SSH jail infrastructure at panel startup on an idempotent, best-effort basis.
//   - It installs servika-jail under /usr/local/bin.
//   - It creates the restricted SSH access group.
//   - It installs the sshd Match chroot configuration and reloads only after `sshd -t` succeeds.
//     Invalid configuration is rolled back so the active sshd setup remains intact.
func EnsureInfra() {
	const srcDir = "/opt/servika/src/scripts"
	// Install the jail script.
	if data, err := os.ReadFile(srcDir + "/servika-jail"); err == nil {
		if e := os.WriteFile(servikaJailBin(), data, 0o755); e == nil {
			_ = os.Chmod(servikaJailBin(), 0o755)
		}
	}
	// Create the restricted SSH access group.
	_ = exec.Command("groupadd", "-f", "servika-ssh").Run()
	// Apply the sshd Match chroot configuration safely.
	dst := "/etc/ssh/sshd_config.d/50-servika-jail.conf"
	src, err := os.ReadFile(srcDir + "/50-servika-jail.conf")
	if err != nil {
		return
	}
	cur, _ := os.ReadFile(dst)
	if string(cur) == string(src) {
		return // The installed configuration is current.
	}
	if e := os.WriteFile(dst, src, 0o644); e != nil {
		log.Printf("could not write jail sshd configuration: %v", e)
		return
	}
	if out, e := exec.Command("sshd", "-t").CombinedOutput(); e != nil {
		// Roll back invalid configuration without disrupting sshd.
		if len(cur) > 0 {
			_ = os.WriteFile(dst, cur, 0o644)
		} else {
			_ = os.Remove(dst)
		}
		log.Printf("jail sshd configuration is invalid and was not applied: %s", strings.TrimSpace(string(out)))
		return
	}
	_ = exec.Command("systemctl", "reload", "sshd").Run()
	log.Printf("SSH jail infrastructure is ready (script + servika-ssh + sshd chroot configuration)")
}
