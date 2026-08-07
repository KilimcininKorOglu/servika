package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"servika/internal/config"
)

// The published manifest must keep the object form and a usable English entry. A
// plain string fails json.Unmarshal on every panel, which silently disables the
// whole version check, and a missing "en" removes the fallback the other eleven
// languages depend on. A key that is not in canonical supported form is
// unreachable, because the lookup normalizes the requested language first.
func TestPublishedManifestIsLocalizable(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "version.json"))
	if err != nil {
		t.Skipf("published manifest is not readable here: %v", err)
	}
	var manifest VersionManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("version.json does not unmarshal into VersionManifest: %v", err)
	}
	if strings.TrimSpace(manifest.Announcement["en"]) == "" {
		t.Fatal("version.json carries no English announcement to fall back to")
	}
	for lang := range manifest.Announcement {
		if config.NormalizeLang(lang) != lang {
			t.Errorf("version.json announces language %q, which no request can resolve to", lang)
		}
	}
}

// End of the wiring: the lang query value must reach the resolver and the response
// must still carry a single string, not the whole translation map.
func TestVersionCheckStatusSelectsRequestLanguage(t *testing.T) {
	versionMu.Lock()
	previousManifest, previousCurrent, previousEnabled := versionManifest, versionCurrent, versionEnabled
	versionManifest = VersionManifest{
		Latest:       "9.9.9",
		Announcement: map[string]string{"en": "English text", "tr": "Türkçe metin"},
	}
	versionCurrent = "9.9.8"
	versionEnabled = true
	versionMu.Unlock()
	t.Cleanup(func() {
		versionMu.Lock()
		versionManifest, versionCurrent, versionEnabled = previousManifest, previousCurrent, previousEnabled
		versionMu.Unlock()
	})

	tests := []struct {
		name, query, want string
	}{
		{name: "translated language", query: "?lang=tr", want: "Türkçe metin"},
		{name: "untranslated language", query: "?lang=de", want: "English text"},
		{name: "no language", query: "", want: "English text"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			VersionCheckStatus(recorder, httptest.NewRequest("GET", "/system/version-check"+tc.query, nil))

			var body struct {
				Announcement string `json:"announcement"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not the documented shape: %v", err)
			}
			if body.Announcement != tc.want {
				t.Fatalf("announcement = %q, want %q", body.Announcement, tc.want)
			}
		})
	}
}

// The announcement must read in the language the panel displays, and must degrade
// to English rather than to nothing whenever a translation is absent or unusable.
func TestLocalizedAnnouncementFallsBackToEnglish(t *testing.T) {
	manifest := VersionManifest{Announcement: map[string]string{
		"en":    "English text",
		"tr":    "Türkçe metin",
		"pt-BR": "Texto brasileiro",
		"ja":    "   ",
	}}
	tests := []struct {
		name string
		lang string
		want string
	}{
		{name: "exact match", lang: "tr", want: "Türkçe metin"},
		{name: "region form is canonicalized", lang: "pt-br", want: "Texto brasileiro"},
		{name: "supported language with no translation", lang: "de", want: "English text"},
		{name: "whitespace-only translation is not usable", lang: "ja", want: "English text"},
		{name: "unsupported code", lang: "xx", want: "English text"},
		{name: "empty code", lang: "", want: "English text"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := manifest.localizedAnnouncement(tc.lang); got != tc.want {
				t.Fatalf("localizedAnnouncement(%q) = %q, want %q", tc.lang, got, tc.want)
			}
		})
	}
}

// A manifest without the field must show no notice at all, never a stale or
// partially rendered one.
func TestLocalizedAnnouncementWithoutManifestText(t *testing.T) {
	var manifest VersionManifest
	if got := manifest.localizedAnnouncement("tr"); got != "" {
		t.Fatalf("localizedAnnouncement() = %q, want an empty string", got)
	}
}

func TestVersionRequestURLForAddsInstallationAndVersion(t *testing.T) {
	got := versionRequestURLFor("https://updates.example.test/version.json", "install-123", "1.2.3")
	want := "https://updates.example.test/version.json?id=install-123&v=1.2.3"
	if got != want {
		t.Fatalf("versionRequestURLFor() = %q, want %q", got, want)
	}
}

func TestVersionRequestURLForPreservesExistingQuery(t *testing.T) {
	got := versionRequestURLFor("https://updates.example.test/version.json?channel=stable", "install-123", "1.2.3")
	want := "https://updates.example.test/version.json?channel=stable&id=install-123&v=1.2.3"
	if got != want {
		t.Fatalf("versionRequestURLFor() = %q, want %q", got, want)
	}
}

func TestVersionRequestURLForKeepsInvalidEndpoint(t *testing.T) {
	got := versionRequestURLFor("http://%41:80/", "install-123", "1.2.3")
	want := "http://%41:80/"
	if got != want {
		t.Fatalf("versionRequestURLFor() = %q, want %q", got, want)
	}
}

// The manifest request carries this installation's ID, so an uncapped
// login-triggered check would publish a record of when the panel's
// administrators sign in.
func TestVersionCheckDueRespectsTheLoginCooldown(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		last time.Time
		want bool
	}{
		{name: "never checked", last: time.Time{}, want: true},
		{name: "just checked", last: now.Add(-time.Minute), want: false},
		{name: "one second inside the cooldown", last: now.Add(-versionLoginCooldown + time.Second), want: false},
		{name: "exactly at the cooldown", last: now.Add(-versionLoginCooldown), want: true},
		{name: "a full poll period ago", last: now.Add(-versionCheckPeriod), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := versionCheckDue(test.last, now); got != test.want {
				t.Errorf("versionCheckDue(%v, %v) = %t, want %t", test.last, now, got, test.want)
			}
		})
	}
}

// A login must never reach the network when the operator turned the check off.
func TestTriggerVersionCheckHonoursTheDisableSwitch(t *testing.T) {
	t.Setenv("SERVIKA_VERSION_CHECK", "0")
	t.Setenv("SERVIKA_VERSION_ENDPOINT", "http://127.0.0.1:1/never-reachable")

	versionMu.Lock()
	versionLast = time.Time{} // A check has never run, so only the switch can stop it.
	versionError = ""
	versionMu.Unlock()

	TriggerVersionCheck()
	time.Sleep(200 * time.Millisecond)

	versionMu.RLock()
	defer versionMu.RUnlock()
	if !versionLast.IsZero() || versionError != "" {
		t.Errorf("a check ran with SERVIKA_VERSION_CHECK=0 (last=%v, error=%q)", versionLast, versionError)
	}
}

// The whole point of the trigger is that a sign-in fetches the manifest, so
// prove the request actually leaves rather than trusting the call site, and
// prove the cooldown then suppresses the next one.
func TestTriggerVersionCheckFetchesOncePerCooldown(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"latest":"9.9.9","announcement":{"en":"test"},"critical":false,"release_date":"2026-01-01"}`))
	}))
	defer server.Close()

	t.Setenv("SERVIKA_VERSION_CHECK", "1")
	t.Setenv("SERVIKA_VERSION_ENDPOINT", server.URL)
	t.Setenv("SERVIKA_VERSION_CACHE", filepath.Join(t.TempDir(), "version.json"))

	versionMu.Lock()
	versionLast = time.Time{}
	versionMu.Unlock()

	TriggerVersionCheck()
	waitForRequests(t, &requests, 1)

	// The second sign-in falls inside the cooldown and must not hit the endpoint.
	TriggerVersionCheck()
	time.Sleep(300 * time.Millisecond)
	if got := requests.Load(); got != 1 {
		t.Errorf("the endpoint received %d requests, want 1: the cooldown did not suppress the second sign-in", got)
	}

	versionMu.RLock()
	defer versionMu.RUnlock()
	if versionManifest.Latest != "9.9.9" {
		t.Errorf("versionManifest.Latest = %q, want the manifest the trigger fetched", versionManifest.Latest)
	}
}

