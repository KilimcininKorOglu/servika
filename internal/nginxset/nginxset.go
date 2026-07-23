// Package nginxset manages per-domain nginx security headers, caching, and custom directives.
package nginxset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"servika/internal/httpx"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// Settings contains per-domain nginx configuration.
type Settings struct {
	HdrXContentType bool `json:"hdr_x_content_type"`
	HdrXXSS         bool `json:"hdr_x_xss"`
	HdrReferrer     bool `json:"hdr_referrer"`
	HdrPermissions  bool `json:"hdr_permissions"`
	HdrCSPUpgrade   bool `json:"hdr_csp_upgrade"`
	HdrHSTS         bool `json:"hdr_hsts"`
	HSTSMaxAge      int  `json:"hsts_max_age"`
	HSTSSubdomains  bool `json:"hsts_subdomains"`
	HSTSPreload     bool `json:"hsts_preload"`

	// Performance caching.
	FastCgiCache        bool `json:"fastcgi_cache"`
	FastCgiCacheMinutes int  `json:"fastcgi_cache_minutes"`
	BrowserCache        bool `json:"browser_cache"`
	BrowserCacheDays    int  `json:"browser_cache_days"`

	ExtraDirectives string `json:"extra_directives"`
}

// Defaults returns the default nginx settings.
func Defaults() Settings {
	return Settings{
		HdrXContentType: true, HdrXXSS: true, HdrReferrer: true,
		HdrPermissions: true, HdrCSPUpgrade: true, HdrHSTS: true,
		HSTSMaxAge: 31536000, HSTSSubdomains: true, HSTSPreload: false,
		FastCgiCache: false, FastCgiCacheMinutes: 60,
		BrowserCache: true, BrowserCacheDays: 30,
		ExtraDirectives: "",
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Get returns nginx settings for a domain.
func Get(ctx context.Context, db *sql.DB, domainID int64) (Settings, error) {
	s := Defaults()
	var b1, b2, b3, b4, b5, b6, b7, b8, bFC, bBC int
	err := db.QueryRowContext(ctx,
		`SELECT hdr_x_content_type, hdr_x_xss, hdr_referrer, hdr_permissions,
		        hdr_csp_upgrade, hdr_hsts, hsts_max_age, hsts_subdomains, hsts_preload,
		        extra_directives, fastcgi_cache, fastcgi_cache_minutes,
		        browser_cache, browser_cache_days
		 FROM nginx_settings WHERE domain_id=?`, domainID).
		Scan(&b1, &b2, &b3, &b4, &b5, &b6, &s.HSTSMaxAge, &b7, &b8,
			&s.ExtraDirectives, &bFC, &s.FastCgiCacheMinutes, &bBC, &s.BrowserCacheDays)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.HdrXContentType = b1 == 1
	s.HdrXXSS = b2 == 1
	s.HdrReferrer = b3 == 1
	s.HdrPermissions = b4 == 1
	s.HdrCSPUpgrade = b5 == 1
	s.HdrHSTS = b6 == 1
	s.HSTSSubdomains = b7 == 1
	s.HSTSPreload = b8 == 1
	s.FastCgiCache = bFC == 1
	s.BrowserCache = bBC == 1
	return s, nil
}

// Save persists nginx settings for a domain.
func Save(ctx context.Context, db *sql.DB, domainID int64, s Settings) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO nginx_settings(domain_id, hdr_x_content_type, hdr_x_xss, hdr_referrer,
		    hdr_permissions, hdr_csp_upgrade, hdr_hsts, hsts_max_age, hsts_subdomains, hsts_preload,
		    extra_directives, fastcgi_cache, fastcgi_cache_minutes, browser_cache, browser_cache_days)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		    hdr_x_content_type=VALUES(hdr_x_content_type),
		    hdr_x_xss=VALUES(hdr_x_xss),
		    hdr_referrer=VALUES(hdr_referrer),
		    hdr_permissions=VALUES(hdr_permissions),
		    hdr_csp_upgrade=VALUES(hdr_csp_upgrade),
		    hdr_hsts=VALUES(hdr_hsts),
		    hsts_max_age=VALUES(hsts_max_age),
		    hsts_subdomains=VALUES(hsts_subdomains),
		    hsts_preload=VALUES(hsts_preload),
		    extra_directives=VALUES(extra_directives),
		    fastcgi_cache=VALUES(fastcgi_cache),
		    fastcgi_cache_minutes=VALUES(fastcgi_cache_minutes),
		    browser_cache=VALUES(browser_cache),
		    browser_cache_days=VALUES(browser_cache_days)`,
		domainID, b2i(s.HdrXContentType), b2i(s.HdrXXSS), b2i(s.HdrReferrer),
		b2i(s.HdrPermissions), b2i(s.HdrCSPUpgrade), b2i(s.HdrHSTS),
		s.HSTSMaxAge, b2i(s.HSTSSubdomains), b2i(s.HSTSPreload),
		s.ExtraDirectives, b2i(s.FastCgiCache), s.FastCgiCacheMinutes,
		b2i(s.BrowserCache), s.BrowserCacheDays)
	return err
}

// Handlers provides HTTP handlers for nginx settings.
type Handlers struct {
	DB *sql.DB
}

type customVhostResponse struct {
	Enabled    bool   `json:"enabled"`
	Content    string `json:"content"`
	DomainName string `json:"domain_name"`
}

type setCustomVhostRequest struct {
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

// ShowCustomVhost returns the raw custom vhost settings for a domain.
func (h *Handlers) ShowCustomVhost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName string
	var enabled int
	var content string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, COALESCE(custom_vhost_enabled,0), COALESCE(custom_vhost_content,'') FROM domains WHERE id=?`, id).
		Scan(&domainName, &enabled, &content)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load custom vhost")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, customVhostResponse{
		Enabled:    enabled == 1,
		Content:    content,
		DomainName: domainName,
	})
}

