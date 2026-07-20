package wordpress

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedDBAccountRejectsInjectedDatabaseName(t *testing.T) {
	injected := "wp_deadbeef`; DROP DATABASE `panel"

	if dbUser, ok := managedDBAccount(injected); ok {
		t.Fatalf("managedDBAccount() accepted an injected database name and returned %q", dbUser)
	}
}

func TestManagedDBAccountDerivesPairedAccount(t *testing.T) {
	dbUser, ok := managedDBAccount("wp_deadbeef")
	if !ok {
		t.Fatal("managedDBAccount() rejected a package-managed database name")
	}
	if dbUser != "wpu_deadbeef" {
		t.Fatalf("managedDBAccount() user = %q, want %q", dbUser, "wpu_deadbeef")
	}
}

func TestInstallAlreadyExists_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if msg, ok := installAlreadyExists(dir); ok {
		t.Fatalf("empty dir should be clean, got: %s", msg)
	}
}

func TestInstallAlreadyExists_PlaceholderOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"index.html", "favicon.ico", ".htaccess"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if msg, ok := installAlreadyExists(dir); ok {
		t.Fatalf("placeholder-only dir should be clean, got: %s", msg)
	}
}

func TestInstallAlreadyExists_WpConfigBlocked(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wp-config.php"), []byte("<?php"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := installAlreadyExists(dir); !ok {
		t.Fatal("wp-config.php should block installation")
	}
}

func TestInstallAlreadyExists_NonPlaceholderBlocked(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "custom.php"), []byte("<?php"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := installAlreadyExists(dir); !ok {
		t.Fatal("non-placeholder content should block installation")
	}
}

func TestInstallAlreadyExists_MissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	if _, ok := installAlreadyExists(dir); ok {
		t.Fatal("missing dir should be clean")
	}
}
