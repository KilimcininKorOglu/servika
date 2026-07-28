// Package pma provides one-time phpMyAdmin SSO token creation and redemption.
package pma

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"servika/internal/config"
	"servika/internal/credentials"
	"servika/internal/httpx"
	"servika/internal/middleware"

	"github.com/go-chi/chi/v5"
)

// Handlers provides HTTP handlers for phpMyAdmin SSO.
type Handlers struct {
	DB *sql.DB
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// internalAuthToken returns the static token used by signon.php to call the panel.
// It denies access when the token file is unavailable.
func internalAuthToken() string {
	b, err := os.ReadFile(config.PMATokenPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// RequestToken creates a short-lived token for opening phpMyAdmin through pma-signon.php.
// URL: POST /api/v1/databases/{dbId}/pma-token
func (h *Handlers) RequestToken(w http.ResponseWriter, r *http.Request) {
	dbID, _ := strconv.ParseInt(chi.URLParam(r, "dbId"), 10, 64)

	// Read database details and join the domain for the demo check.
	var dbUser, dbPassword, dbName string
	var domainID int64
	var demo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db.db_user, db.db_pass_plain, db.db_name, db.domain_id, d.is_demo
		 FROM db_accounts db JOIN domains d ON d.id=db.domain_id
		 WHERE db.id=?`, dbID).Scan(&dbUser, &dbPassword, &dbName, &domainID, &demo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	if !middleware.DomainOwnedBy(r, domainID) {
		httpx.WriteError(w, http.StatusNotFound, "database not found")
		return
	}
	// db_pass_plain is encrypted at rest (bound to db_user). Decrypt to the real
	// password before minting the short-lived signon token. Legacy plaintext rows
	// pass through unchanged.
	if pw, derr := credentials.DecryptDBPass(dbUser, dbPassword); derr == nil {
		dbPassword = pw
	} else {
		httpx.WriteError(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	// This route is keyed by dbId, so CustomerScope (which reads "id") cannot gate it.
	// Apply the same suspended-domain check here so a suspended customer cannot mint a
	// phpMyAdmin signon token that bypasses the suspension boundary.
	if !middleware.EnforceCustomerNotSuspended(w, r, domainID) {
		return
	}
	if demo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "phpMyAdmin is unavailable for demo subscriptions")
		return
	}

	token, err := randomHex(24)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create a secure signon token")
		return
	}
	expires := time.Now().Add(2 * time.Minute) // Two-minute validity window, for the response only.

	// Write expires_at with the MySQL clock. A Go-side time.Now() (UTC in the driver) mixed
	// with a server-local NOW() makes a fresh token look already expired on hosts whose MySQL
	// session timezone is not UTC, so the cleanup below deletes it instantly and redeem 404s.
	_, err = h.DB.ExecContext(r.Context(),
		`INSERT INTO pma_tokens(token, domain_id, db_user, db_pass, db_name, expires_at)
		 VALUES(?,?,?,?,?, DATE_ADD(NOW(), INTERVAL 120 SECOND))`,
		token, domainID, dbUser, dbPassword, dbName)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database operation failed")
		return
	}

	// Delete expired and used tokens on each request.
	_, _ = h.DB.ExecContext(r.Context(),
		`DELETE FROM pma_tokens WHERE expires_at < NOW() OR used=1`)

	// The token is delivered to pma-signon.php in a POST body, never in a URL, so it
	// cannot leak through browser history, proxy access logs, or Referer headers.
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"token":            token,
		"expires_at":       expires.Format(time.RFC3339),
		"validity_seconds": 120,
	})
}

// Redeem validates internal authentication, returns credentials as JSON, and consumes the token once.
// URL: POST /api/v1/internal/pma-redeem  (X-Internal-Auth header)
func (h *Handlers) Redeem(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("X-Internal-Auth")
	expected := internalAuthToken()
	if expected == "" || auth == "" || subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		httpx.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	var dbUser, dbPassword, dbName string
	var used, expired int
	// Evaluate expiry with the MySQL clock so it matches how expires_at was written and how
	// the consume UPDATE below compares it. A Go-side comparison can reject a valid token.
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db_user, db_pass, db_name, used, (expires_at < NOW())
		 FROM pma_tokens WHERE token=?`, req.Token).
		Scan(&dbUser, &dbPassword, &dbName, &used, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "token not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	if used == 1 {
		httpx.WriteError(w, http.StatusGone, "token has already been used")
		return
	}
	if expired == 1 {
		httpx.WriteError(w, http.StatusGone, "token has expired")
		return
	}

	result, err := h.DB.ExecContext(r.Context(),
		`UPDATE pma_tokens SET used=1 WHERE token=? AND used=0 AND expires_at >= NOW()`, req.Token)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	consumed, err := result.RowsAffected()
	if err != nil || consumed != 1 {
		httpx.WriteError(w, http.StatusGone, "token is no longer valid")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"username": dbUser,
		"password": dbPassword,
		"db":       dbName,
		"host":     "localhost",
	})
}
