package auth

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
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
	// Token is intentionally omitted: the session JWT is delivered only via the
	// HttpOnly servika_session cookie (see httpx.SetSessionCookie) so JavaScript
	// cannot read it. The body carries only non-secret session metadata.
	ExpiresAt int64 `json:"expires_at"`
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
	for line := range strings.SplitSeq(string(data), "\n") {
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
	ip := httpx.ClientIP(r)

	// Identity resolution: two separate password worlds (see password.go).
	//
	//   root  -> /etc/shadow (yescrypt). This path was DELIBERATELY left
	//            unchanged when adding multi-user support; it is the only way to
	//            keep the risk of locking yourself out of the panel at zero.
	//   other -> users.password_hash (bcrypt), status='active' accounts only.
	//
	// Both branches return the same failure response ("invalid username or
	// password") so which usernames exist is never leaked.
	var (
		uid      int64
		username string
		role     string
		fullName string
	)

	if IsRootUser(req.Username) {
		if !verifyRootPassword(req.Password) {
			WriteAudit(h.DB, 0, req.Username, ip, "auth.login", req.Username, false)
			httpx.WriteError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		uid, username, role = 1, "root", "admin"
		_ = h.DB.QueryRow(`SELECT full_name FROM users WHERE id=1`).Scan(&fullName)
	} else {
		var hash, status string
		err := h.DB.QueryRow(
			`SELECT id, username, password_hash, role, status, full_name FROM users WHERE username=?`,
			req.Username).Scan(&uid, &username, &hash, &role, &status, &fullName)
		if err != nil || !PasswordMatches(hash, req.Password) {
			WriteAudit(h.DB, 0, req.Username, ip, "auth.login", req.Username, false)
			httpx.WriteError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		if status != "active" {
			WriteAudit(h.DB, uid, username, ip, "auth.login", username, false)
			httpx.WriteError(w, http.StatusForbidden, "account is suspended")
			return
		}
		// The customer role cannot open a management-panel session; customers
		// sign in at /customer/login to their own domain panels instead.
		if role != "admin" && role != "reseller" {
			WriteAudit(h.DB, uid, username, ip, "auth.login", username, false)
			httpx.WriteError(w, http.StatusForbidden, "this account cannot sign in to the management panel")
			return
		}
	}

	// The password is correct; a TOTP code is also required when 2FA is enabled.
	// This is now read from the signing-in user's own record (it used to be
	// hardcoded to id=1). FAIL-CLOSED: when 2FA state cannot be read (DB error)
	// login is DENIED (previously the error was swallowed and 2FA was silently
	// skipped = fail-open).
	{
		var en int
		var sec string
		var lastStep int64
		if err := h.DB.QueryRow(`SELECT totp_enabled, totp_secret, totp_last_step FROM users WHERE id=?`, uid).Scan(&en, &sec, &lastStep); err != nil {
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
				WriteAudit(h.DB, uid, username, ip, "auth.2fa", username, false)
				httpx.WriteError(w, http.StatusUnauthorized, "invalid or reused 2FA code")
				return
			}
			// Persist the accepted step for replay protection. FAIL-CLOSED: if this
			// write fails the code would remain replayable within its validity window,
			// so deny the login rather than issuing a token on unguaranteed protection.
			if _, err := h.DB.Exec(`UPDATE users SET totp_last_step=? WHERE id=?`, step, uid); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "could not update 2FA state")
				return
			}
		}
	}

	var tokenVersion int64
	if err := h.DB.QueryRow(`SELECT token_version FROM users WHERE id=?`, uid).Scan(&tokenVersion); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	tok, err := Issue(h.Secret, h.LifetimeSec, uid, username, role, tokenVersion)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	WriteAudit(h.DB, uid, username, ip, "auth.login", username, true)
	// last_login_at uses the MySQL clock and is display-only (never compared to
	// a Go time value), so NOW() is safe here.
	if _, err := h.DB.Exec(`UPDATE users SET last_login_at=NOW(), last_login_ip=? WHERE id=?`, ip, uid); err != nil {
		log.Printf("last_login update failed for uid=%d: %v", uid, err)
	}

	// Deliver the token only in the HttpOnly session cookie, never in the body.
	httpx.SetSessionCookie(w, r, tok, h.LifetimeSec)

	resp := loginResp{ExpiresAt: time.Now().Add(time.Duration(h.LifetimeSec) * time.Second).Unix()}
	resp.User.ID = uid
	resp.User.Name = username
	resp.User.Role = role
	resp.User.FullName = fullName
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// Logout clears the session cookie. It is a public endpoint: expiring a cookie
// requires no authentication and must succeed even when the token is already
// invalid. Server-side revocation for admins is handled separately by
// RevokeSessions (token_version bump).
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	httpx.ClearSessionCookie(w, r)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func WriteAudit(db *sql.DB, uid int64, username, ip, action, target string, ok bool) {
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

// AuditEntry — a security-log row (read-only).
type AuditEntry struct {
	ID       int64  `json:"id"`
	Time     string `json:"time"`
	Username string `json:"username"`
	IP       string `json:"ip"`
	Action   string `json:"action"`
	Target   string `json:"target"`
	OK       bool   `json:"ok"`
}

// AuditList returns audit_log newest-first.
//
// The table has been written to since the first release but there was no read
// endpoint — seeing failed login attempts meant SSHing into the server and
// querying MySQL by hand.
//
// Filters: ?limit=N (default 200, cap 1000), ?action=auth.login,
// ?only_failed=1. A limit is preferred over a date range — this screen's job is
// "what happened recently", not archive analysis.
// auditLimit parses ?limit into the effective row cap: default 200, values
// <=0 or non-numeric fall back to 200, and anything above 1000 clamps to 1000.
func auditLimit(raw string) int {
	limit := 200
	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	return limit
}

// buildAuditQuery assembles the parameterized audit_log SELECT and its args
// from the filters. The action value is always bound as a `?` placeholder (never
// interpolated) and only_failed adds a constant predicate, so user input can
// never reach the SQL text. Kept pure so the filter/injection-safety logic is
// unit-testable without a database.
func buildAuditQuery(action string, onlyFailed bool, limit int) (string, []any) {
	q := `SELECT id, DATE_FORMAT(ts, '%Y-%m-%d %H:%i:%s'), actor_username, ip, action, target, ok
	      FROM audit_log`
	cond := make([]string, 0, 2)
	arg := make([]any, 0, 3)
	if a := strings.TrimSpace(action); a != "" {
		cond = append(cond, "action = ?")
		arg = append(arg, a)
	}
	if onlyFailed {
		cond = append(cond, "ok = 0")
	}
	if len(cond) > 0 {
		q += " WHERE " + strings.Join(cond, " AND ")
	}
	q += " ORDER BY id DESC LIMIT ?"
	arg = append(arg, limit)
	return q, arg
}

func (h *Handlers) AuditList(w http.ResponseWriter, r *http.Request) {
	limit := auditLimit(r.URL.Query().Get("limit"))
	q, arg := buildAuditQuery(
		r.URL.Query().Get("action"),
		r.URL.Query().Get("only_failed") == "1",
		limit,
	)

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "audit list failed")
		return
	}
	defer func() { _ = rows.Close() }() // read-only query: closing the result set has nothing to flush

	out := make([]AuditEntry, 0)
	for rows.Next() {
		var e AuditEntry
		var okv int
		if err := rows.Scan(&e.ID, &e.Time, &e.Username, &e.IP, &e.Action, &e.Target, &okv); err != nil {
			continue
		}
		e.OK = okv == 1
		out = append(out, e)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// AuditActions returns the distinct action names present in the table, to
// populate the filter dropdown (instead of a hardcoded list — new actions show
// up on their own as they are added).
func (h *Handlers) AuditActions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT DISTINCT action FROM audit_log ORDER BY action`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "audit actions failed")
		return
	}
	defer func() { _ = rows.Close() }() // read-only query: closing the result set has nothing to flush
	out := make([]string, 0)
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err == nil {
			out = append(out, a)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
