//go:build linux

package files

// The first tests for safeio. Everything the file manager does passes through
// these helpers, and until now nothing exercised them.
//
// They have to be Linux-only and they have to assert BOTH directions.
//
// Linux-only because the openat2 helpers exist only here: safeio_stub.go builds
// on every other platform and answers errSafeIOLinuxOnly to every call. A test
// without this tag would therefore run on a development machine against stubs
// that always fail, and would agree with a real implementation that always
// failed too. CI runs go test on ubuntu, so this file does run there; locally it
// needs a Linux container.
//
// Both directions because a refusal test on its own proves nothing about a
// function that refuses everything. "The symlink was rejected" stays true when
// the helper has been broken into returning an error unconditionally, so the
// positive assertions below are what separate a working guard from a dead one.

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeUnder creates rel under home with the given contents and returns its
// absolute path.
func writeUnder(t *testing.T, home, rel, contents string) string {
	t.Helper()
	full := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("create the parent of %q: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %q: %v", rel, err)
	}
	return full
}

// A dot-carrying directory name on purpose: a subdomain docroot
// (~/subdomains/<fqdn>) is the shape the file manager meets most often, and a
// path helper that mishandles it breaks extraction for every subdomain at once.
const docrootRel = "subdomains/test.example.com"

func TestStatBeneathReadsALegitimateFileAndDirectory(t *testing.T) {
	home := t.TempDir()
	writeUnder(t, home, docrootRel+"/archive.zip", "PK\x03\x04")

	info, err := statBeneath(home, docrootRel+"/archive.zip")
	if err != nil {
		t.Fatalf("a legitimate file could not be stat'd: %v", err)
	}
	if info.IsDir() {
		t.Error("IsDir reported true for a regular file")
	}
	if !info.Mode().IsRegular() {
		t.Errorf("mode = %v, want a regular file", info.Mode())
	}
	if info.Size() != 4 {
		t.Errorf("size = %d, want 4", info.Size())
	}

	// Extract decides whether the target is a folder from this same call, so the
	// directory case is not a variation of the file case; it is a second
	// contract.
	dirInfo, err := statBeneath(home, docrootRel)
	if err != nil {
		t.Fatalf("a legitimate directory could not be stat'd: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Error("IsDir reported false for a directory")
	}
}

// A tenant can put a unix socket in its own home, and Extract stats whatever
// path it is handed before deciding the target is not a regular file. The stat
// has to SUCCEED and report the type: an error here carries no information about
// whose fault it is, so it gets reported as a server fault for a path the tenant
// chose. O_RDONLY cannot open a socket at all, which is why statBeneath uses
// O_PATH.
func TestStatBeneathReadsAPathThatCannotBeOpenedForReading(t *testing.T) {
	home := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(home, "app.sock"))
	if err != nil {
		t.Fatalf("create the socket: %v", err)
	}
	defer func() { _ = listener.Close() }()

	info, err := statBeneath(home, "app.sock")
	if err != nil {
		t.Fatalf("a socket could not be stat'd: %v", err)
	}
	if info.Mode().IsRegular() {
		t.Error("a socket was reported as a regular file")
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Errorf("mode = %v, want the socket bit set", info.Mode())
	}
}

func TestStatBeneathRefusesASymlink(t *testing.T) {
	home := t.TempDir()
	writeUnder(t, home, docrootRel+"/archive.zip", "PK\x03\x04")

	// The relative cases are the load-bearing ones. An ABSOLUTE target is
	// refused by RESOLVE_BENEATH as well, so a suite of absolute targets alone
	// would stay green with RESOLVE_NO_SYMLINKS deleted; only a RELATIVE link
	// that stays inside home reaches that guard and nothing else.
	//
	// The reverse does not hold, and no test here claims it does:
	// RESOLVE_NO_SYMLINKS refuses every link before RESOLVE_BENEATH has an
	// opinion, and relClean normalises ".." away before the kernel sees it, so
	// RESOLVE_BENEATH is defence in depth with no input left that isolates it.
	//
	// That normalisation is also why the escape that does reach openat2 is a
	// link rather than a "../" path.
	if err := os.Symlink("/etc/passwd", filepath.Join(home, "escape.txt")); err != nil {
		t.Fatalf("create the escaping symlink: %v", err)
	}
	if err := os.Symlink(docrootRel+"/archive.zip",
		filepath.Join(home, "inside.zip")); err != nil {
		t.Fatalf("create the inside symlink: %v", err)
	}
	if err := os.Symlink(docrootRel, filepath.Join(home, "docroot")); err != nil {
		t.Fatalf("create the directory symlink: %v", err)
	}

	for _, tc := range []struct {
		name string
		rel  string
	}{
		{name: "the leaf leaves home", rel: "escape.txt"},
		{name: "the leaf is a relative link inside home", rel: "inside.zip"},
		{name: "an intermediate component is a relative link", rel: "docroot/archive.zip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := statBeneath(home, tc.rel); err == nil {
				t.Error("a symlinked path was stat'd instead of refused")
			}
		})
	}
}

