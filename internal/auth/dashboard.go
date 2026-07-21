package auth

// Dashboard widget layout — per-user, drag-and-drop ordering.
// Stored as JSON text in the users.dashboard_layout column (see migration 0041).
// Follows the same per-user preference pattern as profile.go.

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"servika/internal/httpx"
)

// GET /dashboard-layout — the current user's saved layout (empty string when absent).
func (h *Handlers) DashboardLayoutGet(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	var layout sql.NullString
	if err := h.DB.QueryRow(`SELECT dashboard_layout FROM users WHERE id=?`, c.UserID).Scan(&layout); err != nil && err != sql.ErrNoRows {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read layout")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"layout": layout.String})
}

// PUT /dashboard-layout — save the layout. Body: {"layout":"<json text>"}.
func (h *Handlers) DashboardLayoutSave(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	var b struct {
		Layout string `json:"layout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(b.Layout) > 16384 { // simple upper bound — prevent abuse
		httpx.WriteError(w, http.StatusBadRequest, "layout payload too large")
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET dashboard_layout=?, updated_at=NOW() WHERE id=?`, b.Layout, c.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not save layout")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
