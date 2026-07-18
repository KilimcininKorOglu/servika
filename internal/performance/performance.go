// Package performance provides a read-only domain acceleration and performance summary.
// It combines current php_settings and nginx_settings data and generates suggestions.
package performance

import (
	"database/sql"
	"net/http"
	"strconv"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// Handlers provides domain performance HTTP handlers.
type Handlers struct {
	DB *sql.DB
}

// Item describes a performance setting and its current state.
type Item struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"active"`
	Value       string `json:"value"`
	Setting     string `json:"setting"` // Settings page slug.
	Description string `json:"description"`
}

// Suggestion describes a performance improvement recommendation.
type Suggestion struct {
	Text     string `json:"text"`
	Severity string `json:"severity"` // "high" | "medium" | "info"
	Setting  string `json:"setting"`
}

// Summary describes a domain's performance configuration.
type Summary struct {
	DomainName  string       `json:"domain_name"`
	PHPVersion  string       `json:"php_version"`
	Score       int          `json:"score"` // Approximate performance score from 0 to 100.
	Items       []Item       `json:"items"`
	Suggestions []Suggestion `json:"suggestions"`
}

// Show returns the current performance summary for a domain.
func (h *Handlers) Show(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName, phpVersion string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, php_version FROM domains WHERE id=?`, id).Scan(&domainName, &phpVersion); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	summary := Summary{DomainName: domainName, PHPVersion: phpVersion}

	// Use default php_settings when no record exists.
	var opcache, fileUploads int
	var memLimit, pmStrategy string
	var pmMaxChildren, maxExec int
	opcache, memLimit, pmStrategy, pmMaxChildren, maxExec = 1, "256M", "ondemand", 8, 30
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT opcache_enable, memory_limit, pm_strategy, pm_max_children, max_execution_time
		   FROM php_settings WHERE domain_id=?`, id).
		Scan(&opcache, &memLimit, &pmStrategy, &pmMaxChildren, &maxExec)
	_ = fileUploads

	// Use default nginx_settings when no record exists.
	var fastcgi, browserCache, browserCacheDays int
	browserCache, browserCacheDays = 1, 30
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT fastcgi_cache, browser_cache, browser_cache_days FROM nginx_settings WHERE domain_id=?`, id).
		Scan(&fastcgi, &browserCache, &browserCacheDays)

	b := func(i int) bool { return i == 1 }
	summary.Items = []Item{
		{Name: "OPcache", Enabled: b(opcache), Value: statusString(b(opcache)), Setting: "php", Description: "PHP bytecode cache significantly reduces CPU usage."},
		{Name: "FastCGI Cache", Enabled: b(fastcgi), Value: statusString(b(fastcgi)), Setting: "web-server", Description: "nginx caches dynamic PHP output for high-traffic sites."},
		{Name: "Browser Cache", Enabled: b(browserCache), Value: ifElse(b(browserCache), strconv.Itoa(browserCacheDays)+" days", "disabled"), Setting: "web-server", Description: "Long-lived cache headers for static files."},
		{Name: "PHP-FPM Pool", Enabled: true, Value: pmStrategy + " · " + strconv.Itoa(pmMaxChildren) + " workers", Setting: "php", Description: "Worker process management strategy."},
		{Name: "Memory Limit", Enabled: true, Value: memLimit, Setting: "php", Description: "PHP memory_limit."},
	}

	// Calculate the score and suggestions.
	score := 40
	if b(opcache) {
		score += 30
	} else {
		summary.Suggestions = append(summary.Suggestions, Suggestion{Text: "Enable OPcache to improve PHP performance.", Severity: "high", Setting: "php"})
	}
	if b(fastcgi) {
		score += 20
	} else {
		summary.Suggestions = append(summary.Suggestions, Suggestion{Text: "Consider enabling FastCGI Cache for high traffic.", Severity: "medium", Setting: "web-server"})
	}
	if b(browserCache) {
		score += 10
	} else {
		summary.Suggestions = append(summary.Suggestions, Suggestion{Text: "Enable browser caching for static assets.", Severity: "medium", Setting: "web-server"})
	}
	if phpVersion < "8.0" {
		summary.Suggestions = append(summary.Suggestions, Suggestion{Text: "PHP " + phpVersion + " is outdated; PHP 8.3 or later is recommended for speed and security.", Severity: "high", Setting: "php"})
	}
	if len(summary.Suggestions) == 0 {
		summary.Suggestions = append(summary.Suggestions, Suggestion{Text: "Performance settings are in good condition.", Severity: "info", Setting: ""})
	}
	summary.Score = score
	httpx.WriteJSON(w, http.StatusOK, summary)
}

func statusString(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

func ifElse(condition bool, a, b string) string {
	if condition {
		return a
	}
	return b
}
