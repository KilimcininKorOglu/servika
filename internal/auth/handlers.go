package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"servika/internal/httpx"
)

// Handlers provides HTTP handlers for administrator authentication.
type Handlers struct {
	DB          *sql.DB
	Secret      []byte
	LifetimeSec int
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

type loginResp struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	User      struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		FullName string `json:"full_name"`
	} `json:"user"`
}

// verifyRootPassword reads the root hash from /etc/shadow and compares it via a Python crypt subprocess.
// Supports yescrypt ($y$), sha512crypt ($6$), sha256crypt ($5$), MD5crypt ($1$).
func verifyRootPassword(password string) bool {
	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		return false
	}
	var hash string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "root:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				hash = parts[1]
			}
			break
		}
	}
	if hash == "" || strings.HasPrefix(hash, "!") || strings.HasPrefix(hash, "*") || !strings.HasPrefix(hash, "$") {
		return false
	}
	cmd := exec.Command("python3", "-c",
		"import sys, crypt; p = sys.stdin.read(); sys.stdout.write(crypt.crypt(p, sys.argv[1]))",
		hash)
	cmd.Stdin = strings.NewReader(password)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	computed := strings.TrimSpace(string(out))
	return computed == hash
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "Username and password are required")
		return
	}
	if req.Username != "root" {
		writeAudit(h.DB, 0, req.Username, httpx.ClientIP(r), "auth.login", req.Username, false)
		httpx.WriteError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	if !verifyRootPassword(req.Password) {
		writeAudit(h.DB, 0, req.Username, httpx.ClientIP(r), "auth.login", req.Username, false)
		httpx.WriteError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	// The password is correct; a TOTP code is also required when 2FA is enabled.
	{
		var en int
		var sec string
		_ = h.DB.QueryRow(`SELECT totp_enabled, totp_secret FROM users WHERE id=1`).Scan(&en, &sec)
		if en == 1 {
			if strings.TrimSpace(req.Code) == "" {
				httpx.WriteJSON(w, http.StatusOK, map[string]any{"two_factor_required": true})
				return
			}
			if !TOTPVerify(sec, req.Code) {
				writeAudit(h.DB, 1, "root", httpx.ClientIP(r), "auth.2fa", "root", false)
				httpx.WriteError(w, http.StatusUnauthorized, "Invalid 2FA code")
				return
			}
		}
	}

	const adminUID = int64(1)
	tok, err := Issue(h.Secret, h.LifetimeSec, adminUID, "root", "admin")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Token generation failed")
		return
	}
	writeAudit(h.DB, adminUID, "root", httpx.ClientIP(r), "auth.login", "root", true)

	resp := loginResp{Token: tok, ExpiresAt: time.Now().Add(time.Duration(h.LifetimeSec) * time.Second).Unix()}
	resp.User.ID = adminUID
	resp.User.Name = "root"
	resp.User.Role = "admin"
	var fullName string
	_ = h.DB.QueryRow(`SELECT full_name FROM users WHERE id=1`).Scan(&fullName)
	resp.User.FullName = fullName
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func writeAudit(db *sql.DB, uid int64, username, ip, action, target string, ok bool) {
	var uidVal any
	if uid > 0 {
		uidVal = uid
	}
	okv := 0
	if ok {
		okv = 1
	}
	_, _ = db.Exec(
		`INSERT INTO audit_log(actor_user_id, actor_username, ip, action, target, ok)
		 VALUES(?,?,?,?,?,?)`,
		uidVal, username, ip, action, target, okv)
}
