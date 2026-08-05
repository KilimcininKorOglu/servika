package sqlimport

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// An import target is named by the panel, but the guard runs before any account
// is created so a caller bug cannot point a scoped grant at the server's own
// tables.
func TestImportRefusesANonImportableTarget(t *testing.T) {
	for _, target := range []string{"mysql", "MySQL", "information_schema", "panel", "", "a-b", "has space", "x`y"} {
		t.Run(target, func(t *testing.T) {
			err := Import(context.Background(), target, strings.NewReader("SELECT 1;"))
			if !errors.Is(err, ErrInvalidTarget) {
				t.Errorf("Import(%q) = %v, want ErrInvalidTarget", target, err)
			}
		})
	}
}

// mysqldump stamps DEFINER onto views, triggers, routines and events. The
// importing account is low-privilege and cannot create an object owned by
// anyone else, so a surviving clause fails the whole restore.
func TestNormalizeRemovesBothClauseForms(t *testing.T) {
	input := "/*!50001 CREATE ALGORITHM=UNDEFINED */\n" +
		"/*!50013 DEFINER=`cpuser`@`localhost` SQL SECURITY DEFINER */\n" +
		"CREATE DEFINER=`cpuser`@`localhost` PROCEDURE `p`()\n" +
		"BEGIN SELECT 1; END;\n"

	got := readAll(t, normalize(strings.NewReader(input)))
	if strings.Contains(got, "cpuser") {
		t.Errorf("a DEFINER clause survived:\n%s", got)
	}
	for _, keep := range []string{"CREATE ALGORITHM=UNDEFINED", "PROCEDURE `p`()", "BEGIN SELECT 1; END;"} {
		if !strings.Contains(got, keep) {
			t.Errorf("the rewrite dropped %q:\n%s", keep, got)
		}
	}
	if lines := strings.Count(got, "\n"); lines != 4 {
		t.Errorf("line count = %d, want 4; the rewrite changed the dump's structure", lines)
	}
}

// Everything that is not a DEFINER clause must arrive byte for byte. A dump is
// data, and silently altering it corrupts the restored database.
func TestNormalizePassesUnrelatedContentThrough(t *testing.T) {
	input := "INSERT INTO `t` VALUES ('a\\'b'),('define'),('DEFINER'),('x');\n" +
		"-- a comment with no newline at the end"

	if got := readAll(t, normalize(strings.NewReader(input))); got != input {
		t.Errorf("normalize altered the dump:\ngot  %q\nwant %q", got, input)
	}
}

// A single INSERT line can exceed any buffer a dump author does not control. It
// must stream through intact rather than being truncated or buffered whole.
func TestNormalizeIsLosslessOnAnOverLongLine(t *testing.T) {
	long := strings.Repeat("abcdefgh", (longLineLimit/8)+512) // comfortably past the buffer
	input := "SET NAMES utf8mb4;\nINSERT INTO `t` VALUES ('" + long + "');\nCOMMIT;\n"

	got := readAll(t, normalize(strings.NewReader(input)))
	if got != input {
		t.Fatalf("an over-long line was not passed through losslessly: got %d bytes, want %d", len(got), len(input))
	}
}

// A dump whose last line has no trailing newline must not lose that line.
func TestNormalizeKeepsAnUnterminatedFinalLine(t *testing.T) {
	const input = "COMMIT;\nDEFINER=`a`@`b` -- trailing"
	got := readAll(t, normalize(strings.NewReader(input)))
	if !strings.HasSuffix(got, "-- trailing") {
		t.Errorf("the unterminated final line was lost: %q", got)
	}
	if strings.Contains(got, "DEFINER") {
		t.Errorf("the clause survived on the final line: %q", got)
	}
}

// A read failure must surface, never end the stream as if the dump had finished:
// a truncated import looks like a successful one to the client.
func TestNormalizePropagatesAReadFailure(t *testing.T) {
	want := errors.New("disk gave up")
	r := normalize(io.MultiReader(strings.NewReader("SET NAMES utf8mb4;\n"), errReader{want}))
	if _, err := io.ReadAll(r); !errors.Is(err, want) {
		t.Errorf("read error = %v, want %v", err, want)
	}
}

// The credentials must never reach argv, which is world-readable through
// /proc/<pid>/cmdline and which a tenant reaches with arbitrary shell via cron.
func TestWriteOptionFileKeepsTheCredentialsOutOfArgvAndOffOtherAccounts(t *testing.T) {
	const password = `p"a\ss'w ord`
	name, cleanup, err := writeOptionFile("svk_imp_abc", password)
	if err != nil {
		t.Fatalf("writeOptionFile() = %v, want nil", err)
	}
	defer cleanup()

	info, err := os.Stat(name)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("option file mode = %04o, want 0600: any local account could read the password", mode)
	}
	if strings.Contains(name, password) {
		t.Error("the password leaked into the file name")
	}

	body, err := os.ReadFile(name) // #nosec G304 -- path produced by the function under test.
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(body)
	if !strings.Contains(content, "user=svk_imp_abc") {
		t.Errorf("the option file names no user:\n%s", content)
	}
	// The client unescapes inside double quotes, so the escaped form is what
	// makes the password arrive as written.
	if !strings.Contains(content, `password="p\"a\\ss'w ord"`) {
		t.Errorf("the password is not escaped for a quoted option value:\n%s", content)
	}

	cleanup()
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Error("the option file outlives the import")
	}
}

// A dump naming its SOURCE schema must not abort the import against an account
// that has no rights there. Dropping these lines is compatibility only, which is
// why the guarantee lives in the account and not here.
func TestNormalizeDropsSchemaSelectionButNotSimilarLines(t *testing.T) {
	input := "CREATE DATABASE `cpuser_old`;\n" +
		"USE `cpuser_old`;\n" +
		"\tuse cpuser_old;\n" +
		"USE`cpuser_old`;\n" +
		"INSERT INTO `t` VALUES ('USE mysql');\n" +
		"-- USE is mentioned in this comment\n"

	got := readAll(t, normalize(strings.NewReader(input)))
	if strings.Contains(got, "cpuser_old") {
		t.Errorf("a schema-selection line survived:\n%s", got)
	}
	if !strings.Contains(got, "INSERT INTO `t` VALUES ('USE mysql');") {
		t.Errorf("a data row mentioning USE was dropped:\n%s", got)
	}
	if !strings.Contains(got, "-- USE is mentioned in this comment") {
		t.Errorf("a comment mentioning USE was dropped:\n%s", got)
	}
}

func TestOptionFileEscapeEscapesBothSpecialCharacters(t *testing.T) {
	if got, want := optionFileEscape(`a\b"c`), `a\\b\"c`; got != want {
		t.Errorf("optionFileEscape() = %q, want %q", got, want)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}
