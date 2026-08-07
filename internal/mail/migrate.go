package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// migrationBudget is how long one copy may run before it is abandoned.
//
// It is generous because a large mailbox on a slow provider genuinely takes
// hours, and short because a job that has stopped making progress must not hold
// a mailbox's only migration slot for ever.
const migrationBudget = 12 * time.Hour

// migrationBatch is how many messages are fetched per FETCH command. Large
// enough that a mailbox of thousands does not cost thousands of round trips,
// small enough that progress is reported while it runs.
const migrationBatch = 200

// ErrMigrationRunning reports that this mailbox already has an unfinished job.
var ErrMigrationRunning = errors.New("a migration is already running for this mailbox")

// RemoteAccount is the source of a copy. The password is never stored; it lives
// in this value for as long as the job runs and nowhere else.
type RemoteAccount struct {
	Host     string
	Port     int
	Security string
	Username string
	Password string
}

// running tracks the cancel function of every job in flight, so the delete
// endpoint can stop one rather than only marking the row.
var running = struct {
	sync.Mutex
	cancels map[int64]context.CancelFunc
}{cancels: map[int64]context.CancelFunc{}}

func rememberJob(id int64, cancel context.CancelFunc) {
	running.Lock()
	running.cancels[id] = cancel
	running.Unlock()
}

func forgetJob(id int64) {
	running.Lock()
	delete(running.cancels, id)
	running.Unlock()
}

// cancelMigrationJob stops a running job. It reports whether one was in flight in
// this process; a job left by an earlier process has no goroutine to stop.
func cancelMigrationJob(id int64) bool {
	running.Lock()
	cancel, found := running.cancels[id]
	running.Unlock()
	if found {
		cancel()
	}
	return found
}

// startMigrationJob records the job and copies in the background.
//
// The insert is what refuses a second job: the unique index over the active
// mailbox does it in the database, so two requests arriving together cannot both
// decide they are the first.
func startMigrationJob(db *sql.DB, mailboxID int64, remote RemoteAccount) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO mail_migration_jobs
		   (mailbox_id, remote_host, remote_port, remote_security, remote_user, status, started_at)
		 VALUES (?,?,?,?,?, 'running', NOW())`,
		mailboxID, remote.Host, remote.Port, remote.Security, remote.Username)
	if err != nil {
		if isDuplicateKey(err) {
			return 0, ErrMigrationRunning
		}
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// The work outlives the request that started it, so it gets its own context
	// rather than the request's, which is cancelled the moment the tab closes.
	ctx, cancel := context.WithTimeout(context.Background(), migrationBudget)
	rememberJob(id, cancel)

	go func() {
		defer cancel()
		defer forgetJob(id)
		err := copyMailbox(ctx, db, id, mailboxID, remote)
		finishJob(db, id, err)
	}()
	return id, nil
}

func isDuplicateKey(err error) bool {
	// The driver's error is formatted, and matching its text is enough here:
	// the only unique constraint on this table is the active-job one.
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}

// finishJob writes the outcome on a context of its own.
//
// The job's own context is cancelled or expired by this point, so reusing it
// would drop the very update that records what happened.
func finishJob(db *sql.DB, id int64, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status, code := "done", ""
	switch {
	case cause == nil:
	case errors.Is(cause, context.Canceled):
		status, code = "cancelled", ""
	case errors.Is(cause, context.DeadlineExceeded):
		status, code = "failed", "timed_out"
	default:
		status, code = "failed", reasonFor(cause)
		// #nosec G706 -- the job id is an integer and the reason is one of this package's own constants; the wrapped error is the library's or the kernel's, never a raw remote string.
		log.Printf("mail migration job=%d failed: %v", id, cause)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE mail_migration_jobs SET status=?, error_code=?, finished_at=NOW() WHERE id=?`,
		status, code, id); err != nil {
		// #nosec G706 -- integer id and a fixed status word.
		log.Printf("mail migration job=%d: could not record %s: %v", id, status, err)
	}
}

