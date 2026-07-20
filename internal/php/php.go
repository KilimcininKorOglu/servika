// Package php manages per-domain PHP settings.
// It provides version listing, pool configuration rendering, and settings CRUD.
package php

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"servika/internal/httpx"
	"servika/internal/phpversion"
	"servika/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// Version describes an installed PHP runtime and its PHP-FPM paths.
type Version struct {
	Version     string `json:"version"`
	PoolDir     string `json:"pool_dir"`
	SockDir     string `json:"sock_dir"`
	Service     string `json:"service"`
	Description string `json:"description"`
}

// InstalledVersions lists the statically supported PHP runtimes.
var InstalledVersions = []Version{
	{Version: "8.3", PoolDir: "/etc/php-fpm.d", SockDir: "/run/php-fpm", Service: "php-fpm", Description: "AppStream · OPcache"},
	{Version: "8.2", PoolDir: "/etc/opt/remi/php82/php-fpm.d", SockDir: "/var/opt/remi/php82/run/php-fpm", Service: "php82-php-fpm", Description: "Remi · Stable"},
	{Version: "7.4", PoolDir: "/etc/opt/remi/php74/php-fpm.d", SockDir: "/var/opt/remi/php74/run/php-fpm", Service: "php74-php-fpm", Description: "Remi · Legacy"},
}

func versionInfo(version string) (Version, bool) {
	// Check the static list first for backward compatibility.
	for _, installedVersion := range InstalledVersions {
		if installedVersion.Version == version {
			return installedVersion, true
		}
	}
	// Discover dynamic versions through phpversion.
	for _, discoveredVersion := range phpversion.AllVersions() {
		if discoveredVersion.Version == version && discoveredVersion.Loaded {
			return Version{
				Version:     discoveredVersion.Version,
				PoolDir:     discoveredVersion.PoolDir,
				SockDir:     discoveredVersion.SockDir,
				Service:     discoveredVersion.Service,
				Description: discoveredVersion.Description,
			}, true
		}
	}
	return Version{}, false
}

// Settings contains all fields on the PHP settings page.
type Settings struct {
	// Performance & Security
	MemoryLimit       string `json:"memory_limit"`
	MaxExecutionTime  int    `json:"max_execution_time"`
	MaxInputTime      int    `json:"max_input_time"`
	PostMaxSize       string `json:"post_max_size"`
	UploadMaxFilesize string `json:"upload_max_filesize"`
	OpcacheEnable     bool   `json:"opcache_enable"`
	DisableFunctions  string `json:"disable_functions"`

	// Common
	DisplayErrors            bool   `json:"display_errors"`
	LogErrors                bool   `json:"log_errors"`
	AllowURLFopen            bool   `json:"allow_url_fopen"`
	FileUploads              bool   `json:"file_uploads"`
	ShortOpenTag             bool   `json:"short_open_tag"`
	ErrorReporting           string `json:"error_reporting"`
	IncludePath              string `json:"include_path"`
	OpenBasedir              string `json:"open_basedir"`
	SessionSavePath          string `json:"session_save_path"`
	MailForceExtraParameters string `json:"mail_force_extra_parameters"`

	// PHP-FPM
	PMStrategy        string `json:"pm_strategy"`
	PMMaxChildren     int    `json:"pm_max_children"`
	PMMaxRequests     int    `json:"pm_max_requests"`
	PMStartServers    int    `json:"pm_start_servers"`
	PMMinSpareServers int    `json:"pm_min_spare_servers"`
	PMMaxSpareServers int    `json:"pm_max_spare_servers"`

	// Additional
	ExtraDirectives string `json:"extra_directives"`
	DebugMode       bool   `json:"debug_mode"`
}

// Defaults returns the default per-domain PHP settings.
func Defaults() Settings {
	return Settings{
		MemoryLimit:       "256M",
		MaxExecutionTime:  30,
		MaxInputTime:      60,
		PostMaxSize:       "64M",
		UploadMaxFilesize: "32M",
		OpcacheEnable:     true,
		DisableFunctions:  "exec,passthru,shell_exec,system,proc_open,popen",
		DisplayErrors:     false,
		LogErrors:         true,
		AllowURLFopen:     true,
		FileUploads:       true,
		ShortOpenTag:      false,
		ErrorReporting:    "E_ALL & ~E_DEPRECATED & ~E_STRICT",
		IncludePath:       ".:/usr/share/php",
		OpenBasedir:       "",
		SessionSavePath:   "",
		PMStrategy:        "ondemand",
		PMMaxChildren:     8,
		PMMaxRequests:     500,
		PMStartServers:    2,
		PMMinSpareServers: 1,
		PMMaxSpareServers: 3,
		ExtraDirectives:   "",
		DebugMode:         false,
	}
}

