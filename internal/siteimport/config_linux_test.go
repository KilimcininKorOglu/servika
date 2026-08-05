package siteimport

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requireSafeIO skips on anything but Linux. internal/files is openat2, and its
// macOS stub refuses every call, so these would pass by never running.
func requireSafeIO(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("openat2 is Linux-only; the safeio stub refuses every call elsewhere")
	}
}

func newHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, "public_html"), 0o750); err != nil {
		t.Fatalf("create home: %v", err)
	}
	return home
}

func write(t *testing.T, home, relative, content string) {
	t.Helper()
	full := filepath.Join(home, relative)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("create %s: %v", relative, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}

func read(t *testing.T, home, relative string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(home, relative)) // #nosec G304 -- test-owned path under t.TempDir().
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(body)
}

var newDB = target{DBName: "c_example_wp", User: "c_example_wp", Password: `p$1a'b\c`, Host: "localhost"}

// The whole point of the rewrite: a transferred site whose config still names
// the previous host's database answers "Error establishing a database
// connection" no matter how well the files and data arrived.
func TestRewriteWordPressPointsTheSiteAtTheNewDatabase(t *testing.T) {
	requireSafeIO(t)
	home := newHome(t)
	write(t, home, "public_html/wp-config.php", `<?php
define('DB_NAME', 'oldhost_wp');
define( "DB_USER" , "oldhost_user" );
define('DB_PASSWORD', 'old-secret');
define('DB_HOST', 'localhost');
define('AUTH_KEY', 'leave me alone');
$table_prefix = 'wp_';
`)

	change, found := rewriteWordPress(home, "c_example", "public_html", newDB)
	if !found {
		t.Fatal("rewriteWordPress reported no wp-config.php")
	}
	if !change.Applied {
		t.Fatalf("the rewrite was not applied: %+v", change)
	}
	if len(change.Fields) != 4 {
		t.Errorf("fields = %v, want all four DB constants", change.Fields)
	}

	got := read(t, home, "public_html/wp-config.php")
	for _, want := range []string{
		`define('DB_NAME', 'c_example_wp');`,
		`define( "DB_USER" , 'c_example_wp' );`,
		`define('DB_HOST', 'localhost');`,
		// The password carries a group reference, a quote and a backslash; all
		// three must survive verbatim into a PHP single-quoted literal.
		`define('DB_PASSWORD', 'p$1a\'b\\c');`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wp-config.php is missing %s\n%s", want, got)
		}
	}
	if !strings.Contains(got, `define('AUTH_KEY', 'leave me alone');`) {
		t.Error("the rewrite touched a constant it does not own")
	}
	if strings.Contains(got, "oldhost") {
		t.Error("an old credential survived the rewrite")
	}
}

// A config whose values come from getenv cannot be rewritten into something that
// still parses, so it must be reported rather than mangled.
func TestRewriteWordPressReportsAConfigItCannotRewrite(t *testing.T) {
	requireSafeIO(t)
	home := newHome(t)
	const original = "<?php\ndefine('DB_NAME', getenv('DB_NAME'));\n"
	write(t, home, "public_html/wp-config.php", original)

	change, found := rewriteWordPress(home, "c_example", "public_html", newDB)
	if !found {
		t.Fatal("rewriteWordPress reported no wp-config.php")
	}
	if change.Applied {
		t.Error("a config built from getenv was rewritten anyway")
	}
	if change.Note != "no_plain_constants" {
		t.Errorf("note = %q, want no_plain_constants", change.Note)
	}
	if got := read(t, home, "public_html/wp-config.php"); got != original {
		t.Errorf("the file was modified:\n%s", got)
	}
}

func TestRewriteDotEnvReplacesAndAppendsKeys(t *testing.T) {
	requireSafeIO(t)
	home := newHome(t)
	write(t, home, "public_html/artisan", "#!/usr/bin/env php\n")
	write(t, home, "public_html/.env", `APP_NAME=Demo
# DB_DATABASE=commented_out
DB_CONNECTION=mysql
DB_DATABASE=oldhost_app
DB_USERNAME=oldhost_user
APP_DEBUG=false
`)

	change, found := rewriteDotEnv(home, "c_example", "public_html", newDB)
	if !found || !change.Applied {
		t.Fatalf("the rewrite was not applied: found=%v change=%+v", found, change)
	}

	got := read(t, home, "public_html/.env")
	for _, want := range []string{
		"DB_CONNECTION=mysql",
		"DB_DATABASE=c_example_wp",
		"DB_USERNAME=c_example_wp",
		"DB_HOST=localhost",
		// Quoted because of the quote, backslash and dollar it carries.
		`DB_PASSWORD="p\$1a'b\\c"`,
		"APP_NAME=Demo",
		"APP_DEBUG=false",
		"# DB_DATABASE=commented_out",
	} {
		if !strings.Contains(got, want) {
			t.Errorf(".env is missing %s\n%s", want, got)
		}
	}
	if strings.Contains(got, "oldhost") {
		t.Error("an old credential survived the rewrite")
	}
	if count := strings.Count(got, "\nDB_DATABASE="); count != 1 {
		t.Errorf("DB_DATABASE appears %d times, want 1", count)
	}
}

// Plenty of projects ship a .env without being Laravel. Rewriting one that has
// no database settings would add settings the project never asked for.
func TestRewriteDotEnvSkipsANonDatabaseEnv(t *testing.T) {
	requireSafeIO(t)
	home := newHome(t)
	const original = "NODE_ENV=production\nAPI_URL=https://example.com\n"
	write(t, home, "public_html/.env", original)

	if _, found := rewriteDotEnv(home, "c_example", "public_html", newDB); found {
		t.Error("a .env with no database settings was rewritten")
	}
	if got := read(t, home, "public_html/.env"); got != original {
		t.Errorf("the file was modified:\n%s", got)
	}
}

// The panel runs as root and the tenant owns the home, so a symlink planted at
// the config's own path must not turn the rewrite into a root write elsewhere.
func TestRewriteRefusesASymlinkedConfig(t *testing.T) {
	requireSafeIO(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, "public_html"), 0o750); err != nil {
		t.Fatalf("create home: %v", err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside: %v", err)
	}
	victim := filepath.Join(outside, "victim.php")
	const untouched = "<?php define('DB_NAME', 'production'); // not the tenant's\n"
	if err := os.WriteFile(victim, []byte(untouched), 0o644); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	if err := os.Symlink(victim, filepath.Join(home, "public_html", "wp-config.php")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, found := rewriteWordPress(home, "c_example", "public_html", newDB); found {
		t.Error("a symlinked wp-config.php was treated as a real config")
	}
	body, err := os.ReadFile(victim) // #nosec G304 -- test-owned path under t.TempDir().
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(body) != untouched {
		t.Fatalf("ESCAPE: the panel rewrote %s as root, outside the tenant home:\n%s", victim, body)
	}
}

// The rewrite has to reach one level down, because a site extracted into
// public_html often lives in public_html/<app>.
func TestSearchDirectoriesCoversOneLevelAndSkipsSymlinks(t *testing.T) {
	requireSafeIO(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	for _, dir := range []string{"public_html/wp", "public_html/.hidden"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o750); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	write(t, home, "public_html/index.php", "<?php\n")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "public_html", "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got := searchDirectories(home, "public_html")
	want := map[string]bool{"public_html": true, "public_html/wp": true}
	for _, dir := range got {
		if !want[dir] {
			t.Errorf("searchDirectories returned %q, which is a hidden entry, a file or a symlink", dir)
		}
		delete(want, dir)
	}
	for missing := range want {
		t.Errorf("searchDirectories missed %q", missing)
	}
}
