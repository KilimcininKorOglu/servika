package git

import (
	"os"
	"path/filepath"
	"testing"
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
	target := t.TempDir()
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

	if err := clearDirectoryContents(target); err != nil {
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
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(protected, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "target")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	if err := clearDirectoryContents(link); err == nil {
		t.Fatal("clearDirectoryContents() error = nil, want symlink rejection")
	}
	if _, err := os.Stat(protected); err != nil {
		t.Fatalf("symlink destination content changed: %v", err)
	}
}
