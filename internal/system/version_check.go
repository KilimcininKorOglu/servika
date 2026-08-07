package system

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"servika/internal/config"
	"servika/internal/httpx"
)

const (
	versionCheckPeriod = 24 * time.Hour
	versionBodyLimit   = 64 << 10

	// versionLoginCooldown is the shortest interval between two login-triggered
	// checks. It is hours rather than minutes on purpose: the manifest request
	// carries this installation's ID, so one check per login would turn the
	// endpoint into a record of when each panel's administrators sign in. Six
	// hours bounds what an admin sees to a fraction of the 24-hour period while
	// adding at most three requests a day to the one the poller already makes.
	versionLoginCooldown = 6 * time.Hour
)

// VersionManifest is the public update and announcement manifest schema.
//
// Announcement is keyed by the supported UI language codes (config.NormalizeLang),
// so the update notice reads in the language the panel is displaying. English is
// the required entry and the fallback for every other code.
type VersionManifest struct {
	Latest       string            `json:"latest"`
	Announcement map[string]string `json:"announcement"`
	Critical     bool              `json:"critical"`
	ReleaseDate  string            `json:"release_date"`
}

// localizedAnnouncement returns the announcement for lang, falling back to English
// when the manifest carries no usable text for that language. It normalizes the
// code itself so an unvetted request parameter can never reach the map as-is; an
// unsupported or missing code collapses to English rather than being rejected.
func (m VersionManifest) localizedAnnouncement(lang string) string {
	if text, ok := m.Announcement[config.NormalizeLang(lang)]; ok {
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(m.Announcement["en"])
}

type versionCache struct {
	Manifest  VersionManifest `json:"manifest"`
	LastCheck time.Time       `json:"last_check"`
	// Current is the compiled version that wrote this file. The cache lives at
	// /opt/servika/version-cache.json, which servika-update never replaces (it
	// swaps bin/, frontend-dist and src/ only), so after an update the previous
	// build's manifest is still on disk and would be compared against the new
	// binary. Written before this field existed, it unmarshals to "" and the
	// cache is refused, which is the safe answer.
	Current string `json:"current"`
}

var (
	versionMu        sync.RWMutex
	versionCurrent   string
	versionBuildDate string
	versionManifest  VersionManifest
	versionLast      time.Time
	versionError     string
	versionEnabled   bool
)

func versionCheckEnabled() bool {
	value := strings.TrimSpace(os.Getenv("SERVIKA_VERSION_CHECK"))
	return value != "0" && !strings.EqualFold(value, "false") && !strings.EqualFold(value, "no")
}

func versionEndpoint() string {
	return config.VersionEndpoint()
}

func versionRequestURL() string {
	return versionRequestURLFor(versionEndpoint(), InstallationID(), currentVersion())
}

func versionRequestURLFor(rawEndpoint, installationID, current string) string {
	parsed, err := url.Parse(rawEndpoint)
	if err != nil {
		return rawEndpoint
	}
	query := parsed.Query()
	if installationID != "" {
		query.Set("id", installationID)
	}
	query.Set("v", current)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// InstallationID returns a persistent anonymous installation identifier.
func InstallationID() string {
	path := config.InstallationIDPath()
	// #nosec G304 -- path is a fixed system/config path, a server-internal temp/archive path, or built from a validated identifier; tenant file reads go through safeio (openat2), not this call.
	if content, err := os.ReadFile(path); err == nil {
		if value := strings.TrimSpace(string(content)); len(value) >= 16 {
			return value
		}
	}

	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	id := hex.EncodeToString(raw)
	// #nosec G301 -- root-owned system directory whose daemon (nginx/php-fpm/named) must traverse it; contains no secret material.
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(id+"\n"), 0o600)
	return id
}

// StartVersionCheck starts the background version check loop.
func StartVersionCheck(current, buildDate string) {
	versionMu.Lock()
	versionCurrent = current
	versionBuildDate = buildDate
	versionEnabled = versionCheckEnabled()
	versionMu.Unlock()

	_ = InstallationID()

	if !versionCheckEnabled() {
		return
	}
	loadVersionCache()

	go func() {
		time.Sleep(versionRandomDuration(10*time.Second, 60*time.Second))
		for {
			fetchVersionManifest()
			time.Sleep(versionCheckPeriod + versionRandomDuration(-2*time.Hour, 2*time.Hour))
		}
	}()
}

// TriggerVersionCheck refreshes the manifest when a panel administrator signs
// in, so the update notice does not sit up to a full poll period behind. It is
// fire-and-forget: the fetch runs in its own goroutine and the login response
// never waits on it, and fetchVersionManifest records a network failure in the
// cached state instead of surfacing it.
//
// Call it only from a path an administrator reaches. A customer login must not
// reach the manifest endpoint, because the request identifies the installation.
func TriggerVersionCheck() {
	if !versionCheckEnabled() {
		return
	}
	versionMu.RLock()
	last := versionLast
	versionMu.RUnlock()
	if !versionCheckDue(last, time.Now()) {
		return
	}
	go fetchVersionManifest()
}

// versionCheckDue reports whether a login-triggered check may run. Split out so
// the cooldown is testable without a clock or a network.
func versionCheckDue(lastCheck, now time.Time) bool {
	if lastCheck.IsZero() {
		return true
	}
	return now.Sub(lastCheck) >= versionLoginCooldown
}

func versionRandomDuration(minimum, maximum time.Duration) time.Duration {
	if maximum <= minimum {
		return minimum
	}
	number, err := rand.Int(rand.Reader, big.NewInt(int64(maximum-minimum)))
	if err != nil {
		return minimum
	}
	return minimum + time.Duration(number.Int64())
}

func fetchVersionManifest() {
	client := &http.Client{Timeout: 20 * time.Second}
	request, err := http.NewRequest(http.MethodGet, versionRequestURL(), nil)
	if err != nil {
		setVersionError("version check request could not be prepared")
		return
	}
	request.Header.Set("User-Agent", "Servika/"+currentVersion())

	response, err := client.Do(request)
	if err != nil {
		setVersionError("version endpoint could not be reached")
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		setVersionError("version endpoint returned a non-OK response")
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, versionBodyLimit))
	if err != nil {
		setVersionError("version response could not be read")
		return
	}

	var manifest VersionManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		setVersionError("version manifest could not be parsed")
		return
	}
	if strings.TrimSpace(manifest.Latest) == "" {
		setVersionError("version manifest has no latest version")
		return
	}

	versionMu.Lock()
	versionManifest = manifest
	versionLast = time.Now()
	versionError = ""
	versionMu.Unlock()
	saveVersionCache()
}