// SaveCustomVhost persists and applies the raw custom vhost for a domain.
func (h *Handlers) SaveCustomVhost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setCustomVhostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var previousEnabled int
	var previousContent, domainName, systemUser, phpVersion string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, php_version, COALESCE(custom_vhost_enabled,0), COALESCE(custom_vhost_content,'') FROM domains WHERE id=?`, id).
		Scan(&domainName, &systemUser, &phpVersion, &previousEnabled, &previousContent)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load domain")
		return
	}

	if req.Enabled {
		if err := provisioner.ValidateCustomVhost(req.Content); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid custom vhost: "+err.Error())
			return
		}
	}

	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET custom_vhost_enabled=?, custom_vhost_content=? WHERE id=?`,
		b2i(req.Enabled), req.Content, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save custom vhost")
		return
	}

	socket, err := provisioner.PHPSocketFor(systemUser, phpVersion)
	if err != nil {
		socket = "/run/php-fpm/" + systemUser + ".sock"
	}
	if err := provisioner.ApplyVhostForDomain(h.DB, id, socket, phpVersion); err != nil {
		if _, rollbackErr := h.DB.ExecContext(r.Context(),
			`UPDATE domains SET custom_vhost_enabled=?, custom_vhost_content=? WHERE id=?`,
			previousEnabled, previousContent, id); rollbackErr != nil {
			log.Printf("custom vhost rollback failed for domain %d: %v", id, rollbackErr)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to apply custom vhost")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, customVhostResponse{
		Enabled:    req.Enabled,
		Content:    req.Content,
		DomainName: domainName,
	})
}

// Show returns nginx settings for a domain.
func (h *Handlers) Show(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name FROM domains WHERE id=?`, id).Scan(&domainName); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	s, err := Get(r.Context(), h.DB, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load nginx settings")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"domain_name": domainName,
		"settings":    s,
	})
}

// Save persists and applies nginx settings for a domain.
func (h *Handlers) Save(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req struct {
		Settings Settings `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var phpVersion, systemUser string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT php_version, system_user FROM domains WHERE id=?`, id).
		Scan(&phpVersion, &systemUser); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if directive := provisioner.DangerousNginxDirective(req.Settings.ExtraDirectives); directive != "" {
		httpx.WriteError(w, http.StatusBadRequest, "nginx directive is not allowed")
		return
	}
	if err := provisioner.ValidateNginxDirectives(req.Settings.ExtraDirectives); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid nginx directives")
		return
	}
	if err := Save(r.Context(), h.DB, id, req.Settings); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save nginx settings")
		return
	}
	socket, err := provisioner.PHPSocketFor(systemUser, phpVersion)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to resolve PHP socket")
		return
	}
	if err := provisioner.ApplyVhostForDomain(h.DB, id, socket, phpVersion); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to apply nginx virtual host")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