// copyMailbox performs the copy, folder by folder.
func copyMailbox(ctx context.Context, db *sql.DB, jobID, mailboxID int64, remote RemoteAccount) error {
	layout, err := layoutFor(ctx, db, mailboxID)
	if err != nil {
		return err
	}

	client, err := dialIMAP(ctx, remote.Host, remote.Port, remote.Security)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := client.Login(remote.Username, remote.Password).Wait(); err != nil {
		if code := providerHint(remote.Host, remote.Username); code != "" {
			return &ReasonError{Code: code, Err: err}
		}
		return &ReasonError{Code: ReasonAuthFailed, Err: err}
	}

	folders, err := client.List("", "*", nil).Collect()
	if err != nil {
		return &ReasonError{Code: ReasonUnreachable, Err: err}
	}

	selectable := make([]*imap.ListData, 0, len(folders))
	for _, folder := range folders {
		if hasAttr(folder.Attrs, imap.MailboxAttrNoSelect) || hasAttr(folder.Attrs, imap.MailboxAttrNonExistent) {
			continue
		}
		selectable = append(selectable, folder)
	}
	setCounter(ctx, db, jobID, "folders_total", len(selectable))

	for index, folder := range selectable {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyFolder(ctx, db, client, layout, jobID, folder); err != nil {
			return err
		}
		setCounter(ctx, db, jobID, "folders_done", index+1)
	}
	return nil
}

func hasAttr(attrs []imap.MailboxAttr, want imap.MailboxAttr) bool {
	for _, attr := range attrs {
		if attr == want {
			return true
		}
	}
	return false
}

// copyFolder selects one remote folder and writes every message it holds.
func copyFolder(ctx context.Context, db *sql.DB, client *imapclient.Client, layout maildirLayout, jobID int64, folder *imap.ListData) error {
	selected, err := client.Select(folder.Mailbox, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		// A folder that cannot be opened is skipped rather than failing the
		// whole migration: one unreadable folder must not cost the customer
		// every other one.
		// #nosec G706 -- the folder name is not logged; only the job id and the library's error.
		log.Printf("mail migration job=%d: skipping an unreadable folder: %v", jobID, err)
		return nil
	}
	if selected.NumMessages == 0 {
		return nil
	}

	curDir, err := layout.ensureFolder(maildirSubdir(folder.Mailbox, folder.Delim))
	if err != nil {
		return err
	}

	addCounter(ctx, db, jobID, "messages_total", int(selected.NumMessages))

	for start := uint32(1); start <= selected.NumMessages; start += migrationBatch {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + migrationBatch - 1
		if end > selected.NumMessages {
			end = selected.NumMessages
		}
		copied, written, err := copyBatch(ctx, client, layout, curDir, jobID, start, end)
		if err != nil {
			return err
		}
		addCounter(ctx, db, jobID, "messages_done", copied)
		addCounter(ctx, db, jobID, "bytes_done", int(written))
	}
	return nil
}

// copyBatch fetches and writes one run of messages.
func copyBatch(ctx context.Context, client *imapclient.Client, layout maildirLayout, curDir string, jobID int64, start, end uint32) (int, int64, error) {
	section := &imap.FetchItemBodySection{Peek: true}
	var window imap.SeqSet
	window.AddRange(start, end)
	fetch := client.Fetch(window, &imap.FetchOptions{
		UID:          true,
		Flags:        true,
		BodySection:  []*imap.FetchItemBodySection{section},
		InternalDate: true,
	})
	defer func() { _ = fetch.Close() }()

	var (
		copied  int
		written int64
	)
	for {
		if err := ctx.Err(); err != nil {
			return copied, written, err
		}
		message := fetch.Next()
		if message == nil {
			break
		}

		var (
			uid   uint32
			flags []string
		)
		for {
			item := message.Next()
			if item == nil {
				break
			}
			switch data := item.(type) {
			case imapclient.FetchItemDataUID:
				uid = uint32(data.UID)
			case imapclient.FetchItemDataFlags:
				flags = flags[:0]
				for _, flag := range data.Flags {
					flags = append(flags, string(flag))
				}
			case imapclient.FetchItemDataBodySection:
				// Streamed rather than buffered: a mailbox can hold messages
				// larger than the panel's whole memory budget.
				size, err := layout.writeMessage(curDir,
					fmt.Sprintf("servika-%d-%d", jobID, uid), flags, data.Literal)
				if err != nil {
					return copied, written, err
				}
				copied++
				written += size
			}
		}
	}
	if err := fetch.Close(); err != nil {
		return copied, written, &ReasonError{Code: ReasonUnreachable, Err: err}
	}
	return copied, written, nil
}

