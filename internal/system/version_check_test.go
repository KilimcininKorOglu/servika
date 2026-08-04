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
