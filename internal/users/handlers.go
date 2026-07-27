// Package users provides authenticated user profile handlers.
package users

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"servika/internal/auth"
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

// ---------- Panel accounts (admin + reseller) ----------
//
// The scope rules live in one place so each handler does not reinterpret them:
//
//   - admin    : sees / manages every account, may create reseller and admin.
//   - reseller : ONLY the accounts below it (users.reseller_id = its own id),
//                and may create accounts in the 'user' role only.
//   - root     : id=1 is untouchable. It cannot be deleted and its role/status
//                cannot change. Its password lives in /etc/shadow and cannot be
//                reset from these endpoints.
//
// Deleting your own account and disabling the last admin are also blocked; both
// lead to a permanent panel lockout.

const rootID = int64(1)

// UserRow is one panel account as returned by List.
type UserRow struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	FullName    string `json:"full_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	ResellerID  *int64 `json:"reseller_id"`
	TwoFA       bool   `json:"two_fa"`
	LastLogin   string `json:"last_login"`
	LastLoginIP string `json:"last_login_ip"`
	CreatedAt   string `json:"created_at"`
}

// authorized reports whether the caller may act on the target user. A target
// that does not exist also returns false, so a reseller cannot probe with a
// nonexistent id to infer whether an account exists.
func (h *Handlers) authorized(r *http.Request, targetID int64) bool {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		return false
	}
	if c.Role == middleware.RoleAdmin {
		return true
	}
	if c.Role != middleware.RoleReseller {
		return false
	}
	var resellerID *int64
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT reseller_id FROM users WHERE id=?`, targetID).Scan(&resellerID); err != nil {
		return false
	}
	return resellerID != nil && *resellerID == c.UserID
}

// List: GET /users
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}

	q := `SELECT id, username, email, full_name, role, status, reseller_id, totp_enabled,
	             COALESCE(DATE_FORMAT(last_login_at,'%Y-%m-%d %H:%i'),''), last_login_ip,
	             COALESCE(DATE_FORMAT(created_at,'%Y-%m-%d'),'')
	      FROM users`
	var arg []any
	if c.Role == middleware.RoleReseller {
		q += ` WHERE reseller_id = ?`
		arg = append(arg, c.UserID)
	}
	q += ` ORDER BY id`

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	defer rows.Close()

	out := make([]UserRow, 0)
	for rows.Next() {
		var s UserRow
		var twoFA int
		if err := rows.Scan(&s.ID, &s.Username, &s.Email, &s.FullName, &s.Role, &s.Status,
			&s.ResellerID, &twoFA, &s.LastLogin, &s.LastLoginIP, &s.CreatedAt); err != nil {
			continue
		}
		s.TwoFA = twoFA == 1
		out = append(out, s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type createReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,31}$`)

// Create: POST /users
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	var b createReq
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	b.Username = strings.ToLower(strings.TrimSpace(b.Username))
	b.Role = strings.TrimSpace(b.Role)

	if !usernamePattern.MatchString(b.Username) {
		httpx.WriteError(w, http.StatusBadRequest,
			"username: 3-32 chars, must start with a letter, may contain letters/digits/_/-")
		return
	}
	// "root" is defined in the system, not the panel DB; a second account with
	// the same name would make the login flow ambiguous.
	if auth.IsRootUser(b.Username) {
		httpx.WriteError(w, http.StatusBadRequest, "this username is reserved")
		return
	}

	// Privilege-escalation guard: a reseller may only create customer accounts.
	switch c.Role {
	case middleware.RoleAdmin:
		if b.Role != middleware.RoleAdmin && b.Role != middleware.RoleReseller && b.Role != middleware.RoleUser {
			httpx.WriteError(w, http.StatusBadRequest, "invalid role")
			return
		}
	case middleware.RoleReseller:
		if b.Role != middleware.RoleUser {
			httpx.WriteError(w, http.StatusForbidden, "a reseller may only create customer accounts")
			return
		}
	default:
		httpx.WriteError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// An account a reseller creates is automatically bound to it; an account an
	// admin creates is unowned (belongs directly to admin).
	var resellerID any
	if c.Role == middleware.RoleReseller {
		resellerID = c.UserID
	}

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO users(username, email, password_hash, role, reseller_id, full_name, status)
		 VALUES(?,?,?,?,?,?, 'active')`,
		b.Username, strings.TrimSpace(b.Email), hash, b.Role, resellerID, strings.TrimSpace(b.FullName))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			httpx.WriteError(w, http.StatusConflict, "this username is already in use")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not create account")
		return
	}
	id, _ := res.LastInsertId()
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.create", b.Username, true)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": id})
}

