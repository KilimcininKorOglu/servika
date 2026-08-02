package system

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
