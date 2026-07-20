package domains

import (
	"database/sql"
	"errors"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// CalculateDisk measures the domain home directory and stores its size in kilobytes.
func (h *Handlers) CalculateDisk(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var systemUser string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, is_demo FROM domains WHERE id=?`, id).
		Scan(&systemUser, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "disk usage cannot be calculated for demo subscriptions")
		return
	}
	if !strings.HasPrefix(systemUser, "c_") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid system user")
		return
	}
	path := "/home/" + systemUser
	out, err := exec.Command("du", "-sb", path).CombinedOutput()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "disk usage calculation failed")
		return
	}
	fields := strings.Fields(string(out))
	if len(fields) < 1 {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read disk usage output")
		return
	}
	byteB, _ := strconv.ParseInt(fields[0], 10, 64)
	kb := byteB / 1024
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET size_kb=? WHERE id=?`, kb, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"id":      id,
		"size_kb": kb,
		"path":    path,
	})
}
