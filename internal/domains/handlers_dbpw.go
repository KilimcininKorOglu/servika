package domains

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"servika/internal/credentials"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type setDBPwReq struct {
	Password string `json:"password"`
}

// SetDatabasePassword handles PUT /api/v1/databases/:dbid/password.
// It generates a random password when the request body is empty and rejects demo subscriptions.
func (h *Handlers) SetDatabasePassword(w http.ResponseWriter, r *http.Request) {
	dbid, _ := strconv.ParseInt(chi.URLParam(r, "dbid"), 10, 64)
	var req setDBPwReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Password == "" {
		req.Password = credentials.RandomPassword(24)
	}
	if len(req.Password) < 6 {
		httpx.WriteError(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}

	var dbName, dbUser string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db.db_name, db.db_user, d.is_demo
		 FROM db_accounts db JOIN domains d ON d.id=db.domain_id
		 WHERE db.id=?`, dbid).Scan(&dbName, &dbUser, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "database record not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database read failed")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "database passwords cannot be changed for demo subscriptions")
		return
	}

	if err := credentials.MySQLChangePassword(h.DB, dbUser, req.Password); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "password change failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"dbid":    dbid,
		"db_name": dbName,
		"db_user": dbUser,
		"db_pass": req.Password,
	})
}
