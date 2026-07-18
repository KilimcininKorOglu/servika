// Package users provides authenticated user profile handlers.
package users

import (
	"database/sql"
	"net/http"

	"servika/internal/httpx"
	"servika/internal/middleware"
)

// Handlers provides user profile HTTP handlers.
type Handlers struct {
	DB *sql.DB
}

type meResp struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Status   string `json:"status"`
	TwoFA    bool   `json:"two_fa"`
	Theme    string `json:"pref_theme"`
	Lang     string `json:"pref_lang"`
}

// Me returns the authenticated user's profile.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	// Return a synthetic profile from claims for customer FTP sessions without a database lookup.
	if customerClaims := middleware.CustomerClaimsFrom(r); customerClaims != nil {
		httpx.WriteJSON(w, http.StatusOK, meResp{
			ID:       0,
			Name:     customerClaims.Username,
			Role:     "customer",
			FullName: customerClaims.DomainName,
			Status:   "active",
		})
		return
	}
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	var resp meResp
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, username, role, email, full_name, status, totp_enabled, pref_theme, pref_lang FROM users WHERE id=?`,
		c.UserID).Scan(&resp.ID, &resp.Name, &resp.Role, &resp.Email, &resp.FullName, &resp.Status, &resp.TwoFA, &resp.Theme, &resp.Lang)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "user profile could not be read")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}
