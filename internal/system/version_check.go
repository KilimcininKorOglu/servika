package system

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"servika/internal/httpx"
)

const (
	versionEndpointDefault = "https://raw.githubusercontent.com/KilimcininKorOglu/servika/main/version.json"
	installationIDPath     = "/etc/servika/installation-id"
	versionCachePath       = "/opt/servika/version-cache.json"
	versionCheckPeriod     = 24 * time.Hour
	versionBodyLimit       = 64 << 10
)

// VersionManifest is the public update and announcement manifest schema.
type VersionManifest struct {
	Latest       string `json:"latest"`
	Announcement string `json:"announcement"`
	Critical     bool   `json:"critical"`
	ReleaseDate  string `json:"release_date"`
}

type versionCache struct {
	Manifest  VersionManifest `json:"manifest"`
	LastCheck time.Time       `json:"last_check"`
}

var (
	versionMu       sync.RWMutex
	versionCurrent  string
	versionManifest VersionManifest
	versionLast     time.Time
	versionError    string
	versionEnabled  bool
)

func versionCheckEnabled() bool {
	value := strings.TrimSpace(os.Getenv("SERVIKA_VERSION_CHECK"))
	return value != "0" && !strings.EqualFold(value, "false") && !strings.EqualFold(value, "no")
}

func versionEndpoint() string {
	if value := strings.TrimSpace(os.Getenv("SERVIKA_VERSION_ENDPOINT")); value != "" {
		return value
	}
	return versionEndpointDefault
}

// InstallationID returns a persistent anonymous installation identifier.
func InstallationID() string {
	if content, err := os.ReadFile(installationIDPath); err == nil {
		if value := strings.TrimSpace(string(content)); len(value) >= 16 {
			return value
		}
	}

	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	id := hex.EncodeToString(raw)
	_ = os.MkdirAll(filepath.Dir(installationIDPath), 0o755)
	_ = os.WriteFile(installationIDPath, []byte(id+"\n"), 0o600)
	return id
}

// StartVersionCheck starts the background version check loop.
func StartVersionCheck(current string) {
	versionMu.Lock()
	versionCurrent = current
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
	request, err := http.NewRequest(http.MethodGet, versionEndpoint(), nil)
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
	cache := versionCache{Manifest: versionManifest, LastCheck: versionLast}
	versionMu.RUnlock()
	content, err := json.Marshal(cache)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(versionCachePath), 0o755)
	_ = os.WriteFile(versionCachePath, content, 0o644)
}

func loadVersionCache() {
	content, err := os.ReadFile(versionCachePath)
	if err != nil {
		return
	}
	var cache versionCache
	if json.Unmarshal(content, &cache) != nil {
		return
	}
	versionMu.Lock()
	versionManifest = cache.Manifest
	versionLast = cache.LastCheck
	versionMu.Unlock()
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

// VersionCheckStatus returns the current version check state.
func VersionCheckStatus(w http.ResponseWriter, _ *http.Request) {
	versionMu.RLock()
	current := versionCurrent
	manifest := versionManifest
	lastCheck := versionLast
	message := versionError
	enabled := versionEnabled
	versionMu.RUnlock()

	updateAvailable := enabled && manifest.Latest != "" && manifest.Latest != current
	response := map[string]any{
		"enabled":          enabled,
		"current":          current,
		"latest":           manifest.Latest,
		"update_available": updateAvailable,
		"announcement":     manifest.Announcement,
		"critical":         manifest.Critical && updateAvailable,
		"release_date":     manifest.ReleaseDate,
		"error":            message,
	}
	if !lastCheck.IsZero() {
		response["last_check"] = lastCheck.UTC().Format(time.RFC3339)
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}