// realPathBeneath is the one place safeio opens with O_PATH, which the kernel
// accepts alongside O_CLOEXEC, O_DIRECTORY and O_NOFOLLOW and rejects with
// EINVAL alongside anything else. openat2 does that rejection where openat would
// have ignored the extra bit, so a flag added here fails every call rather than
// being quietly dropped. Archive hands the resolved path to zip and tar, so that
// failure would take archive creation with it.
func TestRealPathBeneathResolvesALegitimatePath(t *testing.T) {
	home := t.TempDir()
	writeUnder(t, home, docrootRel+"/archive.zip", "PK\x03\x04")

	resolved, err := realPathBeneath(home, docrootRel+"/archive.zip")
	if err != nil {
		t.Fatalf("a legitimate path could not be resolved: %v", err)
	}
	if !strings.HasSuffix(resolved, docrootRel+"/archive.zip") {
		t.Errorf("resolved = %q, want it to end in %q", resolved, docrootRel+"/archive.zip")
	}
	// The path is handed to external tools, which write it into the archive, so
	// it must be a real filesystem path and not the /proc/self/fd stand-in the
	// helper resolves it through.
	if strings.Contains(resolved, "/proc/self/fd") {
		t.Errorf("resolved = %q, want a real path", resolved)
	}
}

func TestRealPathBeneathRefusesASymlink(t *testing.T) {
	home := t.TempDir()
	writeUnder(t, home, docrootRel+"/archive.zip", "PK\x03\x04")
	if err := os.Symlink("/etc", filepath.Join(home, "elsewhere")); err != nil {
		t.Fatalf("create the escaping symlink: %v", err)
	}
	// Relative and inside home, so RESOLVE_BENEATH has no objection and only
	// RESOLVE_NO_SYMLINKS can refuse it. See the comment in the statBeneath
	// refusal test.
	if err := os.Symlink(docrootRel+"/archive.zip",
		filepath.Join(home, "inside.zip")); err != nil {
		t.Fatalf("create the inside symlink: %v", err)
	}

	for _, rel := range []string{"elsewhere", "inside.zip"} {
		t.Run(rel, func(t *testing.T) {
			if _, err := realPathBeneath(home, rel); err == nil {
				t.Error("a symlinked path was resolved instead of refused")
			}
		})
	}
}