// Get returns saved settings for a domain or defaults when no record exists.
func Get(ctx context.Context, db *sql.DB, domainID int64) (Settings, error) {
	s := Defaults()
	row := db.QueryRowContext(ctx, `SELECT memory_limit, max_execution_time, max_input_time, post_max_size,
		upload_max_filesize, opcache_enable, disable_functions,
		display_errors, log_errors, allow_url_fopen, file_uploads, short_open_tag,
		error_reporting, include_path, open_basedir, session_save_path, mail_force_extra_parameters,
		pm_strategy, pm_max_children, pm_max_requests, pm_start_servers, pm_min_spare_servers, pm_max_spare_servers,
		extra_directives, debug_mode FROM php_settings WHERE domain_id=?`, domainID)
	err := row.Scan(&s.MemoryLimit, &s.MaxExecutionTime, &s.MaxInputTime, &s.PostMaxSize,
		&s.UploadMaxFilesize, &s.OpcacheEnable, &s.DisableFunctions,
		&s.DisplayErrors, &s.LogErrors, &s.AllowURLFopen, &s.FileUploads, &s.ShortOpenTag,
		&s.ErrorReporting, &s.IncludePath, &s.OpenBasedir, &s.SessionSavePath, &s.MailForceExtraParameters,
		&s.PMStrategy, &s.PMMaxChildren, &s.PMMaxRequests, &s.PMStartServers, &s.PMMinSpareServers, &s.PMMaxSpareServers,
		&s.ExtraDirectives, &s.DebugMode)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil // Return defaults.
	}
	return s, err
}

// Save persists PHP settings for a domain.
func Save(ctx context.Context, db *sql.DB, domainID int64, s Settings) error {
	sanitized, err := sanitizeSettings(s)
	if err != nil {
		return err
	}
	s = sanitized
	_, err = db.ExecContext(ctx,
		`INSERT INTO php_settings(domain_id, memory_limit, max_execution_time, max_input_time, post_max_size,
			upload_max_filesize, opcache_enable, disable_functions,
			display_errors, log_errors, allow_url_fopen, file_uploads, short_open_tag,
			error_reporting, include_path, open_basedir, session_save_path, mail_force_extra_parameters,
			pm_strategy, pm_max_children, pm_max_requests, pm_start_servers, pm_min_spare_servers, pm_max_spare_servers,
			extra_directives, debug_mode)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
			memory_limit=VALUES(memory_limit),
			max_execution_time=VALUES(max_execution_time),
			max_input_time=VALUES(max_input_time),
			post_max_size=VALUES(post_max_size),
			upload_max_filesize=VALUES(upload_max_filesize),
			opcache_enable=VALUES(opcache_enable),
			disable_functions=VALUES(disable_functions),
			display_errors=VALUES(display_errors),
			log_errors=VALUES(log_errors),
			allow_url_fopen=VALUES(allow_url_fopen),
			file_uploads=VALUES(file_uploads),
			short_open_tag=VALUES(short_open_tag),
			error_reporting=VALUES(error_reporting),
			include_path=VALUES(include_path),
			open_basedir=VALUES(open_basedir),
			session_save_path=VALUES(session_save_path),
			mail_force_extra_parameters=VALUES(mail_force_extra_parameters),
			pm_strategy=VALUES(pm_strategy),
			pm_max_children=VALUES(pm_max_children),
			pm_max_requests=VALUES(pm_max_requests),
			pm_start_servers=VALUES(pm_start_servers),
			pm_min_spare_servers=VALUES(pm_min_spare_servers),
			pm_max_spare_servers=VALUES(pm_max_spare_servers),
			extra_directives=VALUES(extra_directives),
			debug_mode=VALUES(debug_mode)`,
		domainID, s.MemoryLimit, s.MaxExecutionTime, s.MaxInputTime, s.PostMaxSize,
		s.UploadMaxFilesize, b2i(s.OpcacheEnable), s.DisableFunctions,
		b2i(s.DisplayErrors), b2i(s.LogErrors), b2i(s.AllowURLFopen), b2i(s.FileUploads), b2i(s.ShortOpenTag),
		s.ErrorReporting, s.IncludePath, s.OpenBasedir, s.SessionSavePath, s.MailForceExtraParameters,
		s.PMStrategy, s.PMMaxChildren, s.PMMaxRequests, s.PMStartServers, s.PMMinSpareServers, s.PMMaxSpareServers,
		s.ExtraDirectives, b2i(s.DebugMode))
	return err
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func onoff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

var extraDirectivePattern = regexp.MustCompile(`^php_(?:value|flag)\[([a-zA-Z0-9_.]+)\]\s*=`)

var prohibitedExtraDirectives = map[string]bool{
	"open_basedir": true, "disable_functions": true, "disable_classes": true,
	"extension": true, "zend_extension": true,
	"auto_prepend_file": true, "auto_append_file": true,
	"error_log": true, "sys_temp_dir": true, "upload_tmp_dir": true,
	"session.save_path": true, "mail.force_extra_parameters": true,
	"curl.cainfo": true, "openssl.capath": true, "include_path": true,
}

func sanitizeExtraDirectives(raw string) (string, error) {
	if strings.ContainsRune(raw, '\x00') {
		return "", errors.New("extra directives contain a NUL character")
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			cleaned = append(cleaned, trimmed)
			continue
		}
		match := extraDirectivePattern.FindStringSubmatch(trimmed)
		if match == nil {
			return "", fmt.Errorf("extra directive line %d is not an allowed php_value or php_flag directive", i+1)
		}
		key := strings.ToLower(match[1])
		if prohibitedExtraDirectives[key] {
			return "", fmt.Errorf("extra directive line %d cannot override %s", i+1, key)
		}
		cleaned = append(cleaned, trimmed)
	}
	return strings.Join(cleaned, "\n"), nil
}