func currentVersion() string {
	versionMu.RLock()
	defer versionMu.RUnlock()
	return versionCurrent
}

func setVersionError(message string) {
	versionMu.Lock()
	versionError = message
	versionMu.Unlock()
}

func saveVersionCache() {
	versionMu.RLock()
	cache := versionCache{Manifest: versionManifest, LastCheck: versionLast, Current: versionCurrent}
	versionMu.RUnlock()
	content, err := json.Marshal(cache)
	if err != nil {
		return
	}
	path := config.VersionCachePath()
	// #nosec G301 -- root-owned system directory whose daemon (nginx/php-fpm/named) must traverse it; contains no secret material.
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	// #nosec G306 -- root-owned system integration file (nginx/php-fpm/named/systemd config, script, or web content) that its daemon must read/execute; no secret stored here (secrets use 0600/0640).
	_ = os.WriteFile(path, content, 0o644)
}

// loadVersionCache restores the last known state across a restart so the panel
// does not show "never checked" the moment it comes up. A cache left by a
// different build is discarded instead: showing nothing until the first
// background scan (10-60 seconds after boot) or the sign-in trigger is always
// better than showing a comparison against someone else's version.
func loadVersionCache() {
	content, err := os.ReadFile(config.VersionCachePath())
	if err != nil {
		return
	}
	var cache versionCache
	if json.Unmarshal(content, &cache) != nil {
		return
	}
	versionMu.Lock()
	defer versionMu.Unlock()
	// Read the field directly: currentVersion() takes the read lock, which this
	// goroutine already holds for writing.
	if !versionCacheUsable(cache.Current, versionCurrent) {
		return
	}
	versionManifest = cache.Manifest
	versionLast = cache.LastCheck
}