func waitForRequests(t *testing.T, counter *atomic.Int32, want int32) {
	t.Helper()
	for range 50 {
		if counter.Load() >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the endpoint received %d requests, want %d: the trigger never fetched", counter.Load(), want)
}

// servika-update swaps bin/, frontend-dist and src/ but leaves
// /opt/servika/version-cache.json in place, so the previous build's manifest
// survives an update. Loading it produced "installed 1.1.0, new 1.0.9": an
// update notice pointing at an OLDER release than the one running.
func TestVersionCacheUsableRefusesAnotherBuildsCache(t *testing.T) {
	tests := []struct {
		name           string
		cached, riding string
		want           bool
	}{
		{name: "same build, plain restart", cached: "1.1.0", riding: "1.1.0", want: true},
		{name: "cache from the previous build", cached: "1.0.9", riding: "1.1.0", want: false},
		{name: "cache from a newer build after a rollback", cached: "1.2.0", riding: "1.1.0", want: false},
		{name: "cache written before the field existed", cached: "", riding: "1.1.0", want: false},
		{name: "version not yet published to internal/system", cached: "1.1.0", riding: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := versionCacheUsable(test.cached, test.riding); got != test.want {
				t.Errorf("versionCacheUsable(%q, %q) = %t, want %t", test.cached, test.riding, got, test.want)
			}
		})
	}
}

