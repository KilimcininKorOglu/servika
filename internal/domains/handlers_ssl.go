package domains

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

type sslIssueReq struct {
	Type string `json:"type"` // "self-signed" | "letsencrypt"
}

type sslStatusResp struct {
	Enabled   bool   `json:"active"`
	Source    string `json:"source"`
	ExpiresAt string `json:"expires_at,omitempty"`
	CertPath  string `json:"cert_path,omitempty"`
	KeyPath   string `json:"key_path,omitempty"`
}

func (h *Handlers) SSLStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var enabled int
	var source, certPath, keyPath, expiresAt string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT ssl_enabled, ssl_source, cert_path, key_path,
		   COALESCE(DATE_FORMAT(ssl_expiry,'%Y-%m-%dT%H:%i:%sZ'),'')
		 FROM domains WHERE id=?`, id).
		Scan(&enabled, &source, &certPath, &keyPath, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database read failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sslStatusResp{
		Enabled:   enabled == 1,
		Source:    source,
		ExpiresAt: expiresAt,
		CertPath:  certPath,
		KeyPath:   keyPath,
	})
}

func (h *Handlers) SSLIssue(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req sslIssueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type == "" {
		req.Type = "self-signed"
	}
	if req.Type != "self-signed" && req.Type != "letsencrypt" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid type (self-signed|letsencrypt)")
		return
	}
	var domainName, systemUser, phpVersion, backend string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, php_version, is_demo, COALESCE(web_backend,'php-fpm') FROM domains WHERE id=?`, id).
		Scan(&domainName, &systemUser, &phpVersion, &isDemo, &backend)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "SSL cannot be installed for demo subscriptions")
		return
	}

	var certPath, keyPath string
	actualType := req.Type
	switch req.Type {
	case "self-signed":
		certPath, keyPath, err = provisioner.EnableSelfSigned(domainName, systemUser, phpVersion, backend)
	case "letsencrypt":
		var real bool
		certPath, keyPath, real, err = provisioner.EnableLetsEncrypt(domainName, systemUser, phpVersion, backend)
		if !real {
			actualType = "self-signed"
		}
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "SSL installation failed")
		return
	}

	expiresAt := time.Now().Add(365 * 24 * time.Hour)
	if actualType == "letsencrypt" {
		expiresAt = time.Now().Add(90 * 24 * time.Hour)
	}

	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET ssl_enabled=1, ssl_source=?, cert_path=?, key_path=?, ssl_expiry=?
		 WHERE id=?`, actualType, certPath, keyPath, expiresAt, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database update failed")
		return
	}

	response := map[string]any{
		"ok":         true,
		"id":         id,
		"type":       actualType,
		"cert":       certPath,
		"key":        keyPath,
		"expires_at": expiresAt.Format("2006-01-02"),
	}
	if req.Type == "letsencrypt" && actualType != "letsencrypt" {
		response["warning"] = "Let's Encrypt certificate issuance failed; the site is temporarily protected with a self-signed certificate. Fix DNS and try again."
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handlers) SSLDisable(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName, systemUser, phpVersion, backend string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, php_version, is_demo, COALESCE(web_backend,'php-fpm') FROM domains WHERE id=?`, id).
		Scan(&domainName, &systemUser, &phpVersion, &isDemo, &backend)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo subscriptions cannot be modified")
		return
	}
	if err := provisioner.DisableSSL(domainName, systemUser, phpVersion, backend); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "SSL disable failed")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET ssl_enabled=0, ssl_source='', cert_path='', key_path='', ssl_expiry=NULL
		 WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