// versionCacheUsable reports whether a cache file may be trusted. Split out so
// the rule is testable without a filesystem. An empty cacheCurrent is a file
// written before the field existed and counts as a different build.
func versionCacheUsable(cacheCurrent, runningCurrent string) bool {
	return cacheCurrent != "" && cacheCurrent == runningCurrent
}

// VersionCheckRefresh runs an immediate version check when it is enabled.
func VersionCheckRefresh(w http.ResponseWriter, r *http.Request) {
	if !versionCheckEnabled() {
		httpx.WriteError(w, http.StatusConflict, "version check is disabled by SERVIKA_VERSION_CHECK=0")
		return
	}
	fetchVersionManifest()
	VersionCheckStatus(w, r)
}

// releaseIsNewer reports whether latest is a strictly newer release than
// current. Asking only whether the two differ turns any manifest that trails
// the installed build into an update notice pointing backwards, which the
// operator reads as "you are out of date" while running the newer release. The
// published manifest can trail for a few minutes after a release, because the
// raw content endpoint is served from a CDN cache.
//
// Both values are the X.Y.Z form every Servika release carries. When either
// side is not in that form the answer falls back to plain inequality, matching
// the previous behaviour: an unreadable version must not silently suppress a
// real update notice.
func releaseIsNewer(latest, current string) bool {
	newer, ok := parseReleaseVersion(latest)
	if !ok {
		return latest != current
	}
	running, ok := parseReleaseVersion(current)
	if !ok {
		return latest != current
	}
	for i := range newer {
		if newer[i] != running[i] {
			return newer[i] > running[i]
		}
	}
	return false
}

// parseReleaseVersion splits a strict X.Y.Z version. Anything else, including a
// build suffix or a leading "v", is reported as unparseable rather than guessed
// at.
func parseReleaseVersion(value string) ([3]int, bool) {
	var parts [3]int
	fields := strings.Split(strings.TrimSpace(value), ".")
	if len(fields) != len(parts) {
		return parts, false
	}
	for i, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil || number < 0 {
			return parts, false
		}
		parts[i] = number
	}
	return parts, true
}

// VersionInfo reports which panel this is, and nothing else.
//
// It exists so the footer can name the version to EVERY signed-in account.
// VersionCheckStatus is ResellerOrAbove, so a customer only ever received a 403
// there and the footer showed a product name with no version after it.
//
// Relaxing that guard instead would have been the wrong fix. Its response
// carries latest, update_available, announcement, critical and release_date,
// all of it read from an external manifest and all of it the operator's
// business: opening it would tell every customer that their host is running an
// out-of-date panel and what the newer one changed. This endpoint deliberately
// carries neither, so widening the audience widens nothing else with it.
func VersionInfo(w http.ResponseWriter, _ *http.Request) {
	versionMu.RLock()
	current, buildDate := versionCurrent, versionBuildDate
	versionMu.RUnlock()

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"current":    current,
		"build_date": buildDate,
	})
}

// VersionCheckStatus returns the current version check state. The optional lang
// parameter carries the language the panel is displaying, which selects the
// announcement translation; the response itself always holds a single string.
func VersionCheckStatus(w http.ResponseWriter, r *http.Request) {
	versionMu.RLock()
	current := versionCurrent
	buildDate := versionBuildDate
	manifest := versionManifest
	lastCheck := versionLast
	message := versionError
	enabled := versionEnabled
	versionMu.RUnlock()

	updateAvailable := enabled && manifest.Latest != "" && releaseIsNewer(manifest.Latest, current)
	response := map[string]any{
		"enabled":          enabled,
		"current":          current,
		"build_date":       buildDate,
		"latest":           manifest.Latest,
		"update_available": updateAvailable,
		"announcement":     manifest.localizedAnnouncement(r.URL.Query().Get("lang")),
		"critical":         manifest.Critical && updateAvailable,
		"release_date":     manifest.ReleaseDate,
		"error":            message,
	}
	if !lastCheck.IsZero() {
		response["last_check"] = lastCheck.UTC().Format(time.RFC3339)
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}
