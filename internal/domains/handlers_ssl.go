package domains

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"servika/internal/httpx"
	"servika/internal/mail"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

type sslIssueReq struct {
	Type string `json:"type"` // "self-signed" | "letsencrypt"
	// MailSSL also orders a certificate for the domain's mail hostnames and
	// serves it to mail clients through SNI. Only meaningful with letsencrypt: a
	// self-signed mail certificate warns exactly as the shared one already does.
	MailSSL bool `json:"mail_ssl,omitempty"`
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
	// Any other Scan error has to stop the request here. It cannot be checked
	// further down because the switch below assigns to err, which would discard
	// it; and continuing means calling the provisioner with an empty domain name
	// and system user while isDemo still reads 0, so the demo guard above is
	// bypassed as well.
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database read failed")
		return
	}
	if domainName == "" || systemUser == "" {
		httpx.WriteError(w, http.StatusInternalServerError, "domain record is incomplete")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "SSL cannot be installed for demo subscriptions")
		return
	}

	var certPath, keyPath string
	var outcome provisioner.IssueOutcome
	actualType := req.Type
	switch req.Type {
	case "self-signed":
		certPath, keyPath, err = provisioner.EnableSelfSigned(domainName, systemUser, phpVersion, backend)
	case "letsencrypt":
		certPath, keyPath, outcome, err = provisioner.EnableLetsEncrypt(domainName, systemUser, phpVersion, backend)
		if !outcome.Real {
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

	// Written on a context of its own, not the request's.
	//
	// Issuance can outlast the client: an ACME order plus the mail certificate
	// takes longer than a browser is willing to wait, and an abandoned request
	// cancels r.Context(). The certificate is already installed and nginx is
	// already serving it at this point, so losing this write would leave the
	// panel reporting a domain as unprotected while the disk says otherwise, and
	// nothing would correct it until the next startup heal.
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelWrite()
	if _, err := h.DB.ExecContext(writeCtx,
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
	// The mail certificate is a SEPARATE order, so it cannot regress the web
	// certificate that was just installed. A failure here is reported as its own
	// field rather than failing the request: the site is already secured, and
	// saying otherwise would be a lie about work that succeeded.
	if req.MailSSL && actualType == "letsencrypt" {
		mailCert, mailErr := provisioner.IssueMailCertificate(domainName)
		switch {
		case mailErr != nil:
			response["mail_ssl_error"] = "mail_certificate_failed"
			if len(mailCert.Skipped) > 0 {
				response["mail_ssl_skipped"] = mailCert.Skipped
			}
			log.Printf("mail certificate for %s: %v", domainName, mailErr)
		default:
			response["mail_ssl"] = map[string]any{
				"hosts":      mailCert.Hosts,
				"expires_at": mailCert.ExpiresAt,
			}
			if len(mailCert.Skipped) > 0 {
				response["mail_ssl_skipped"] = mailCert.Skipped
			}
			if e := mail.ApplySNI(); e != nil {
				// The certificate exists but nothing serves it yet, which is a
				// different situation from not having one, so it gets its own code.
				response["mail_ssl_error"] = "mail_sni_apply_failed"
				log.Printf("applying the mail SNI configuration for %s: %v", domainName, e)
			}
		}
	}

	// A name left out of the SAN is not a failure, so it is reported alongside a
	// successful issuance too: the certificate is real and the site is secured,
	// but the coverage the customer expected is not all there, and without this
	// the only symptom is a mail client that keeps asking for a password.
	if len(outcome.Skipped) > 0 {
		response["web_ssl_skipped"] = outcome.Skipped
	}
	if req.Type == "letsencrypt" && actualType != "letsencrypt" {
		response["warning"] = "letsencrypt_fallback"
		// The reason is what makes the warning actionable. Without it the panel
		// reports a certificate was installed while the browser reports the site
		// is not secure, and nothing on screen connects the two.
		if outcome.Reason != "" {
			response["reason"] = outcome.Reason
		}
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
	// Same class as SSLIssue: a swallowed Scan error would take DisableSSL an
	// empty domain name, and would leave the demo guard below reading 0.
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database read failed")
		return
	}
	if domainName == "" || systemUser == "" {
		httpx.WriteError(w, http.StatusInternalServerError, "domain record is incomplete")
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
	// Detached for the same reason as the issue path: the vhost has already been
	// rewritten and nginx reloaded, so a lost write would leave the panel
	// claiming SSL is on for a site now serving plain HTTP.
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelWrite()
	if _, err := h.DB.ExecContext(writeCtx,
		`UPDATE domains SET ssl_enabled=0, ssl_source='', cert_path='', key_path='', ssl_expiry=NULL
		 WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