func sanitizeSettings(s Settings) (Settings, error) {
	scalars := map[string]string{
		"memory_limit":                s.MemoryLimit,
		"post_max_size":               s.PostMaxSize,
		"upload_max_filesize":         s.UploadMaxFilesize,
		"disable_functions":           s.DisableFunctions,
		"error_reporting":             s.ErrorReporting,
		"include_path":                s.IncludePath,
		"open_basedir":                s.OpenBasedir,
		"session_save_path":           s.SessionSavePath,
		"mail_force_extra_parameters": s.MailForceExtraParameters,
		"pm_strategy":                 s.PMStrategy,
	}
	for name, value := range scalars {
		if strings.ContainsAny(value, "\r\n\x00") {
			return Settings{}, fmt.Errorf("setting %s contains a line break or NUL character", name)
		}
	}
	cleaned, err := sanitizeExtraDirectives(s.ExtraDirectives)
	if err != nil {
		return Settings{}, err
	}
	s.ExtraDirectives = cleaned
	return s, nil
}

// poolTmpl contains the complete PHP-FPM pool configuration.
var poolTmpl = template.Must(template.New("pool").Funcs(template.FuncMap{"onoff": onoff}).Parse(`[{{.SystemUser}}]
user = {{.SystemUser}}
group = {{.SystemUser}}
listen = {{.SockDir}}/{{.SystemUser}}.sock
listen.owner = nginx
listen.group = nginx
listen.mode = 0660

pm = {{.S.PMStrategy}}
pm.max_children = {{.S.PMMaxChildren}}
pm.max_requests = {{.S.PMMaxRequests}}
pm.start_servers = {{.S.PMStartServers}}
pm.min_spare_servers = {{.S.PMMinSpareServers}}
pm.max_spare_servers = {{.S.PMMaxSpareServers}}
pm.process_idle_timeout = 30s

; ---- Performance & Security ----
php_admin_value[memory_limit] = {{.S.MemoryLimit}}
php_admin_value[max_execution_time] = {{.S.MaxExecutionTime}}
php_admin_value[max_input_time] = {{.S.MaxInputTime}}
php_admin_value[post_max_size] = {{.S.PostMaxSize}}
php_admin_value[upload_max_filesize] = {{.S.UploadMaxFilesize}}
php_admin_value[max_input_vars] = 10000
php_admin_value[disable_functions] = {{.S.DisableFunctions}}

; ---- Common ----
php_admin_flag[log_errors] = {{onoff .S.LogErrors}}
php_admin_flag[allow_url_fopen] = {{onoff .S.AllowURLFopen}}
php_admin_flag[file_uploads] = {{onoff .S.FileUploads}}
php_admin_flag[short_open_tag] = {{onoff .S.ShortOpenTag}}
{{if .S.DebugMode}}; ---- Debug Mode (overrides display_errors/error_reporting) ----
php_admin_flag[display_errors] = on
php_admin_value[error_reporting] = E_ALL
php_admin_value[auto_prepend_file] = /home/{{.SystemUser}}/.servika/debug_prepend.php
{{else}}php_admin_flag[display_errors] = {{onoff .S.DisplayErrors}}
php_admin_value[error_reporting] = {{.S.ErrorReporting}}
{{end}}
php_admin_value[include_path] = {{.S.IncludePath}}
php_admin_value[open_basedir] = {{if .S.OpenBasedir}}{{.S.OpenBasedir}}{{else}}/home/{{.SystemUser}}/:/tmp/{{end}}
{{if .S.MailForceExtraParameters}}php_admin_value[mail.force_extra_parameters] = {{.S.MailForceExtraParameters}}{{end}}
php_admin_value[session.save_path] = {{if .S.SessionSavePath}}{{.S.SessionSavePath}}{{else}}/home/{{.SystemUser}}/tmp{{end}}
php_admin_value[upload_tmp_dir] = /home/{{.SystemUser}}/tmp
php_admin_value[sys_temp_dir] = /home/{{.SystemUser}}/tmp

catch_workers_output = yes

; ---- BEGIN_CUSTOM ----
{{.S.ExtraDirectives}}
; ---- END_CUSTOM ----
`))

