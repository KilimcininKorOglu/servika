package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Without -y, Info-ZIP follows a symbolic link and stores the CONTENT of its
// target, so archiving a tenant home would package whatever their links point
// at. The flag has to be there, and ahead of the archive name.
func TestArchiveCommandStoresSymlinksAsLinks(t *testing.T) {
	tool, args := archiveCommand("zip", "/home/c_x/out.zip", []string{"/home/c_x/public_html"})
	if tool != "zip" {
		t.Fatalf("tool = %q, want zip", tool)
	}
	if !slices.Contains(args, "-y") {
		t.Errorf("the zip arguments do not carry -y: %v", args)
	}
	if slices.Index(args, "-y") > slices.Index(args, "/home/c_x/out.zip") {
		t.Errorf("-y comes after the archive name, where zip no longer reads it: %v", args)
	}
	if got := args[len(args)-1]; got != "/home/c_x/public_html" {
		t.Errorf("the source is not the last argument: %v", args)
	}
}

// tar stores links as links on its own. -h is the flag that would introduce the
// very behaviour -y removes from zip, so it must never appear.
func TestArchiveCommandDoesNotDereferenceForTar(t *testing.T) {
	tool, args := archiveCommand("tar.gz", "/home/c_x/out.tar.gz", []string{"/home/c_x/a", "/home/c_x/b"})
	if tool != "tar" {
		t.Fatalf("tool = %q, want tar", tool)
	}
	for _, forbidden := range []string{"-h", "--dereference"} {
		if slices.Contains(args, forbidden) {
			t.Errorf("the tar arguments carry %q: %v", forbidden, args)
		}
	}
	if !slices.Equal(args[len(args)-2:], []string{"/home/c_x/a", "/home/c_x/b"}) {
		t.Errorf("the sources are not passed through in order: %v", args)
	}
}

// The behaviour the flag is there for, measured against the real tool: the same
// tree archived with and without -y, read back through unzip.
func TestZipFollowsSymlinksWithoutTheFlag(t *testing.T) {
	zip, err := exec.LookPath("zip")
	if err != nil {
		t.Skip("zip is unavailable")
	}
	unzip, err := exec.LookPath("unzip")
	if err != nil {
		t.Skip("unzip is unavailable")
	}
	dir := t.TempDir()
	const secret = "content-outside-the-jail\n"
	if err := os.WriteFile(filepath.Join(dir, "outside.txt"), []byte(secret), 0o600); err != nil {
		t.Fatalf("write the target: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "home"), 0o750); err != nil {
		t.Fatalf("create the source tree: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "outside.txt"), filepath.Join(dir, "home", "link.txt")); err != nil {
		t.Fatalf("create the link: %v", err)
	}

	read := func(name string, args []string) string {
		t.Helper()
		build := exec.Command(zip, args...)
		build.Dir = dir
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("zip %v: %v\n%s", args, err, out)
		}
		out, err := exec.Command(unzip, "-p", filepath.Join(dir, name), "home/link.txt").Output()
		if err != nil {
			t.Fatalf("unzip %s: %v", name, err)
		}
		return string(out)
	}

	_, withFlag := archiveCommand("zip", filepath.Join(dir, "safe.zip"), []string{"home"})
	if got := read("safe.zip", withFlag); strings.Contains(got, secret) {
		t.Errorf("the archive built with our arguments contains the target's content: %q", got)
	}

	_, withoutFlag := archiveCommand("zip", filepath.Join(dir, "unsafe.zip"), []string{"home"})
	withoutFlag = slices.DeleteFunc(withoutFlag, func(a string) bool { return a == "-y" })
	if got := read("unsafe.zip", withoutFlag); !strings.Contains(got, secret) {
		t.Errorf("dropping -y no longer packages the target's content, so the flag guards nothing: %q", got)
	}
}

// The tree-walking tools are pointed at a /proc/self/fd path, so they must
// dereference that ONE argument and nothing else. -L dereferences every symlink
// met during the walk, which put the whole host inside a tenant's search results
// and inside their reported folder size.
func TestWalkArgumentsDereferenceOnlyTheStartingPoint(t *testing.T) {
	find := searchArgs("/proc/self/fd/7", "*needle*")
	if !slices.Contains(find, "-H") {
		t.Errorf("find is not given -H, so it cannot enter the pinned directory: %v", find)
	}
	if slices.Contains(find, "-L") {
		t.Errorf("find is given -L, which follows tenant symlinks out of the tree: %v", find)
	}
	if find[1] != "/proc/self/fd/7" {
		t.Errorf("the starting point is not the first operand: %v", find)
	}

	du := sizeArgs("/proc/self/fd/7")
	if !slices.Contains(du, "-D") {
		t.Errorf("du is not given -D, so it measures the link rather than the directory: %v", du)
	}
	for _, forbidden := range []string{"-L", "--dereference", "-sbL"} {
		if slices.Contains(du, forbidden) {
			t.Errorf("du is given %q, which measures whatever tenant symlinks point at: %v", forbidden, du)
		}
	}
}
