// Package phpversion provides dynamic PHP version discovery, installation, and removal.
package phpversion

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"servika/internal/httpx"
)

// SupportedVersions contains supported Remi versions and AppStream PHP 8.3.
var SupportedVersions = []VersionMetadata{
	{"5.6", "56", "remi"},
	{"7.0", "70", "remi"},
	{"7.1", "71", "remi"},
	{"7.2", "72", "remi"},
	{"7.3", "73", "remi"},
	{"7.4", "74", "remi"},
	{"8.0", "80", "remi"},
	{"8.1", "81", "remi"},
	{"8.2", "82", "remi"},
	{"8.3", "", "appstream"}, // Native AppStream version.
	{"8.3", "83", "remi"},
	{"8.4", "84", "remi"},
	{"8.5", "85", "remi"},
}

// VersionMetadata identifies a supported PHP distribution.
type VersionMetadata struct {
	Version  string `json:"version"`
	Code     string `json:"code"`     // Remi package prefix such as "74" or "82".
	Resource string `json:"resource"` // "remi" | "appstream"
}

// Version describes a supported PHP distribution and its runtime state.
type Version struct {
	VersionMetadata
	Loaded      bool   `json:"loaded"`
	PoolDir     string `json:"pool_dir,omitempty"`
	SockDir     string `json:"sock_dir,omitempty"`
	Service     string `json:"service,omitempty"`
	PHPBin      string `json:"php_bin,omitempty"`
	RealVersion string `json:"real_version,omitempty"` // For example, "8.3.31".
	ModuleCount int    `json:"module_count,omitempty"`
	Description string `json:"description,omitempty"`
}

// paths returns installation paths for version metadata.
func paths(m VersionMetadata) (poolDir, sockDir, service, phpBin string) {
	if m.Resource == "appstream" {
		return "/etc/php-fpm.d", "/run/php-fpm", "php-fpm", "/usr/bin/php"
	}
	pre := "/opt/remi/php" + m.Code + "/root"
	return "/etc/opt/remi/php" + m.Code + "/php-fpm.d",
		"/var/opt/remi/php" + m.Code + "/run/php-fpm",
		"php" + m.Code + "-php-fpm",
		pre + "/usr/bin/php"
}

// Discover fills runtime metadata for one version.
func Discover(m VersionMetadata) Version {
	s := Version{VersionMetadata: m}
	s.PoolDir, s.SockDir, s.Service, s.PHPBin = paths(m)
	// Consider the version installed when its PHP binary exists.
	if _, err := os.Stat(s.PHPBin); err == nil {
		s.Loaded = true
		// Read module count and the actual version.
		if out, err := exec.Command(s.PHPBin, "-v").Output(); err == nil {
			line := strings.SplitN(string(out), "\n", 2)[0]
			// "PHP 8.3.31 (cli) ..."
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				s.RealVersion = parts[1]
			}
		}
		if out, err := exec.Command(s.PHPBin, "-m").Output(); err == nil {
			lines := strings.Split(string(out), "\n")
			n := 0
			for _, ln := range lines {
				ln = strings.TrimSpace(ln)
				if ln != "" && !strings.HasPrefix(ln, "[") {
					n++
				}
			}
			s.ModuleCount = n
		}
	}
	if m.Resource == "appstream" {
		s.Description = "System default (AlmaLinux AppStream)"
	} else {
		s.Description = "Remi modular, development/test/legacy"
	}
	return s
}

// AllVersions discovers all supported versions.
func AllVersions() []Version {
	out := make([]Version, 0, len(SupportedVersions))
	for _, m := range SupportedVersions {
		out = append(out, Discover(m))
	}
	// Sort installed versions first, then by version descending.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Loaded != out[j].Loaded {
			return out[i].Loaded
		}
		return compareVersions(out[i].Version, out[j].Version) > 0
	})
	return out
}

func compareVersions(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		ia, ib := 0, 0
		fmt.Sscanf(pa[i], "%d", &ia)
		fmt.Sscanf(pb[i], "%d", &ib)
		if ia != ib {
			return ia - ib
		}
	}
	return 0
}

