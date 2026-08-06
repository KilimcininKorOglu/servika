package mail

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

// A recording driver, in the same shape as the failing connector in
// quota_test.go: the repository has no sqlmock dependency, and the statements a
// purge issues plus their order are exactly what has to be asserted.
type purgeRecorder struct {
	mu      sync.Mutex
	steps   []string
	failOn  string // a statement containing this substring returns an error
	diskRan bool
}

func (r *purgeRecorder) record(step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, step)
}

func (r *purgeRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.steps...)
}

var (
	purgeStateMu sync.Mutex
	purgeState   = map[string]*purgeRecorder{}
	errPurgeExec = errors.New("statement failed")
)

type purgeDriver struct{}

func (purgeDriver) Open(name string) (driver.Conn, error) {
	purgeStateMu.Lock()
	defer purgeStateMu.Unlock()
	recorder, ok := purgeState[name]
	if !ok {
		return nil, fmt.Errorf("no recorder registered for %q", name)
	}
	return &purgeConn{recorder: recorder}, nil
}

func init() { sql.Register("mail_purge_recorder", purgeDriver{}) }

type purgeConn struct{ recorder *purgeRecorder }

func (c *purgeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not used by this test")
}
func (c *purgeConn) Close() error { return nil }

func (c *purgeConn) Begin() (driver.Tx, error) {
	c.recorder.record("BEGIN")
	return &purgeTx{recorder: c.recorder}, nil
}

func (c *purgeConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.recorder.record(query)
	if c.recorder.failOn != "" && strings.Contains(query, c.recorder.failOn) {
		return nil, errPurgeExec
	}
	return driver.RowsAffected(1), nil
}

// QueryContext records the statement and answers the one lookup a handler needs
// before it can act, so a test can assert on which queries a refusal did NOT
// reach. Anything else comes back empty, which surfaces as sql.ErrNoRows.
func (c *purgeConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.recorder.record(query)
	if strings.Contains(query, "system_user") && strings.Contains(query, "FROM domains WHERE id=?") {
		return &recorderRows{
			columns: []string{"system_user", "is_demo"},
			values:  [][]driver.Value{{"c_example", int64(0)}},
		}, nil
	}
	return &recorderRows{}, nil
}

type recorderRows struct {
	columns []string
	values  [][]driver.Value
	next    int
}

func (r *recorderRows) Columns() []string { return r.columns }
func (r *recorderRows) Close() error      { return nil }
func (r *recorderRows) Next(dest []driver.Value) error {
	if r.next >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.next])
	r.next++
	return nil
}

type purgeTx struct{ recorder *purgeRecorder }

func (t *purgeTx) Commit() error   { t.recorder.record("COMMIT"); return nil }
func (t *purgeTx) Rollback() error { t.recorder.record("ROLLBACK"); return nil }

