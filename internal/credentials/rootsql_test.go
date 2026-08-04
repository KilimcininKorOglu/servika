package credentials

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// stubRootSQL replaces the privileged MariaDB client with a script that records
// exactly what the process received, so a test can inspect argv and stdin
// separately instead of trusting the call site by reading it.
func stubRootSQL(t *testing.T, exitCode int) (argvPath, stdinPath string) {
	t.Helper()
	dir := t.TempDir()
	argvPath = filepath.Join(dir, "argv")
	stdinPath = filepath.Join(dir, "stdin")
	script := filepath.Join(dir, "mysql-stub")
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"" + argvPath + "\"\n" +
		"cat > \"" + stdinPath + "\"\n" +
		"[ " + strconv.Itoa(exitCode) + " -ne 0 ] && echo 'stub client failure' >&2\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write stub client: %v", err)
	}
	original := rootSQLCommand
	rootSQLCommand = func() *exec.Cmd { return exec.Command(script) }
	t.Cleanup(func() { rootSQLCommand = original })
	return argvPath, stdinPath
}

func readStub(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test-owned path under t.TempDir().
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	return string(data)
}

// A database password in argv is readable by every local account through
// /proc/<pid>/cmdline, and tenants reach that window through cron, so the
// statements must arrive on stdin and argv must stay empty.
func TestRunRootSQLKeepsTheStatementsOutOfArgv(t *testing.T) {
	argvPath, stdinPath := stubRootSQL(t, 0)

	const password = "SentinelPassword234"
	statements := []string{
		"CREATE USER IF NOT EXISTS 'c_example_com_app'@'localhost' IDENTIFIED BY '" + password + "';",
		"FLUSH PRIVILEGES;",
	}
	if err := runRootSQL(statements...); err != nil {
		t.Fatalf("runRootSQL() = %v, want nil", err)
	}

	argv := readStub(t, argvPath)
	if strings.Contains(argv, password) {
		t.Fatalf("the database password reached argv: %q", argv)
	}
	if strings.TrimSpace(argv) != "" {
		t.Fatalf("argv = %q, want no arguments at all", argv)
	}

	stdin := readStub(t, stdinPath)
	for _, statement := range statements {
		if !strings.Contains(stdin, statement) {
			t.Errorf("stdin is missing %q, so the statement never reached the client", statement)
		}
	}
}

// The client is invoked bare: root@localhost authenticates through the
// unix_socket plugin from the panel's own identity. A guard against reviving
// `mysql -e <sql>`, which is what put the password in argv.
func TestRootSQLCommandTakesNoArguments(t *testing.T) {
	if args := rootSQLCommand().Args; len(args) != 1 {
		t.Fatalf("rootSQLCommand().Args = %q, want the binary alone", args)
	}
}

// A failed grant or drop must not look like success, or the panel records an
// account the database never got.
func TestRunRootSQLSurfacesAClientFailure(t *testing.T) {
	stubRootSQL(t, 1)

	err := runRootSQL("SELECT 1;")
	if err == nil {
		t.Fatal("runRootSQL() = nil, want the client failure")
	}
	if !strings.Contains(err.Error(), "stub client failure") {
		t.Errorf("error %q drops the client output, leaving nothing to diagnose", err)
	}
}