// DefaultBundle contains extensions for modern PHP versions.
var DefaultBundle = []string{
	"php-fpm",
	"php-cli",
	"php-mysqlnd",
	"php-mbstring",
	"php-bcmath",
	"php-intl",
	"php-gd",
	"php-soap",
	"php-opcache",
	"php-pdo",
	"php-xml",
	"php-zip",
	"php-pgsql",
	"php-ldap",
}

// PackageNames builds package names for a version.
func PackageNames(m VersionMetadata) []string {
	pre := "php"
	if m.Resource == "remi" {
		pre = "php" + m.Code + "-php"
	}
	out := make([]string, 0, len(DefaultBundle))
	for _, p := range DefaultBundle {
		out = append(out, strings.Replace(p, "php", pre, 1))
	}
	return out
}

// ----- HTTP -----

// Handlers provides HTTP handlers for PHP version management.
type Handlers struct {
	DB *sql.DB
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"versions": AllVersions(),
	})
}

type opReq struct {
	Version  string `json:"version"`
	Resource string `json:"resource"`
}

func (h *Handlers) Install(w http.ResponseWriter, r *http.Request) {
	var req opReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var m VersionMetadata
	for _, d := range SupportedVersions {
		if d.Version == req.Version && d.Resource == req.Resource {
			m = d
			break
		}
	}
	if m.Version == "" {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported version")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	args := append([]string{"install", "-y"}, PackageNames(m)...)
	cmd := exec.CommandContext(ctx, "dnf", args...)
	_, err := cmd.CombinedOutput()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			"failed to install PHP packages with DNF")
		return
	}

	// Create the pool directory and default www.conf when absent.
	pd, _, svc, _ := paths(m)
	_ = os.MkdirAll(pd, 0755)
	// Enable www.conf.disabled for Remi when present.
	if m.Resource == "remi" {
		dis := filepath.Join(pd, "www.conf.disabled")
		main := filepath.Join(pd, "www.conf")
		if _, err := os.Stat(dis); err == nil {
			_, _ = os.Stat(main) // Skip when it already exists.
			if _, err := os.Stat(main); err != nil {
				_ = os.Rename(dis, main)
			}
		}
	}
	// Set max_input_vars for large phpMyAdmin and WordPress forms and imports.
	phpdDir := "/etc/php.d"
	if m.Resource == "remi" {
		phpdDir = "/etc/opt/remi/php" + m.Code + "/php.d"
	}
	if err := os.MkdirAll(phpdDir, 0755); err == nil {
		_ = os.WriteFile(filepath.Join(phpdDir, "99-servika-input.ini"),
			[]byte("; Servika: supports large forms and imports (phpMyAdmin, WordPress)\nmax_input_vars = 10000\n"), 0644)
	}

	// Enable and start the PHP-FPM service.
	_, _ = exec.Command("systemctl", "enable", "--now", svc).CombinedOutput()

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":      true,
		"version": req.Version,
		"source":  req.Resource,
	})
}

func (h *Handlers) Remove(w http.ResponseWriter, r *http.Request) {
	var req opReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Resource == "appstream" {
		httpx.WriteError(w, http.StatusForbidden,
			"AppStream PHP is the system default and cannot be removed")
		return
	}
	var m VersionMetadata
	for _, d := range SupportedVersions {
		if d.Version == req.Version && d.Resource == req.Resource {
			m = d
			break
		}
	}
	if m.Version == "" || m.Resource != "remi" {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported version")
		return
	}

	// Check whether any domain uses this version.
	var count int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM domains WHERE php_version=?`, req.Version).Scan(&count)
	if count > 0 {
		httpx.WriteError(w, http.StatusConflict,
			fmt.Sprintf("%d domains use this version; migrate them to another version first", count))
		return
	}

	// Stop PHP-FPM.
	_, _, service, _ := paths(m)
	_, _ = exec.Command("systemctl", "disable", "--now", service).CombinedOutput()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	args := append([]string{"remove", "-y"}, "php"+m.Code+"-*")
	cmd := exec.CommandContext(ctx, "dnf", args...)
	_, err := cmd.CombinedOutput()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			"failed to remove PHP packages with DNF")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}
