package auth

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	yescrypt "github.com/openwall/yescrypt-go"

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

// rootShadowHash reads the root password hash from /etc/shadow ("" = not found).
func rootShadowHash() string {
	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "root:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				return parts[1]
			}
			return ""
		}
	}
	return ""
}

// verifyRootPassword verifies password against the root hash in /etc/shadow.
//
// yescrypt ($y$) — AlmaLinux 10 default — is computed NATIVELY in Go
// (github.com/openwall/yescrypt-go: the yescrypt authors' own implementation).
// This removes the python3 crypt dependency from the PRIMARY path. That module
// was deprecated in Python 3.11 and REMOVED in 3.13 — when the server upgrades
// the panel login would break entirely.
//
// Legacy formats ($6$/$5$/$1$) retain the python3 fallback so login does not
// break on those servers; they should be migrated to native as well.
//
// Comparison uses subtle.ConstantTimeCompare.
func verifyRootPassword(password string) bool {
	hash := rootShadowHash()
	// Locked ("!", "!!", "*") or passwordless account — never accept.
	if len(hash) < 3 || !strings.HasPrefix(hash, "$") {
		return false
	}
	if strings.HasPrefix(hash, "$y$") { // yescrypt → native Go
		computed, err := yescrypt.Hash([]byte(password), []byte(hash))
		if err != nil {
			return false
		}
		return subtle.ConstantTimeCompare(computed, []byte(hash)) == 1
	}
	return pythonCryptVerify(password, hash)
}

// pythonCryptVerify — LEGACY PATH: fallback for non-yescrypt formats only.
// WARNING: python3 crypt module was removed in Python 3.13; this path will not work there.
func pythonCryptVerify(password, hash string) bool {
	cmd := exec.Command("python3", "-c",
		"import sys, crypt; p = sys.stdin.read(); sys.stdout.write(crypt.crypt(p, sys.argv[1]))",
		hash)
	cmd.Stdin = strings.NewReader(password)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(string(out))), []byte(hash)) == 1
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10) // login body over 64KB is abuse (DoS)
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if req.Username != "root" {
		writeAudit(h.DB, 0, req.Username, httpx.ClientIP(r), "auth.login", req.Username, false)
		httpx.WriteError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if !verifyRootPassword(req.Password) {
		writeAudit(h.DB, 0, req.Username, httpx.ClientIP(r), "auth.login", req.Username, false)
		httpx.WriteError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	// The password is correct; a TOTP code is also required when 2FA is enabled.
	// FAIL-CLOSED: when 2FA state cannot be read (DB error) login is DENIED
	// (previously the error was swallowed and 2FA was silently skipped = fail-open).
	{
		var en int
		var sec string
		var lastStep int64
		if err := h.DB.QueryRow(`SELECT totp_enabled, totp_secret, totp_last_step FROM users WHERE id=1`).Scan(&en, &sec, &lastStep); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not verify 2FA state")
			return
		}
		if en == 1 {
			if strings.TrimSpace(sec) == "" {
				httpx.WriteError(w, http.StatusInternalServerError, "2FA configuration is invalid")
				return
			}
			if strings.TrimSpace(req.Code) == "" {
				httpx.WriteJSON(w, http.StatusOK, map[string]any{"two_factor_required": true})
				return
			}
			step, ok := TOTPVerifyStep(sec, req.Code, lastStep)
			if !ok {
				writeAudit(h.DB, 1, "root", httpx.ClientIP(r), "auth.2fa", "root", false)
				httpx.WriteError(w, http.StatusUnauthorized, "invalid or reused 2FA code")
				return
			}
			_, _ = h.DB.Exec(`UPDATE users SET totp_last_step=? WHERE id=1`, step) // replay protection
		}
	}

	const adminUID = int64(1)
	tok, err := Issue(h.Secret, h.LifetimeSec, adminUID, "root", "admin")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token generation failed")
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
	if _, err := db.Exec(
		`INSERT INTO audit_log(actor_user_id, actor_username, ip, action, target, ok)
		 VALUES(?,?,?,?,?,?)`,
		uidVal, username, ip, action, target, okv); err != nil {
		log.Printf("audit log insert failed: %v", err)
	}
}