// End to end over the real file, because the bug was in what loadVersionCache
// trusted, not in the comparison alone.
func TestLoadVersionCacheDropsThePreviousBuildsManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-cache.json")
	t.Setenv("SERVIKA_VERSION_CACHE", path)

	writeCache := func(t *testing.T, cache versionCache) {
		t.Helper()
		content, err := json.Marshal(cache)
		if err != nil {
			t.Fatalf("marshal cache: %v", err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("write cache: %v", err)
		}
	}
	reset := func(t *testing.T, running string) {
		t.Helper()
		versionMu.Lock()
		defer versionMu.Unlock()
		versionCurrent = running
		versionManifest = VersionManifest{}
		versionLast = time.Time{}
	}
	loadedLatest := func() string {
		versionMu.RLock()
		defer versionMu.RUnlock()
		return versionManifest.Latest
	}

	stale := versionCache{
		Manifest:  VersionManifest{Latest: "1.0.9", Announcement: map[string]string{"en": "older release"}},
		LastCheck: time.Now().Add(-time.Hour),
		Current:   "1.0.9",
	}

	writeCache(t, stale)
	reset(t, "1.1.0")
	loadVersionCache()
	if got := loadedLatest(); got != "" {
		t.Errorf("loaded latest %q from the previous build's cache, so the panel would offer a downgrade", got)
	}

	// The same file is still usable by the build that wrote it.
	reset(t, "1.0.9")
	loadVersionCache()
	if got := loadedLatest(); got != "1.0.9" {
		t.Errorf("loaded latest %q, want the cache restored across a plain restart", got)
	}
}

// An update notice must mean "a newer release exists", not "the two strings
// differ". The published manifest is served from a CDN cache, so it can trail
// the installed build for a few minutes after a release, and a bare inequality
// then tells the operator to update while they are already on the newer one.
func TestReleaseIsNewerOnlyPointsForward(t *testing.T) {
	tests := []struct {
		name            string
		latest, running string
		want            bool
	}{
		{name: "patch ahead", latest: "1.1.1", running: "1.1.0", want: true},
		{name: "minor ahead", latest: "1.2.0", running: "1.1.9", want: true},
		{name: "major ahead", latest: "2.0.0", running: "1.9.9", want: true},
		{name: "identical", latest: "1.1.0", running: "1.1.0", want: false},
		{name: "manifest trails by a patch", latest: "1.0.9", running: "1.1.0", want: false},
		{name: "manifest trails by a minor", latest: "1.1.0", running: "1.2.0", want: false},
		{name: "manifest trails by a major", latest: "1.9.9", running: "2.0.0", want: false},
		// Double-digit segments must compare as numbers, not as text.
		{name: "ten is newer than nine", latest: "1.10.0", running: "1.9.0", want: true},
		{name: "nine is not newer than ten", latest: "1.9.0", running: "1.10.0", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := releaseIsNewer(test.latest, test.running); got != test.want {
				t.Errorf("releaseIsNewer(%q, %q) = %t, want %t", test.latest, test.running, got, test.want)
			}
		})
	}
}

