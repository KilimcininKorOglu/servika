// Package customer provides domain-owner authentication and scope control.
// Customers authenticate with FTP credentials and receive a domain-scoped JWT.
package customer

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"servika/internal/auth"
	"servika/internal/credentials"
	"servika/internal/httpx"
	mw "servika/internal/middleware"
)

type Handlers struct {
	DB     *sql.DB
	Secret []byte
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login: with FTP user/password, returns a JWT for the domain the FTP account belongs to
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

	// PATH 1 — panel account (Phase 5C, users table + bcrypt).
	//
	// Tried first; when nothing matches the request falls through to the legacy
	// FTP path below. MIGRATION BRIDGE: accounts produced by the backfill have
	// an empty password_hash, and an empty hash never matches, so customers
	// without an assigned password fall through to FTP automatically. The
	// moment a password is set from the panel this path takes over — per
	// customer, with no interruption.
	if h.panelAccountLogin(w, r, req, ip) {
		return
	}

	// PATH 2 — legacy FTP identity (Pure-FTPd). Kept for one more release, then
	// removed.
	// Validate the credentials against ftp_accounts.
	var ftpID, domainID, tokenVersion int64
	var storedPassword, domainName, status string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT fa.id, fa.domain_id, fa.password_md5, fa.status, fa.token_version, d.domain_name
		 FROM ftp_accounts fa
		 JOIN domains d ON d.id = fa.domain_id
		 WHERE fa.username = ?`, req.Username).
		Scan(&ftpID, &domainID, &storedPassword, &status, &tokenVersion, &domainName)
	if errors.Is(err, sql.ErrNoRows) {
		auth.WriteAudit(h.DB, 0, req.Username, ip, "customer.login", req.Username, false)
		httpx.WriteError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "authentication failed")
		return
	}
	if status != "active" {
		httpx.WriteError(w, http.StatusForbidden, "FTP account is suspended")
		return
	}
	// Passwords are stored as SHA-512-crypt ($6$) hashes; verify against the hash.
	// A legacy cleartext row (not yet backfilled) never verifies, so it cannot log in.
	if !credentials.VerifyPassword(storedPassword, req.Password) {
		auth.WriteAudit(h.DB, 0, req.Username, ip, "customer.login", req.Username, false)
		httpx.WriteError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	auth.WriteAudit(h.DB, 0, req.Username, ip, "customer.login", req.Username, true)

	// Generate a customer JWT scoped to the domain.
	c := auth.CustomerClaims{
		FTPAccountID: ftpID,
		DomainID:     domainID,
		Username:     req.Username,
		DomainName:   domainName,
		TokenVersion: tokenVersion,
	}
	tok, exp, err := auth.GenerateCustomer(h.Secret, c, 24*3600)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	// Deliver the token only in the HttpOnly session cookie, never in the body.
	httpx.SetSessionCookie(w, r, tok, 24*3600)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"expires_at":  exp,
		"domain_id":   domainID,
		"domain_name": domainName,
		"username":    req.Username,
	})
}

// panelAccountLogin attempts a login with a customer account in the users table.
//
// When it returns false the caller falls through to the legacy FTP path and the
// response is UNWRITTEN. When it returns true the response has been written
// (either success or a definite rejection such as a suspended account).
//
// An unknown username or a password that does not verify returns false
// silently: a customer whose panel account has no password yet must keep
// signing in with the FTP identity (the migration bridge). An empty
// password_hash never matches (see auth.PasswordMatches).
func (h *Handlers) panelAccountLogin(w http.ResponseWriter, r *http.Request, req loginReq, ip string) bool {
	var (
		uid          int64
		hash         string
		role         string
		status       string
		tokenVersion int64
	)
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, password_hash, role, status, token_version FROM users WHERE username=?`,
		req.Username).Scan(&uid, &hash, &role, &status, &tokenVersion)
	if err != nil || role != mw.RoleUser || !auth.PasswordMatches(hash, req.Password) {
		return false
	}
	if status != "active" {
		auth.WriteAudit(h.DB, uid, req.Username, ip, "customer.login", req.Username, false)
		httpx.WriteError(w, http.StatusForbidden, "account is suspended")
		return true
	}

	// The customer account's domains: the scope is not embedded in the token,
	// it is resolved from the chain on each request (see
	// middleware.CustomerUserOwnsDomain). This lookup only tells the UI where to
	// land on first load.
	var firstDomainID int64
	var firstDomainName string
	_ = h.DB.QueryRowContext(r.Context(), `
		SELECT d.id, d.domain_name
		FROM domains d
		JOIN customers c ON c.id = d.customer_id
		WHERE c.user_id = ?
		ORDER BY d.id LIMIT 1`, uid).Scan(&firstDomainID, &firstDomainName)

	tok, err := auth.Issue(h.Secret, 24*3600, uid, req.Username, role, tokenVersion)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token generation failed")
		return true
	}
	auth.WriteAudit(h.DB, uid, req.Username, ip, "customer.login", req.Username, true)
	if _, err := h.DB.Exec(`UPDATE users SET last_login_at=NOW(), last_login_ip=? WHERE id=?`, ip, uid); err != nil {
		log.Printf("customer panel login: last_login update failed for uid=%d: %v", uid, err)
	}

	// Deliver the token only in the HttpOnly session cookie, never in the body.
	httpx.SetSessionCookie(w, r, tok, 24*3600)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"expires_at":    time.Now().Add(24 * time.Hour).Unix(),
		"domain_id":     firstDomainID,
		"domain_name":   firstDomainName,
		"username":      req.Username,
		"panel_account": true,
	})
	return true
}

// CheckScope: manual scope check inside a handler. Allow if admin.
// If a customer token, {id} in the URL must match token.DomainID.
func CheckScope(r *http.Request, secret []byte, urlDomainIDParam string) (bool, error) {
	authH := r.Header.Get("Authorization")
	if !strings.HasPrefix(authH, "Bearer ") {
		return false, errors.New("authorization required")
	}
	raw := strings.TrimPrefix(authH, "Bearer ")
	// Try admin claims first
	if c, err := auth.Parse(secret, raw); err == nil {
		_ = c
		return true, nil // admin
	}
	// Then try customer claims.
	mc, err := auth.ParseCustomer(secret, raw)
	if err != nil {
		return false, errors.New("invalid token")
	}
	if urlDomainIDParam == "" {
		// This endpoint has no domain ID scope but the customer is still restricted (e.g. /domains list)
		return false, errors.New("customers cannot access this endpoint")
	}
	id, _ := strconv.ParseInt(urlDomainIDParam, 10, 64)
	if id != mc.DomainID {
		return false, errors.New("access to this domain is forbidden")
	}
	_ = time.Now
	return false, nil // Customer scope is valid.
}
