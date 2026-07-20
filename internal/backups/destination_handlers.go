package backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"servika/internal/httpx"
)

// GetDestination returns a domain's backup destination with its password hidden.
func (h *Handlers) GetDestination(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	d, err := readDestination(r.Context(), h.DB, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if d == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"missing": true})
		return
	}
	d.Password = "" // Hide the stored password.
	if d.LastError != "" {
		d.LastError = "upload failed"
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

type destinationRequest struct {
	Type      string `json:"type"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"` // if empty, the current one is kept
	RemoteDir string `json:"remote_dir"`
	Enabled   bool   `json:"active"`
}

// PutDestination creates or updates a domain's backup destination.
func (h *Handlers) PutDestination(w http.ResponseWriter, r *http.Request) {
	id, _, _, demo, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "backup destinations are unavailable for demo subscriptions")
		return
	}
	var req destinationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validType(req.Type) {
		httpx.WriteError(w, http.StatusBadRequest, "type must be ftp or sftp")
		return
	}
	if req.Host == "" || req.Username == "" {
		httpx.WriteError(w, http.StatusBadRequest, "host and username are required")
		return
	}
	if !validHost(req.Host) {
		httpx.WriteError(w, http.StatusBadRequest, "host must be a valid hostname or IPv4/IPv6 address")
		return
	}
	if req.Port == 0 {
		if req.Type == "sftp" {
			req.Port = 22
		} else {
			req.Port = 21
		}
	}
	if req.RemoteDir == "" {
		req.RemoteDir = "/"
	}
	// Was the password sent empty? Keep the current record.
	var existingPassword string
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(password,'') FROM backup_destinations WHERE domain_id=?`, id).Scan(&existingPassword)
	if req.Password == "" {
		req.Password = existingPassword
	}
	if req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "password is required for a new destination")
		return
	}
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	_, err = h.DB.ExecContext(r.Context(),
		`INSERT INTO backup_destinations(domain_id, type, host, port, username, password, remote_dir, enabled)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   type=VALUES(type), host=VALUES(host), port=VALUES(port),
		   username=VALUES(username), password=VALUES(password),
		   remote_dir=VALUES(remote_dir), enabled=VALUES(enabled),
		   last_status='', last_error=''`,
		id, req.Type, req.Host, req.Port, req.Username, req.Password, req.RemoteDir, enabled)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not save backup destination")
		return
	}
	d, _ := readDestination(r.Context(), h.DB, id)
	if d != nil {
		d.Password = "" // Hide the stored password.
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

// DeleteDestination deletes a domain's backup destination.
func (h *Handlers) DeleteDestination(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM backup_destinations WHERE domain_id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete backup destination")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// TestDestination tests supplied destination settings or the stored destination.
func (h *Handlers) TestDestination(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	var d *Destination
	var request destinationRequest
	if json.NewDecoder(r.Body).Decode(&request) == nil && request.Host != "" {
		// Ad-hoc test (test from the UI without saving): if the password is empty, fetch from the DB
		existingPassword := ""
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT COALESCE(password,'') FROM backup_destinations WHERE domain_id=?`, id).Scan(&existingPassword)
		if request.Password == "" {
			request.Password = existingPassword
		}
		port := request.Port
		if port == 0 {
			if request.Type == "sftp" {
				port = 22
			} else {
				port = 21
			}
		}
		dz := request.RemoteDir
		if dz == "" {
			dz = "/"
		}
		d = &Destination{
			DomainID: id, Type: request.Type, Host: request.Host, Port: port,
			Username: request.Username, Password: request.Password, RemoteDir: dz, Enabled: true,
		}
	} else {
		d, err = readDestination(r.Context(), h.DB, id)
		if err != nil || d == nil {
			httpx.WriteError(w, http.StatusBadRequest, "destination is missing or request body is invalid")
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := testConnection(ctx, d); err != nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "connection test failed",
		})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