// An unreadable version on either side must not silently suppress a real update
// notice, so the answer falls back to the plain inequality used before.
func TestReleaseIsNewerFallsBackForUnreadableVersions(t *testing.T) {
	tests := []struct {
		latest, running string
		want            bool
	}{
		{latest: "1.2", running: "1.1.0", want: true},
		{latest: "1.2.0-rc1", running: "1.1.0", want: true},
		{latest: "v1.2.0", running: "1.1.0", want: true},
		{latest: "1.2.0", running: "dev", want: true},
		{latest: "not a version", running: "not a version", want: false},
		{latest: "1.1.0", running: "", want: true},
	}
	for _, test := range tests {
		if got := releaseIsNewer(test.latest, test.running); got != test.want {
			t.Errorf("releaseIsNewer(%q, %q) = %t, want %t", test.latest, test.running, got, test.want)
		}
	}
}

// The wiring: a trailing manifest must not set update_available in the response
// the dashboard reads.
func TestVersionCheckStatusHidesAnUpdateThatPointsBackwards(t *testing.T) {
	versionMu.Lock()
	previous := struct {
		manifest VersionManifest
		current  string
		enabled  bool
	}{versionManifest, versionCurrent, versionEnabled}
	versionManifest = VersionManifest{Latest: "1.0.9", Announcement: map[string]string{"en": "older"}, Critical: true}
	versionCurrent = "1.1.0"
	versionEnabled = true
	versionMu.Unlock()
	t.Cleanup(func() {
		versionMu.Lock()
		versionManifest, versionCurrent, versionEnabled = previous.manifest, previous.current, previous.enabled
		versionMu.Unlock()
	})

	recorder := httptest.NewRecorder()
	VersionCheckStatus(recorder, httptest.NewRequest("GET", "/system/version-check", nil))

	var body struct {
		UpdateAvailable bool `json:"update_available"`
		Critical        bool `json:"critical"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the documented shape: %v", err)
	}
	if body.UpdateAvailable {
		t.Error("update_available is set for a manifest older than the running build, which reads as \"downgrade now\"")
	}
	if body.Critical {
		t.Error("critical is set for an update that does not exist, which paints the notice red")
	}
}

// The footer names the panel to every signed-in account, so this endpoint has no
// role guard. That is only safe because of what it leaves out: the update state
// and the announcement stay behind ResellerOrAbove on /system/version-check.
// Both halves are asserted, because carrying the version is useless if it also
// carries the manifest, and refusing the manifest is useless if it refuses the
// version too.
func TestVersionInfoCarriesTheVersionAndNothingFromTheManifest(t *testing.T) {
	versionMu.Lock()
	previousManifest, previousCurrent := versionManifest, versionCurrent
	previousBuild, previousEnabled := versionBuildDate, versionEnabled
	versionManifest = VersionManifest{
		Latest:       "9.9.9",
		Critical:     true,
		ReleaseDate:  "2026-01-01",
		Announcement: map[string]string{"en": "an operator-facing announcement"},
	}
	versionCurrent, versionBuildDate, versionEnabled = "9.9.8", "2026-01-02", true
	versionMu.Unlock()
	t.Cleanup(func() {
		versionMu.Lock()
		versionManifest, versionCurrent = previousManifest, previousCurrent
		versionBuildDate, versionEnabled = previousBuild, previousEnabled
		versionMu.Unlock()
	})

	recorder := httptest.NewRecorder()
	VersionInfo(recorder, httptest.NewRequest(http.MethodGet, "/system/version", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode the response: %v", err)
	}
	if payload["current"] != "9.9.8" || payload["build_date"] != "2026-01-02" {
		t.Errorf("payload = %v, want this installation's version and build date", payload)
	}
	// The whole reason the endpoint is separate: an update notice and its text
	// are the operator's business, not every customer's.
	for _, leaked := range []string{"latest", "update_available", "announcement", "critical", "release_date", "error", "enabled", "last_check"} {
		if _, present := payload[leaked]; present {
			t.Errorf("the open endpoint carries %q, which belongs to /system/version-check", leaked)
		}
	}
	// The announcement text must not reach the body under any key at all.
	if strings.Contains(recorder.Body.String(), "operator-facing") {
		t.Error("the announcement text reached the open endpoint")
	}
}