type updateReq struct {
	Email    *string `json:"email"`
	FullName *string `json:"full_name"`
	Role     *string `json:"role"`
}

// Update: PUT /users/{id}
func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if c == nil || !h.authorized(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	var b updateReq
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if b.Role != nil {
		if id == rootID {
			httpx.WriteError(w, http.StatusForbidden, "the root account's role cannot be changed")
			return
		}
		if c.Role != middleware.RoleAdmin {
			httpx.WriteError(w, http.StatusForbidden, "only an administrator may change roles")
			return
		}
		if *b.Role != middleware.RoleAdmin && *b.Role != middleware.RoleReseller && *b.Role != middleware.RoleUser {
			httpx.WriteError(w, http.StatusBadRequest, "invalid role")
			return
		}
		// The last admin cannot be demoted out of the admin role.
		if *b.Role != middleware.RoleAdmin {
			if only, err := h.lastAdmin(r, id); err != nil || only {
				httpx.WriteError(w, http.StatusForbidden, "the last administrator account cannot be changed")
				return
			}
		}
	}

	if b.Email != nil {
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET email=?, updated_at=NOW() WHERE id=?`, strings.TrimSpace(*b.Email), id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not update")
			return
		}
	}
	if b.FullName != nil {
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET full_name=?, updated_at=NOW() WHERE id=?`, strings.TrimSpace(*b.FullName), id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not update")
			return
		}
	}
	if b.Role != nil {
		if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET role=?, updated_at=NOW() WHERE id=?`, *b.Role, id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not update")
			return
		}
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.update", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// lastAdmin reports whether the given user is the only active admin in the system.
func (h *Handlers) lastAdmin(r *http.Request, id int64) (bool, error) {
	var n int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM users WHERE role='admin' AND status='active' AND id<>?`, id).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// ResetPassword: POST /users/{id}/password — admin/reseller reset (the current
// password is not asked for; to change your own use /me/password).
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if c == nil || !h.authorized(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	if id == rootID {
		httpx.WriteError(w, http.StatusForbidden,
			"the root password is the system password; change it from the Profile screen")
		return
	}
	var b struct {
		New string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	hash, err := auth.HashPassword(b.New)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET password_hash=?, updated_at=NOW() WHERE id=?`, hash, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.password", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// SetStatus: POST /users/{id}/status
func (h *Handlers) SetStatus(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if c == nil || !h.authorized(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	if id == rootID {
		httpx.WriteError(w, http.StatusForbidden, "the root account cannot be suspended")
		return
	}
	if id == c.UserID {
		httpx.WriteError(w, http.StatusForbidden, "you cannot suspend your own account")
		return
	}
	var b struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || (b.Status != "active" && b.Status != "suspended") {
		httpx.WriteError(w, http.StatusBadRequest, "status must be 'active' or 'suspended'")
		return
	}
	if b.Status == "suspended" {
		if only, err := h.lastAdmin(r, id); err != nil || only {
			httpx.WriteError(w, http.StatusForbidden, "the last administrator cannot be suspended")
			return
		}
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET status=?, updated_at=NOW() WHERE id=?`, b.Status, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not change status")
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.status", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Delete: DELETE /users/{id}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if c == nil || !h.authorized(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	if id == rootID {
		httpx.WriteError(w, http.StatusForbidden, "the root account cannot be deleted")
		return
	}
	if id == c.UserID {
		httpx.WriteError(w, http.StatusForbidden, "you cannot delete your own account")
		return
	}
	if only, err := h.lastAdmin(r, id); err != nil || only {
		httpx.WriteError(w, http.StatusForbidden, "the last administrator cannot be deleted")
		return
	}
	// If a reseller is being deleted, the accounts under it are kept (not
	// deleted): the link is cut so no data is lost, and those accounts become
	// owned directly by admin.
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET reseller_id=NULL WHERE reseller_id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not reassign linked accounts")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM users WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete")
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "user.delete", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
