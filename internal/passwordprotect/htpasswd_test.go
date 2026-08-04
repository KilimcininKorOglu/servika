package passwordprotect

import (
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sentinelPassword = "SentinelProtect234"

// argv is world-readable through /proc/<pid>/cmdline and a tenant reaches that
// window with arbitrary shell from a cron entry, so the directory password must
// arrive on stdin.
func TestHtpasswdCommandKeepsThePasswordOutOfArgv(t *testing.T) {
	cmd := htpasswdCommand("/etc/nginx/htpasswd/d1_private", "alice", sentinelPassword, false)

	joined := strings.Join(cmd.Args, " ")
	if strings.Contains(joined, sentinelPassword) {
		t.Fatalf("the directory password reached argv: %q", joined)
	}
	if !strings.Contains(joined, "alice") || !strings.Contains(joined, "d1_private") {
		t.Errorf("argv %q lost the user or the file", joined)
	}

	stdin, err := io.ReadAll(cmd.Stdin)
	if err != nil {
		t.Fatalf("read stdin: %v", err)
	}
	if string(stdin) != sentinelPassword+"\n" {
		t.Errorf("stdin = %q, want the password and one terminator", stdin)
	}
}

// -c creates the file and would truncate an existing one, so it must appear
// only when the file is new.
func TestHtpasswdCommandAddsCreateFlagOnlyForANewFile(t *testing.T) {
	appendArgs := strings.Join(htpasswdCommand("/f", "alice", "p", false).Args, " ")
	if strings.Contains(appendArgs, "-ciB") {
		t.Errorf("append run uses %q, which would truncate the existing users out of the file", appendArgs)
	}
	if !strings.Contains(appendArgs, "-iB") {
		t.Errorf("append run uses %q, which does not read the password from stdin", appendArgs)
	}
	if createArgs := strings.Join(htpasswdCommand("/f", "alice", "p", true).Args, " "); !strings.Contains(createArgs, "-ciB") {
		t.Errorf("create run uses %q, which does not create the file", createArgs)
	}
}

// The flags are bundled and the password never touches the file the way the
// command line spells it, so run the real binary end to end where it exists and
// verify htpasswd both accepts the form and stores the password unchanged.
func TestHtpasswdStoresThePasswordItReadsFromStdin(t *testing.T) {
	if _, err := exec.LookPath("htpasswd"); err != nil {
		t.Skip("htpasswd is not installed on this host")
	}
	file := filepath.Join(t.TempDir(), "htpasswd")

	if out, err := htpasswdCommand(file, "alice", sentinelPassword, true).CombinedOutput(); err != nil {
		t.Fatalf("create run failed: %v: %s", err, out)
	}
	if out, err := htpasswdCommand(file, "bob", "SecondPass234", false).CombinedOutput(); err != nil {
		t.Fatalf("append run failed: %v: %s", err, out)
	}

	for _, user := range []struct{ name, password string }{
		{name: "alice", password: sentinelPassword},
		{name: "bob", password: "SecondPass234"},
	} {
		// #nosec G204 -- test-owned temp path and literal credentials.
		if out, err := exec.Command("htpasswd", "-vb", file, user.name, user.password).CombinedOutput(); err != nil {
			t.Errorf("%s cannot log in with the password that was sent: %v: %s", user.name, err, out)
		}
	}
	// The append run must not have dropped the first user.
	// #nosec G204 -- test-owned temp path and literal credentials.
	if out, err := exec.Command("htpasswd", "-vb", file, "alice", sentinelPassword).CombinedOutput(); err != nil {
		t.Errorf("the append run truncated the file: %v: %s", err, out)
	}
}