// purgeHarness opens a recording database and swaps the disk step for one that
// reports whether it ran and returns diskErr. The real remover is Linux-only, so
// without this every macOS run would take the failure branch and the success
// path would never be tested.
func purgeHarness(t *testing.T, failOn string, diskErr error) (*sql.DB, *purgeRecorder) {
	t.Helper()
	recorder := &purgeRecorder{failOn: failOn}
	name := t.Name()

	purgeStateMu.Lock()
	purgeState[name] = recorder
	purgeStateMu.Unlock()
	t.Cleanup(func() {
		purgeStateMu.Lock()
		delete(purgeState, name)
		purgeStateMu.Unlock()
	})

	original := removeMailFiles
	removeMailFiles = func(string) error {
		recorder.mu.Lock()
		recorder.diskRan = true
		recorder.mu.Unlock()
		return diskErr
	}
	t.Cleanup(func() { removeMailFiles = original })

	db, err := sql.Open("mail_purge_recorder", name)
	if err != nil {
		t.Fatalf("open the recording database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, recorder
}

// The five tables listed here hang off domains(id), not mail_domains(id), so
// nothing deletes them on the panel's behalf. Dropping one from the list would
// hand its rows straight back the next time the domain enabled mail.
//
// mail_domains has to go LAST because it is the row that cascades: mailboxes go
// with it, and their autoresponders and filters go with them.
func TestPurgeDeletesEveryNonCascadingTableInOneTransaction(t *testing.T) {
	db, recorder := purgeHarness(t, "", nil)

	diskFailed, err := PurgeDomain(context.Background(), db, 7, "c_example")
	if err != nil {
		t.Fatalf("PurgeDomain: %v", err)
	}
	if diskFailed {
		t.Error("diskFailed is set although the removal succeeded")
	}

	steps := recorder.recorded()
	if len(steps) == 0 || steps[0] != "BEGIN" {
		t.Fatalf("the work did not start in a transaction: %v", steps)
	}
	if steps[len(steps)-1] != "COMMIT" {
		t.Errorf("the transaction was not committed: %v", steps)
	}
	for _, table := range []string{
		"mail_aliases", "mail_send_log", "mail_spam_settings",
		"mail_delivery_log", "webmail_tokens",
	} {
		want := "DELETE FROM " + table + " WHERE domain_id=?"
		if !containsStep(steps, want) {
			t.Errorf("%s was never deleted; its rows would survive the purge", table)
		}
	}
	mailDomains := indexOfStep(steps, "DELETE FROM mail_domains WHERE domain_id=?")
	if mailDomains < 0 {
		t.Fatalf("mail_domains was never deleted: %v", steps)
	}
	if mailDomains != len(steps)-2 {
		t.Errorf("mail_domains is not the last delete before COMMIT: %v", steps)
	}
	if !recorder.diskRan {
		t.Error("the files were never removed")
	}
}

// A half-applied purge is the worst outcome: rows gone, files still charged to
// the customer, and no record of which half failed. The transaction has to roll
// back, and the disk must not be touched at all.
func TestPurgeRollsBackAndLeavesTheDiskAloneOnDatabaseFailure(t *testing.T) {
	db, recorder := purgeHarness(t, "mail_spam_settings", nil)

	if _, err := PurgeDomain(context.Background(), db, 7, "c_example"); err == nil {
		t.Fatal("PurgeDomain reported success although a statement failed")
	}

	steps := recorder.recorded()
	if containsStep(steps, "COMMIT") {
		t.Errorf("the failed transaction was committed: %v", steps)
	}
	if !containsStep(steps, "ROLLBACK") {
		t.Errorf("the failed transaction was not rolled back: %v", steps)
	}
	if recorder.diskRan {
		t.Error("the files were removed even though the database work failed")
	}
}

// The database really is clean, so this is not an error: the service is gone.
// The files are not, though, and they keep occupying the customer's disk, so the
// caller has to be told rather than shown a plain success.
func TestPurgeReportsFilesThatCouldNotBeRemoved(t *testing.T) {
	db, recorder := purgeHarness(t, "", errors.New("device is busy"))

	diskFailed, err := PurgeDomain(context.Background(), db, 7, "c_example")
	if err != nil {
		t.Fatalf("a disk failure must not fail the whole purge: %v", err)
	}
	if !diskFailed {
		t.Error("the disk failure was swallowed")
	}
	if !containsStep(recorder.recorded(), "COMMIT") {
		t.Error("the database work was rolled back although only the disk failed")
	}
}

// A domain that never received a message, or whose Linux user is already gone,
// has nothing to delete. Reporting that as a partial failure would send the
// customer looking for files that do not exist.
func TestPurgeTreatsAMissingMaildirAsSuccess(t *testing.T) {
	db, _ := purgeHarness(t, "", fmt.Errorf("open mail: %w", fs.ErrNotExist))

	diskFailed, err := PurgeDomain(context.Background(), db, 7, "c_example")
	if err != nil {
		t.Fatalf("PurgeDomain: %v", err)
	}
	if diskFailed {
		t.Error("a missing Maildir was reported as a failure to remove files")
	}
}

// /mail/service and /mail/{mid} are the same shape under the same method, and
// chi resolves that by preferring the static segment. The consequence of getting
// it wrong is not a 404: a customer deleting one mailbox would lose the entire
// service. This asserts the router rule the real registration in
// cmd/server/main.go relies on, which /mail/enable already depends on today.
func TestServiceRouteIsNotSwallowedByTheMailboxRoute(t *testing.T) {
	router := chi.NewRouter()
	router.Delete("/domains/{id}/mail/service", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("purge"))
	})
	router.Delete("/domains/{id}/mail/{mid}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("mailbox"))
	})

	for _, tc := range []struct{ path, want string }{
		{"/domains/1/mail/service", "purge"},
		{"/domains/1/mail/7", "mailbox"},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, tc.path, nil))
		if got := recorder.Body.String(); got != tc.want {
			t.Errorf("DELETE %s reached the %s handler, want %s", tc.path, got, tc.want)
		}
	}
}

func containsStep(steps []string, want string) bool { return indexOfStep(steps, want) >= 0 }

func indexOfStep(steps []string, want string) int {
	for i, step := range steps {
		if strings.TrimSpace(step) == want {
			return i
		}
	}
	return -1
}
