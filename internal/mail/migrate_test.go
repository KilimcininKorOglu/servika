package mail

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// A recording driver, in the same shape as the one in purge_test.go: the
// repository carries no sqlmock dependency, and what has to be asserted here is
// which statement ran and what the queue then did with the row.
type migrateRecorder struct {
	mu       sync.Mutex
	steps    []string
	insertID int64
	// claimed is how many rows the claim UPDATE reports. Zero stands for a job
	// that was cancelled, or closed by the startup heal, while it waited.
	claimed int64
	// failClaim makes the claim UPDATE return an error rather than a count.
	failClaim bool
}

func (r *migrateRecorder) record(step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, step)
}

// sawStatement reports whether any recorded statement contains the fragment.
func (r *migrateRecorder) sawStatement(fragment string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, step := range r.steps {
		if strings.Contains(step, fragment) {
			return true
		}
	}
	return false
}

var (
	migrateStateMu sync.Mutex
	migrateState   = map[string]*migrateRecorder{}
	errMigrateExec = errors.New("statement failed")
)

type migrateDriver struct{}

func (migrateDriver) Open(name string) (driver.Conn, error) {
	migrateStateMu.Lock()
	defer migrateStateMu.Unlock()
	recorder, ok := migrateState[name]
	if !ok {
		return nil, fmt.Errorf("no recorder registered for %q", name)
	}
	return &migrateConn{recorder: recorder}, nil
}

func init() { sql.Register("mail_migrate_recorder", migrateDriver{}) }

type migrateConn struct{ recorder *migrateRecorder }

func (c *migrateConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not used by this test")
}
func (c *migrateConn) Close() error              { return nil }
func (c *migrateConn) Begin() (driver.Tx, error) { return nil, errors.New("no transactions here") }

type migrateResult struct{ id, rows int64 }

func (r migrateResult) LastInsertId() (int64, error) { return r.id, nil }
func (r migrateResult) RowsAffected() (int64, error) { return r.rows, nil }

func (c *migrateConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.recorder.record(query)
	switch {
	case strings.Contains(query, "INSERT INTO mail_migration_jobs"):
		return migrateResult{id: c.recorder.insertID, rows: 1}, nil
	case strings.Contains(query, "SET status='running'"):
		if c.recorder.failClaim {
			return nil, errMigrateExec
		}
		return migrateResult{rows: c.recorder.claimed}, nil
	}
	return migrateResult{rows: 1}, nil
}

