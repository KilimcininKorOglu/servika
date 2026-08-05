package git

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"servika/internal/files"
)

func TestValidTargetDir(t *testing.T) {
	tests := []struct {
		name      string
		targetDir string
		valid     bool
	}{
		{name: "public directory", targetDir: "public_html", valid: true},
		{name: "nested directory", targetDir: "apps/site", valid: true},
		{name: "current directory", targetDir: ".", valid: false},
		{name: "absolute directory", targetDir: "/tmp/site", valid: false},
		{name: "parent traversal", targetDir: "../site", valid: false},
		{name: "embedded traversal", targetDir: "site/../public", valid: false},
		{name: "shell metacharacter", targetDir: "site;id", valid: false},
		{name: "surrounding whitespace", targetDir: " public_html ", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validTargetDir(test.targetDir); got != test.valid {
				t.Fatalf("validTargetDir(%q) = %t, want %t", test.targetDir, got, test.valid)
			}
		})
	}
}

func TestValidBranch(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		valid  bool
	}{
		{name: "main branch", branch: "main", valid: true},
		{name: "nested branch", branch: "release/v1.2", valid: true},
		{name: "option", branch: "--upload-pack=evil", valid: false},
		{name: "revision expression", branch: "main..evil", valid: false},
		{name: "shell metacharacter", branch: "main;id", valid: false},
		{name: "surrounding whitespace", branch: " main ", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validBranch(test.branch); got != test.valid {
				t.Fatalf("validBranch(%q) = %t, want %t", test.branch, got, test.valid)
			}
		})
	}
}

func TestValidRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		valid   bool
	}{
		{name: "HTTPS", repoURL: "https://github.com/example/site.git", valid: true},
		{name: "SSH URL", repoURL: "ssh://git@example.com/example/site.git", valid: true},
		{name: "SSH shorthand", repoURL: "git@example.com:example/site.git", valid: true},
		{name: "unsupported scheme", repoURL: "file:///tmp/site", valid: false},
		{name: "shell metacharacter", repoURL: "https://example.com/site.git;id", valid: false},
		{name: "command substitution", repoURL: "https://example.com/$(id).git", valid: false},
		{name: "surrounding whitespace", repoURL: " https://example.com/site.git ", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validRepoURL(test.repoURL); got != test.valid {
				t.Fatalf("validRepoURL(%q) = %t, want %t", test.repoURL, got, test.valid)
			}
		})
	}
}

func TestClearDirectoryContentsPreservesTarget(t *testing.T) {
	requireSafeIO(t)
	home := t.TempDir()
	target := filepath.Join(home, "public_html")
	if err := os.Mkdir(target, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "index.html"), []byte("site"), 0600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(target, "assets")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "app.js"), []byte("app"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := clearDirectoryContents(home, "public_html"); err != nil {
		t.Fatalf("clearDirectoryContents() error = %v", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("target directory was removed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("target directory contains %d entries, want 0", len(entries))
	}
}

func TestClearDirectoryContentsRejectsSymlink(t *testing.T) {
	requireSafeIO(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(protected, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "target")); err != nil {
		t.Fatal(err)
	}

	if err := clearDirectoryContents(home, "target"); err == nil {
		t.Fatal("clearDirectoryContents() error = nil, want symlink rejection")
	}
	if _, err := os.Stat(protected); err != nil {
		t.Fatalf("symlink destination content changed: %v", err)
	}
}

// The escape this closed: validTargetDir permits a separator, so only the LAST
// component was ever checked. A tenant symlink at an INTERMEDIATE component
// made the panel list and delete a directory outside the home as root, and the
// mkdir and chown that followed handed that directory to the tenant. Pointing
// it at /etc/cron.d turns that into running code as root.
func TestClearDirectoryContentsRejectsAnIntermediateSymlink(t *testing.T) {
	requireSafeIO(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "etc")
	if err := os.MkdirAll(filepath.Join(outside, "cron.d"), 0755); err != nil {
		t.Fatal(err)
	}
	rootJob := filepath.Join(outside, "cron.d", "root-job")
	if err := os.WriteFile(rootJob, []byte("* * * * * root id\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// What the tenant does through FTP, SSH or PHP symlink().
	if err := os.Symlink(outside, filepath.Join(home, "pwn")); err != nil {
		t.Fatal(err)
	}

	const targetDir = "pwn/cron.d"
	if !validTargetDir(targetDir) {
		t.Fatalf("validTargetDir(%q) = false; the escape path is unreachable and this test proves nothing", targetDir)
	}
	if err := clearDirectoryContents(home, targetDir); err == nil {
		t.Error("clearDirectoryContents() error = nil for a symlinked intermediate component")
	}
	if err := files.MkdirAllBeneath(home, targetDir, "c_example"); err == nil {
		t.Error("MkdirAllBeneath() error = nil for a symlinked intermediate component")
	}
	if _, err := os.Stat(rootJob); err != nil {
		t.Fatalf("ESCAPE: the panel reached %s outside the tenant home: %v", rootJob, err)
	}
}

// safeio is openat2, so it only works on Linux; on macOS every call returns the
// platform error and these tests would pass for the wrong reason.
func requireSafeIO(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("safeio needs openat2, which is Linux-only")
	}
}