// RenderPool generates pool configuration from settings, system user, and socket directory.
func RenderPool(systemUser string, sockDir string, s Settings) (string, error) {
	sanitized, err := sanitizeSettings(s)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = poolTmpl.Execute(&buf, map[string]any{"SystemUser": systemUser, "SockDir": sockDir, "S": sanitized})
	return buf.String(), err
}

// ApplyToFilesystem writes pool configuration, removes old-version pools, and reloads PHP-FPM.
func ApplyToFilesystem(systemUser, version string, s Settings) (socket string, err error) {
	sb, ok := versionInfo(version)
	if !ok {
		return "", fmt.Errorf("unsupported PHP version: %s", version)
	}
	// Remove pools from previous versions.
	for _, other := range InstalledVersions {
		if other.Version == version {
			continue
		}
		old := filepath.Join(other.PoolDir, systemUser+".conf")
		if _, err := os.Stat(old); err == nil {
			_ = os.Remove(old)
			_, _ = exec.Command("systemctl", "reload-or-restart", other.Service).CombinedOutput()
		}
	}

	_ = os.MkdirAll(sb.PoolDir, 0755)
	_ = os.MkdirAll(sb.SockDir, 0755)
	body, err := RenderPool(systemUser, sb.SockDir, s)
	if err != nil {
		return "", err
	}
	poolPath := filepath.Join(sb.PoolDir, systemUser+".conf")
	if err := os.WriteFile(poolPath, []byte(body), 0644); err != nil {
		return "", err
	}
	if out, err := exec.Command("systemctl", "reload-or-restart", sb.Service).CombinedOutput(); err != nil {
		return "", fmt.Errorf("php-fpm reload (%s): %s: %w", sb.Service, strings.TrimSpace(string(out)), err)
	}
	socket = filepath.Join(sb.SockDir, systemUser+".sock")
	return socket, nil
}

// ----- HTTP handlers -----

// Handlers provides HTTP handlers for PHP runtime and settings operations.
type Handlers struct {
	DB *sql.DB
}

