package system

import "testing"

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