func migrateHarness(t *testing.T) (*sql.DB, *migrateRecorder) {
	t.Helper()
	recorder := &migrateRecorder{insertID: 41, claimed: 1}
	name := t.Name()

	migrateStateMu.Lock()
	migrateState[name] = recorder
	migrateStateMu.Unlock()
	t.Cleanup(func() {
		migrateStateMu.Lock()
		delete(migrateState, name)
		migrateStateMu.Unlock()
	})

	db, err := sql.Open("mail_migrate_recorder", name)
	if err != nil {
		t.Fatalf("open the recording database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// The queue is package state, so a test that leaves entries behind would
	// change the next one's result.
	drainMigrationQueue()
	t.Cleanup(drainMigrationQueue)
	return db, recorder
}

func drainMigrationQueue() {
	for {
		select {
		case <-migrationQueue:
		default:
			return
		}
	}
}

// swapCopy replaces the copy with one that only records that it was reached, so
// the claim guard can be tested without an IMAP server anywhere.
func swapCopy(t *testing.T, ran *bool) {
	t.Helper()
	original := copyMailboxFn
	copyMailboxFn = func(context.Context, *sql.DB, int64, int64, RemoteAccount) error {
		*ran = true
		return nil
	}
	t.Cleanup(func() { copyMailboxFn = original })
}

// Only four copies run at a time, so a started job is WAITING, not running. A
// row written as running would make the screen show a copy that is not
// happening, and the status endpoint would then contradict the start response.
func TestAStartedMigrationIsWrittenAsQueuedAndPutOnTheQueue(t *testing.T) {
	db, recorder := migrateHarness(t)

	remote := RemoteAccount{
		Host: "imap.example.com", Port: 993, Security: "ssl",
		Username: "someone@example.com", Password: "the-remote-password",
	}
	id, err := startMigrationJob(db, 7, remote)
	if err != nil {
		t.Fatalf("startMigrationJob: %v", err)
	}
	if id != recorder.insertID {
		t.Errorf("id = %d, want %d", id, recorder.insertID)
	}
	if !recorder.sawStatement("'queued'") {
		t.Error("the job was not written as queued")
	}
	if recorder.sawStatement("'running'") {
		t.Error("the job was written as running before any worker claimed it")
	}

	select {
	case job := <-migrationQueue:
		if job.id != id || job.mailboxID != 7 {
			t.Errorf("queued job = %+v, want id %d for mailbox 7", job, id)
		}
		// The password is never stored, so the queue entry is the only place it
		// exists while the job waits. Losing it here would start a copy that
		// cannot sign in.
		if job.remote.Password != remote.Password {
			t.Error("the remote password did not travel with the queued job")
		}
	default:
		t.Fatal("the job was not put on the queue")
	}
}

// The wait list is bounded because every entry holds a credential in memory.
// Past the bound the panel has to say no, and the row it already wrote must not
// be left occupying the mailbox's only migration slot.
func TestTheWaitListRefusesInsteadOfGrowing(t *testing.T) {
	db, recorder := migrateHarness(t)
	for i := range maxQueuedMigrations {
		migrationQueue <- pendingMigration{id: int64(i)}
	}

	if _, err := startMigrationJob(db, 7, RemoteAccount{}); !errors.Is(err, ErrTooManyMigrations) {
		t.Fatalf("err = %v, want ErrTooManyMigrations", err)
	}
	if !recorder.sawStatement("too_many_migrations") {
		t.Error("the refused row was not closed, so it still holds the mailbox")
	}
}

// The other direction: with room on the queue the same call must succeed, or the
// bound above would be indistinguishable from a migration that never starts.
func TestAMigrationStartsWhenTheWaitListHasRoom(t *testing.T) {
	db, _ := migrateHarness(t)
	for i := range maxQueuedMigrations - 1 {
		migrationQueue <- pendingMigration{id: int64(i)}
	}

	if _, err := startMigrationJob(db, 7, RemoteAccount{}); err != nil {
		t.Fatalf("startMigrationJob with one free slot: %v", err)
	}
}

// A job cancelled while it waited has no goroutine for CancelMigration to
// interrupt, so that endpoint only writes the row. The claim guard is what makes
// the write bite.
func TestAJobCancelledWhileItWaitedIsNeverCopied(t *testing.T) {
	db, recorder := migrateHarness(t)
	recorder.claimed = 0

	var ran bool
	swapCopy(t, &ran)
	runMigrationJob(context.Background(), db, pendingMigration{id: 5, mailboxID: 7})

	if ran {
		t.Error("a job that was no longer queued was copied anyway")
	}
}

// The other direction: a row still queued must actually be copied, or the guard
// above would simply have stopped every migration.
func TestAJobStillQueuedIsCopied(t *testing.T) {
	db, recorder := migrateHarness(t)
	recorder.claimed = 1

	var ran bool
	swapCopy(t, &ran)
	runMigrationJob(context.Background(), db, pendingMigration{id: 5, mailboxID: 7})

	if !ran {
		t.Error("a queued job was claimed but never copied")
	}
}

// A claim that could not be written leaves the row queued on purpose: every
// reason code this package has blames the remote server, which did nothing
// wrong. Starting the copy anyway would run it against a row saying it never
// started.
func TestAClaimThatFailsDoesNotStartTheCopy(t *testing.T) {
	db, recorder := migrateHarness(t)
	recorder.failClaim = true

	var ran bool
	swapCopy(t, &ran)
	runMigrationJob(context.Background(), db, pendingMigration{id: 5, mailboxID: 7})

	if ran {
		t.Error("the copy ran even though the row was never claimed")
	}
	if recorder.sawStatement("SET status=?") {
		t.Error("an unclaimed job was reported as finished")
	}
}