// RemoveAllBeneath is what a root-run cleanup uses instead of os.RemoveAll. The
// difference only shows when a component RESOLVES somewhere else: a string check
// on the path cannot see that, because the path still reads like a path inside
// the home.
func TestRemoveAllBeneathDeletesATreeItOwns(t *testing.T) {
	home := t.TempDir()
	writeUnder(t, home, docrootRel+"/index.html", "<h1>hello</h1>")
	writeUnder(t, home, docrootRel+"/nested/deep.txt", "deep")

	if err := RemoveAllBeneath(home, docrootRel); err != nil {
		t.Fatalf("a legitimate tree could not be removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, docrootRel)); !os.IsNotExist(err) {
		t.Errorf("the document root survived: %v", err)
	}
	// Only the named tree goes. Removing its parent as well would take every
	// other subdomain of the same tenant with it.
	if _, err := os.Lstat(filepath.Join(home, "subdomains")); err != nil {
		t.Errorf("the parent directory was removed too: %v", err)
	}
}

func TestRemoveAllBeneathRefusesToDeleteThroughASymlink(t *testing.T) {
	home := t.TempDir()
	// Stands in for anything outside the jail. It is created inside the temp
	// directory so a bug in the code under test cannot damage the machine
	// running the test, and it is reached through a RELATIVE link so only
	// RESOLVE_NO_SYMLINKS can refuse it.
	writeUnder(t, home, "elsewhere/test.example.com/keep.txt", "keep")
	if err := os.Symlink("elsewhere", filepath.Join(home, "subdomains")); err != nil {
		t.Fatalf("create the redirecting symlink: %v", err)
	}

	if err := RemoveAllBeneath(home, docrootRel); err == nil {
		t.Error("a removal was carried out through a symlinked parent")
	}
	if _, err := os.Lstat(filepath.Join(home, "elsewhere/test.example.com/keep.txt")); err != nil {
		t.Errorf("the file behind the symlink was deleted: %v", err)
	}
}

// A symlink named as the target is unlinked, not followed. Removing what it
// points at would let a tenant nominate anything root can reach for deletion.
func TestRemoveAllBeneathUnlinksALeafSymlinkWithoutFollowingIt(t *testing.T) {
	home := t.TempDir()
	writeUnder(t, home, "elsewhere/keep.txt", "keep")
	if err := os.Symlink("elsewhere", filepath.Join(home, "link")); err != nil {
		t.Fatalf("create the symlink: %v", err)
	}

	if err := RemoveAllBeneath(home, "link"); err != nil {
		t.Fatalf("the symlink itself could not be removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, "link")); !os.IsNotExist(err) {
		t.Errorf("the symlink survived: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, "elsewhere/keep.txt")); err != nil {
		t.Errorf("the target of the symlink was deleted: %v", err)
	}
}

// A new file has to be written, or the refusals below would be indistinguishable
// from a helper that never writes anything at all.
func TestStreamIntoBeneathWritesANewFile(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, docrootRel), 0o755); err != nil {
		t.Fatalf("create the docroot: %v", err)
	}

	const body = "the message body"
	n, err := StreamIntoBeneath(home, docrootRel+"/new.txt", strings.NewReader(body), "")
	if err != nil {
		t.Fatalf("StreamIntoBeneath: %v", err)
	}
	if n != int64(len(body)) {
		t.Errorf("wrote %d bytes, want %d", n, len(body))
	}
	got, err := os.ReadFile(filepath.Join(home, docrootRel, "new.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != body {
		t.Errorf("contents = %q, want %q", got, body)
	}
}

// The refusal an import depends on: a destination that already exists is never
// opened for writing, so an upload cannot replace a file that is already there.
func TestStreamIntoBeneathRefusesAnExistingFile(t *testing.T) {
	home := t.TempDir()
	writeUnder(t, home, docrootRel+"/kept.txt", "real mail")

	if _, err := StreamIntoBeneath(home, docrootRel+"/kept.txt",
		strings.NewReader("replacement"), ""); err == nil {
		t.Fatal("an existing file was written through")
	}
	got, err := os.ReadFile(filepath.Join(home, docrootRel, "kept.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "real mail" {
		t.Errorf("the existing file was altered: %q", got)
	}
}

// A hard link is not a symlink, so RESOLVE_NO_SYMLINKS does not refuse it. The
// O_EXCL create is what stops it: the link is an existing name, so nothing is
// opened and the file it points at keeps its contents.
func TestStreamIntoBeneathWillNotWriteThroughAHardLink(t *testing.T) {
	home := t.TempDir()
	target := writeUnder(t, home, docrootRel+"/target.txt", "must survive")
	link := filepath.Join(home, docrootRel, "planted.txt")
	if err := os.Link(target, link); err != nil {
		t.Skipf("hard links are unavailable here: %v", err)
	}

	if _, err := StreamIntoBeneath(home, docrootRel+"/planted.txt",
		strings.NewReader("overwritten"), ""); err == nil {
		t.Fatal("a hard link was written through")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read the link target: %v", err)
	}
	if string(got) != "must survive" {
		t.Errorf("the link target was altered: %q", got)
	}
}