// Versions returns dynamically discovered installed versions.
func (h *Handlers) Versions(w http.ResponseWriter, r *http.Request) {
	all := phpversion.AllVersions()
	installed := []Version{}
	seen := map[string]bool{}
	for _, s := range all {
		if !s.Loaded {
			continue
		}
		if seen[s.Version] {
			continue
		}
		seen[s.Version] = true
		description := "Remi · " + s.Description
		if s.Resource == "appstream" {
			description = "AppStream · OPcache"
		}
		installed = append(installed, Version{
			Version:     s.Version,
			PoolDir:     s.PoolDir,
			SockDir:     s.SockDir,
			Service:     s.Service,
			Description: description,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, installed)
}

// GetSettings returns domain PHP settings and the active version.
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName, systemUser, version string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, system_user, php_version FROM domains WHERE id=?`, id).
		Scan(&domainName, &systemUser, &version); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	s, err := Get(r.Context(), h.DB, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load PHP settings")
		return
	}
	// List modules installed for the domain PHP version.
	modules := versionModules(version)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"domain_name": domainName,
		"system_user": systemUser,
		"php_version": version,
		"settings":    s,
		"modules":     modules,
		"versions": func() []Version {
			all := phpversion.AllVersions()
			installed := []Version{}
			seen := map[string]bool{}
			for _, discoveredVersion := range all {
				if !discoveredVersion.Loaded || seen[discoveredVersion.Version] {
					continue
				}
				seen[discoveredVersion.Version] = true
				description := "Remi"
				if discoveredVersion.Resource == "appstream" {
					description = "AppStream"
				}
				installed = append(installed, Version{Version: discoveredVersion.Version, PoolDir: discoveredVersion.PoolDir, SockDir: discoveredVersion.SockDir, Service: discoveredVersion.Service, Description: description})
			}
			return installed
		}(),
	})
}

// PutSettings saves settings and an optional version, then rewrites pool configuration.
func (h *Handlers) PutSettings(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req struct {
		PHPVersion string   `json:"php_version,omitempty"` // Optional. Changes the version when provided.
		Settings   Settings `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var systemUser, version string
	var demo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, php_version, is_demo FROM domains WHERE id=?`, id).
		Scan(&systemUser, &version, &demo); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "pHP settings are fixed for demo subscriptions")
		return
	}
	if req.PHPVersion != "" && req.PHPVersion != version {
		if _, ok := versionInfo(req.PHPVersion); !ok {
			httpx.WriteError(w, http.StatusBadRequest, "unsupported PHP version")
			return
		}
		version = req.PHPVersion
	}

	sanitized, err := sanitizeSettings(req.Settings)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid PHP settings")
		return
	}
	req.Settings = sanitized
	if err := Save(r.Context(), h.DB, id, req.Settings); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save PHP settings")
		return
	}
	var socket string
	provisioner.WriteDebugShim(h.DB, systemUser, id)
	if provisioner.TenantFPMActive(systemUser) {
		socket, err = provisioner.EnableTenantFPM(h.DB, id, systemUser, version)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to apply tenant PHP-FPM configuration")
			return
		}
	} else {
		socket, err = ApplyToFilesystem(systemUser, version, req.Settings)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to apply PHP pool configuration")
			return
		}
		if err := provisioner.ApplyVhostForDomain(h.DB, id, socket, version); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to apply nginx virtual host")
			return
		}
	}

	if req.PHPVersion != "" {
		_, _ = h.DB.ExecContext(r.Context(),
			`UPDATE domains SET php_version=? WHERE id=?`, version, id)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"php_version": version,
		"socket":      socket,
	})
}

// GetDebugLog returns the last 200 lines of the per-domain PHP debug log.
func (h *Handlers) GetDebugLog(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var systemUser string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user FROM domains WHERE id=?`, id).Scan(&systemUser); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	p, err := debugLogPath(systemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	f, openErr := os.Open(p)
	if openErr != nil {
		// File missing or unreadable -- debug may never have been triggered.
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"lines": []string{}})
		return
	}
	defer func() { _ = f.Close() }()
	// DoS-safe: only read the last ~64KB instead of the entire file.
	const tailBytes = 64 * 1024
	var data []byte
	if st, statErr := f.Stat(); statErr == nil && st.Size() > tailBytes {
		buf := make([]byte, tailBytes)
		if _, e := f.ReadAt(buf, st.Size()-tailBytes); e == nil || e == io.EOF {
			if i := bytes.IndexByte(buf, '\n'); i >= 0 {
				buf = buf[i+1:] // skip the partial first line
			}
			data = buf
		}
	} else {
		data, _ = io.ReadAll(f)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}
	const maxLines = 200
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

// ClearDebugLog truncates the per-domain PHP debug log.
func (h *Handlers) ClearDebugLog(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var systemUser string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user FROM domains WHERE id=?`, id).Scan(&systemUser); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	p, err := debugLogPath(systemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.Truncate(p, 0); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to clear debug log")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// debugLogPath returns the per-domain PHP debug log path. The systemUser value
// originates from the domain record and is validated for the c_ prefix, so
// there is no path traversal risk.
func debugLogPath(systemUser string) (string, error) {
	if systemUser == "" || !strings.HasPrefix(systemUser, "c_") {
		return "", fmt.Errorf("invalid system user")
	}
	return "/home/" + systemUser + "/.servika/php_debug.log", nil
}

// versionModules lists modules loaded by PHP-FPM for a version.
func versionModules(version string) []string {
	sb, ok := versionInfo(version)
	if !ok {
		return nil
	}
	// Find the PHP binary.
	phpBin := "/usr/bin/php"
	if sb.Service != "php-fpm" {
		// "php82-php-fpm" -> "/opt/remi/php82/root/usr/bin/php"
		// Extract the service prefix.
		// Query phpversion for the precise path.
		for _, discoveredVersion := range phpversion.AllVersions() {
			if discoveredVersion.Version == version && discoveredVersion.Loaded {
				phpBin = discoveredVersion.PHPBin
				break
			}
		}
	}
	out, err := exec.Command(phpBin, "-m").Output()
	if err != nil {
		return nil
	}
	modules := []string{}
	for _, ln := range strings.Split(string(out), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "[") {
			continue
		}
		modules = append(modules, ln)
	}
	return modules
}
