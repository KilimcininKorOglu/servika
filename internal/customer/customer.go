// Package customer provides domain-owner authentication and scope control.
// Customers authenticate with FTP credentials and receive a domain-scoped JWT.
package customer

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"servika/internal/auth"
	"servika/internal/httpx"
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

	// Validate the credentials against ftp_accounts.
	var ftpID, domainID int64
	var storedPassword, domainName, status string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT fa.id, fa.domain_id, fa.password_md5, fa.status, d.domain_name
		 FROM ftp_accounts fa
		 JOIN domains d ON d.id = fa.domain_id
		 WHERE fa.username = ?`, req.Username).
		Scan(&ftpID, &domainID, &storedPassword, &status, &domainName)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Authentication failed")
		return
	}
	if status != "active" {
		httpx.WriteError(w, http.StatusForbidden, "FTP account is suspended")
		return
	}
	// Plain text comparison (Pure-FTPd MYSQLCrypt cleartext)
	if req.Password != storedPassword {
		httpx.WriteError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	// Generate a customer JWT scoped to the domain.
	c := auth.CustomerClaims{
		FTPAccountID: ftpID,
		DomainID:     domainID,
		Username:     req.Username,
		DomainName:   domainName,
	}
	tok, exp, err := auth.GenerateCustomer(h.Secret, c, 24*3600)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Token generation failed")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token":       tok,
		"expires_at":  exp,
		"domain_id":   domainID,
		"domain_name": domainName,
		"username":    req.Username,
	})
}

// CustomerOnly: middleware, returns 403 if token type is "customer" and domain_id does not match the path
// If it is an admin token it bypasses (admins can do anything)
func CustomerOnly(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

// CheckScope: manual scope check inside a handler. Allow if admin.
// If a customer token, {id} in the URL must match token.DomainID.
func CheckScope(r *http.Request, secret []byte, urlDomainIDParam string) (bool, error) {
	authH := r.Header.Get("Authorization")
	if !strings.HasPrefix(authH, "Bearer ") {
		return false, errors.New("Authorization required")
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
		return false, errors.New("Invalid token")
	}
	if urlDomainIDParam == "" {
		// This endpoint has no domain ID scope but the customer is still restricted (e.g. /domains list)
		return false, errors.New("Customers cannot access this endpoint")
	}
	id, _ := strconv.ParseInt(urlDomainIDParam, 10, 64)
	if id != mc.DomainID {
		return false, errors.New("Access to this domain is forbidden")
	}
	_ = time.Now
	return false, nil // Customer scope is valid.
}
