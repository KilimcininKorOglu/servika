package domains

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

type domainRedirect struct {
	TargetURL  string `json:"target_url"`
	StatusCode int    `json:"status_code"`
}

func (h *Handlers) RedirectStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var redirect domainRedirect
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT target_url, status_code FROM domain_redirects WHERE domain_id=?`, id).
		Scan(&redirect.TargetURL, &redirect.StatusCode)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "redirect settings could not be read")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"active": true, "target_url": redirect.TargetURL, "status_code": redirect.StatusCode})
}

func (h *Handlers) SetRedirect(w http.ResponseWriter, r *http.Request) {
	id, systemUser, phpVersion, demo, ok := h.redirectDomainInfo(w, r)
	if !ok {
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "redirects cannot be changed on demo subscriptions")
		return
	}
	var req domainRedirect
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	targetURL, err := cleanRedirectTarget(req.TargetURL)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid target URL")
		return
	}
	if req.StatusCode != 301 && req.StatusCode != 302 {
		httpx.WriteError(w, http.StatusBadRequest, "status_code must be 301 or 302")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO domain_redirects(domain_id, target_url, status_code) VALUES(?,?,?)
		 ON DUPLICATE KEY UPDATE target_url=VALUES(target_url), status_code=VALUES(status_code)`,
		id, targetURL, req.StatusCode); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "redirect settings could not be saved")
		return
	}
	if err := h.applyRedirectVhost(id, systemUser, phpVersion); err != nil {
		log.Printf("redirect vhost render warn (domain_id=%d): %v", id, err)
		httpx.WriteError(w, http.StatusInternalServerError, "virtual host update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) DeleteRedirect(w http.ResponseWriter, r *http.Request) {
	id, systemUser, phpVersion, demo, ok := h.redirectDomainInfo(w, r)
	if !ok {
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "redirects cannot be changed on demo subscriptions")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM domain_redirects WHERE domain_id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "redirect settings could not be deleted")
		return
	}
	if err := h.applyRedirectVhost(id, systemUser, phpVersion); err != nil {
		log.Printf("redirect vhost render warn (domain_id=%d): %v", id, err)
		httpx.WriteError(w, http.StatusInternalServerError, "virtual host update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) redirectDomainInfo(w http.ResponseWriter, r *http.Request) (int64, string, string, bool, bool) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var systemUser, phpVersion string
	var demo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, COALESCE(php_version,'8.3'), COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&systemUser, &phpVersion, &demo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return 0, "", "", false, false
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "database read failed")
		return 0, "", "", false, false
	}
	return id, systemUser, phpVersion, demo == 1, true
}

func (h *Handlers) applyRedirectVhost(id int64, systemUser, phpVersion string) error {
	socket, err := provisioner.PHPSocketFor(systemUser, phpVersion)
	if err != nil {
		socket = "/run/php-fpm/" + systemUser + ".sock"
	}
	return provisioner.ApplyVhostForDomain(h.DB, id, socket, phpVersion)
}

func cleanRedirectTarget(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("unsupported URL scheme")
	}
	if strings.ContainsAny(target, "\r\n\t ;{}\\\"") {
		return "", errors.New("unsafe URL")
	}
	return target, nil
}
