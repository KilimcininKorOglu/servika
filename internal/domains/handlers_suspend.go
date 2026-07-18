package domains

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"

	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// Suspend marks a domain as suspended and re-renders its vhost.
func (h *Handlers) Suspend(w http.ResponseWriter, r *http.Request) {
	h.setSuspended(w, r, true)
}

// Resume restores a suspended domain and re-renders its vhost.
func (h *Handlers) Resume(w http.ResponseWriter, r *http.Request) {
	h.setSuspended(w, r, false)
}

func (h *Handlers) setSuspended(w http.ResponseWriter, r *http.Request, suspended bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid domain ID")
		return
	}
	var domainName, systemUser, previousStatus string
	var isDemo, previousSuspended int
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, is_demo, status, COALESCE(suspended,0) FROM domains WHERE id=?`, id).
		Scan(&domainName, &systemUser, &isDemo, &previousStatus, &previousSuspended)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		log.Printf("read domain suspension state: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not read domain")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo subscriptions cannot be suspended")
		return
	}

	value := 0
	status := "active"
	if suspended {
		value = 1
		status = "passive"
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET suspended=?, status=? WHERE id=?`, value, status, id); err != nil {
		log.Printf("update domain suspension state: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not update domain")
		return
	}
	if err := provisioner.RerenderVhost(h.DB, id); err != nil {
		if _, rollbackErr := h.DB.ExecContext(r.Context(),
			`UPDATE domains SET suspended=?, status=? WHERE id=?`, previousSuspended, previousStatus, id); rollbackErr != nil {
			log.Printf("rollback domain suspension state: %v", rollbackErr)
		} else if restoreErr := provisioner.RerenderVhost(h.DB, id); restoreErr != nil {
			log.Printf("restore domain vhost after suspension rollback: %v", restoreErr)
		}
		log.Printf("render suspended domain vhost: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not update domain vhost")
		return
	}

	ftpStatus := "active"
	if suspended {
		ftpStatus = "suspended"
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE ftp_accounts SET status=? WHERE domain_id=?`, ftpStatus, id); err != nil {
		log.Printf("update FTP account suspension state for domain %d: %v", id, err)
	}
	if systemUser != "" {
		provisioner.SuspendUserRuntime(systemUser, suspended)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "domain_name": domainName, "suspended": suspended,
	})
}