// layoutFor resolves where a mailbox's files belong.
func layoutFor(ctx context.Context, db *sql.DB, mailboxID int64) (maildirLayout, error) {
	var (
		maildir    string
		systemUser string
	)
	err := db.QueryRowContext(ctx,
		`SELECT m.maildir, d.system_user
		   FROM mailboxes m
		   JOIN mail_domains d ON d.id = m.mail_domain_id
		  WHERE m.id = ?`, mailboxID).Scan(&maildir, &systemUser)
	if err != nil {
		return maildirLayout{}, err
	}
	if systemUser == "" || maildir == "" {
		return maildirLayout{}, fmt.Errorf("mailbox %d has no maildir", mailboxID)
	}

	home := "/home/" + systemUser
	rel, inside := strings.CutPrefix(maildir, home+"/")
	if !inside {
		return maildirLayout{}, fmt.Errorf("mailbox %d stores mail outside its home", mailboxID)
	}
	return maildirLayout{home: home, root: rel, systemUser: systemUser}, nil
}

// progressStatements holds the two ways each counter is written, as complete
// literals.
//
// Building the column name into the query would work, but it is dynamic SQL in a
// file that also handles a remote server's output, and a reader has to prove the
// name is safe every time they pass it. There is no query here that was not
// written out in full.
var progressStatements = map[string]struct{ set, add string }{
	"folders_total": {
		set: `UPDATE mail_migration_jobs SET folders_total=? WHERE id=?`,
		add: `UPDATE mail_migration_jobs SET folders_total=folders_total+? WHERE id=?`,
	},
	"folders_done": {
		set: `UPDATE mail_migration_jobs SET folders_done=? WHERE id=?`,
		add: `UPDATE mail_migration_jobs SET folders_done=folders_done+? WHERE id=?`,
	},
	"messages_total": {
		set: `UPDATE mail_migration_jobs SET messages_total=? WHERE id=?`,
		add: `UPDATE mail_migration_jobs SET messages_total=messages_total+? WHERE id=?`,
	},
	"messages_done": {
		set: `UPDATE mail_migration_jobs SET messages_done=? WHERE id=?`,
		add: `UPDATE mail_migration_jobs SET messages_done=messages_done+? WHERE id=?`,
	},
	"bytes_done": {
		set: `UPDATE mail_migration_jobs SET bytes_done=? WHERE id=?`,
		add: `UPDATE mail_migration_jobs SET bytes_done=bytes_done+? WHERE id=?`,
	},
}

func isProgressColumn(column string) bool {
	_, known := progressStatements[column]
	return known
}

// setCounter writes an absolute progress value.
func setCounter(ctx context.Context, db *sql.DB, jobID int64, column string, value int) {
	statement, known := progressStatements[column]
	if !known {
		return
	}
	runProgress(ctx, db, jobID, column, statement.set, value)
}

// addCounter advances a progress value, so a folder's contribution is added to
// what earlier folders already reported.
func addCounter(ctx context.Context, db *sql.DB, jobID int64, column string, value int) {
	statement, known := progressStatements[column]
	if !known || value == 0 {
		return
	}
	runProgress(ctx, db, jobID, column, statement.add, value)
}

// runProgress records a counter. A failure to write progress does not stop the
// copy: the messages are what the customer came for, and the number on screen
// standing still is better than the transfer dying over it.
func runProgress(ctx context.Context, db *sql.DB, jobID int64, column, statement string, value int) {
	if _, err := db.ExecContext(ctx, statement, value, jobID); err != nil {
		// #nosec G706 -- integer id and a key from the fixed map above.
		log.Printf("mail migration job=%d: could not record %s: %v", jobID, column, err)
	}
}

// HealMigrationJobs closes out jobs the previous process was running.
//
// Their goroutines died with it, so the rows would otherwise say "running" for
// ever and the unique index would keep the mailbox's slot occupied.
func HealMigrationJobs(db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := db.ExecContext(ctx,
		`UPDATE mail_migration_jobs
		    SET status='failed', error_code='interrupted', finished_at=NOW()
		  WHERE status IN ('queued','running')`)
	if err != nil {
		log.Printf("mail migration heal: %v", err)
		return
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		log.Printf("mail migration heal: %d job(s) left running by a previous process were closed", affected)
	}
}
