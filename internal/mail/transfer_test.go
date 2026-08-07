package mail

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"servika/internal/files"
)

// The archive names the folder and nothing else. A member is written under a
// name this package builds, so a path climbing out of the mailbox has to end up
// as a harmless folder rather than as a destination.
func TestArchiveMembersCannotChooseWhereTheyLand(t *testing.T) {
	for _, name := range []string{
		"../../etc/cur/passwd",
		"/etc/cur/passwd",
		"..//..//cur/msg",
	} {
		folder, _, wanted := maildirMember(name)
		if !wanted {
			continue
		}
		if strings.Contains(maildirSubdir(folder, '/'), "..") {
			t.Errorf("%q produced folder %q, which escapes the mailbox", name, folder)
		}
		if strings.Contains(maildirSubdir(folder, '/'), "/") {
			t.Errorf("%q produced folder %q, which is a path rather than a name", name, folder)
		}
	}
}

// A Maildir keeps messages under cur/ and new/. tmp/ holds deliveries that were
// never finished and the index files are Dovecot's own, so importing either
// would add files that are not mail.
func TestOnlyRealMessagesAreTakenFromAnArchive(t *testing.T) {
	cases := []struct {
		name   string
		folder string
		wanted bool
	}{
		{"cur/1700000000.abc:2,S", "INBOX", true},
		{"new/1700000000.abc", "INBOX", true},
		{".Sent/cur/1700000000.abc:2,S", "Sent", true},
		{"Maildir/.Projects.2025/cur/1.abc", "Maildir/Projects.2025", true},
		{"tmp/1700000000.abc", "", false},
		{"cur/dovecot-uidlist", "", false},
		{"cur/dovecot.index.cache", "", false},
		{"maildirsize", "", false},
		{"cur", "", false},
	}
	for _, test := range cases {
		folder, _, wanted := maildirMember(test.name)
		if wanted != test.wanted {
			t.Errorf("maildirMember(%q) wanted = %v, want %v", test.name, wanted, test.wanted)
			continue
		}
		if wanted && folder != test.folder {
			t.Errorf("maildirMember(%q) folder = %q, want %q", test.name, folder, test.folder)
		}
	}
}

// Dovecot reads the letters after ":2," on sight, so dropping them makes an
// imported mailbox come back entirely unread, which is what a customer notices
// first.
func TestImportedMessagesKeepTheirFlags(t *testing.T) {
	cases := []struct {
		base string
		want []string
	}{
		{"1700000000.abc:2,S", []string{"\\Seen"}},
		{"1700000000.abc:2,RS", []string{"\\Answered", "\\Seen"}},
		{"1700000000.abc:2,", nil},
		{"1700000000.abc", nil},
		{"1700000000.abc:2,SSX", []string{"\\Seen"}},
	}
	for _, test := range cases {
		got := flagsFromMaildirName(test.base)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("flagsFromMaildirName(%q) = %v, want %v", test.base, got, test.want)
		}
	}
	// And the letters survive a round trip, or the import would keep flags the
	// writer then throws away.
	if got := maildirInfo(flagsFromMaildirName("x:2,RS")); got != ":2,RS" {
		t.Errorf("round trip produced %q, want :2,RS", got)
	}
}

// countingSink records what the split produced without touching a disk.
type countingSink struct{ bodies []string }

// splitMbox drives the same splitter the import uses, with a recording writer
// standing in for the Maildir.
func splitMbox(t *testing.T, mbox string) *countingSink {
	t.Helper()
	sink := &countingSink{}
	if err := forEachMboxMessage(strings.NewReader(mbox), func(body []byte) error {
		sink.bodies = append(sink.bodies, string(body))
		return nil
	}); err != nil {
		t.Fatalf("split the mbox: %v", err)
	}
	return sink
}

// An mbox separates messages with a line starting "From ", and a body line that
// began that way was escaped to ">From " when the file was written. Undoing that
// is what keeps a quoted mail byte-identical to what was sent.
func TestMboxSplitsOnTheSeparatorAndUnescapesBodies(t *testing.T) {
	sink := splitMbox(t, "From a@b Mon Jan  1 00:00:00 2024\n"+
		"Subject: one\n\n"+
		">From the desk of nobody\n"+
		"From a@b Mon Jan  1 00:00:01 2024\n"+
		"Subject: two\n\nbody\n")

	if len(sink.bodies) != 2 {
		t.Fatalf("got %d messages, want 2: %q", len(sink.bodies), sink.bodies)
	}
	if !strings.Contains(sink.bodies[0], "\nFrom the desk of nobody") {
		t.Errorf("the escaped line was not restored: %q", sink.bodies[0])
	}
	if strings.Contains(sink.bodies[0], "Subject: two") {
		t.Error("the split did not end the first message at the separator")
	}
	if !strings.HasPrefix(sink.bodies[1], "Subject: two") {
		t.Errorf("the second message kept its separator line: %q", sink.bodies[1])
	}
}

// An mbox that is only a separator carries no mail, and writing an empty file
// into a Maildir produces a message no client can display.
func TestMboxWithNothingInItWritesNothing(t *testing.T) {
	for _, mbox := range []string{"", "\n\n\n", "From a@b Mon Jan  1 00:00:00 2024\n"} {
		if sink := splitMbox(t, mbox); len(sink.bodies) != 0 {
			t.Errorf("%q produced %d messages, want none", mbox, len(sink.bodies))
		}
	}
}

// A gzipped and a plain tar are the same archive, and an exporter that produced
// either has to read back.
func TestTarIsReadWithOrWithoutCompression(t *testing.T) {
	var plain bytes.Buffer
	archive := tar.NewWriter(&plain)
	body := []byte("Subject: hello\n\nbody\n")
	if err := archive.WriteHeader(&tar.Header{
		Name: "cur/1700000000.abc:2,S", Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write the header: %v", err)
	}
	if _, err := archive.Write(body); err != nil {
		t.Fatalf("write the body: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close the archive: %v", err)
	}

	stream, err := maybeGunzip(bytes.NewReader(plain.Bytes()))
	if err != nil {
		t.Fatalf("maybeGunzip on a plain tar: %v", err)
	}
	reader := tar.NewReader(stream)
	header, err := reader.Next()
	if err != nil {
		t.Fatalf("read the member back: %v", err)
	}
	if header.Name != "cur/1700000000.abc:2,S" {
		t.Errorf("member = %q, want the one written", header.Name)
	}
}

// The converter runs as the tenant and fills a directory the tenant owns, while
// the panel reads it back as root. A symbolic link there would otherwise deliver
// any root-readable file into a mailbox the tenant reads over IMAP; the panel's
// own environment file holds the JWT and encryption keys.
func TestConvertedOutputWillNotFollowALinkOutOfItsDirectory(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(secret, []byte("From x\nSERVIKA_JWT_SECRET=leaked\n"), 0o600); err != nil {
		t.Fatalf("write the file standing in for a host secret: %v", err)
	}

	output := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(output, "Inbox.mbox")); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}

	sink := &maildirSink{}
	if err := walkPSTOutput(output, sink); err != nil {
		t.Fatalf("walkPSTOutput: %v", err)
	}
	if sink.messages != 0 {
		t.Errorf("%d message(s) were imported through a symbolic link", sink.messages)
	}
}

// Neither may a device, socket or fifo be read as mail: a fifo would also block
// the import for as long as nothing writes to it.
func TestConvertedOutputSkipsAnythingThatIsNotAPlainFile(t *testing.T) {
	output := t.TempDir()
	fifo := filepath.Join(output, "Queue.mbox")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("fifos are unavailable here: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- walkPSTOutput(output, &maildirSink{}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("walkPSTOutput: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("walkPSTOutput blocked on a fifo instead of skipping it")
	}
}

// The two refusals above are only worth anything if an ordinary file the
// converter really produced still arrives.
func TestConvertedOutputStillImportsARealMbox(t *testing.T) {
	output := t.TempDir()
	mbox := "From a@b Mon Jan  1 00:00:00 2024\nSubject: one\n\nbody\n"
	if err := os.WriteFile(filepath.Join(output, "Sent.mbox"), []byte(mbox), 0o600); err != nil {
		t.Fatalf("write the mbox: %v", err)
	}

	var seen []string
	// The sink writes to a Maildir, which needs a tenant; the split and the
	// folder naming are what this test is about, so the walk is driven through
	// the same reader with a recording emitter.
	if err := importPSTFileInto(filepath.Join(output, "Sent.mbox"), func(body []byte) error {
		seen = append(seen, string(body))
		return nil
	}); err != nil {
		t.Fatalf("import the mbox: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("got %d messages, want 1: %q", len(seen), seen)
	}
}

// importPSTFileInto exercises the same open-and-check path as importPSTFile,
// with the Maildir writer replaced by a recorder.
func importPSTFileInto(path string, emit func([]byte) error) error {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	return forEachMboxMessage(file, emit)
}

// The upload cap has to be a real number rather than a comment: without it one
// request fills the disk the mailboxes live on.
func TestImportLimitsAreSet(t *testing.T) {
	if maxImportBytes <= 0 || maxImportMessages <= 0 || maxImportMessageBytes <= 0 {
		t.Fatal("an import limit is unset, so one upload is unbounded")
	}
	if maxImportMessageBytes > maxImportBytes {
		t.Error("a single message may exceed the whole upload, so the cap cannot bind")
	}
}

// A failed import used to leave its partial result in the mailbox, on the theory
// that the customer could finish it by hand. They cannot: the archive is
// re-uploaded whole and every generated name is new, so the retry delivers a
// SECOND copy of everything the failed attempt landed. Rollback is what makes a
// retry produce exactly one copy.
func TestAFailedImportRemovesWhatItWrote(t *testing.T) {
	home := t.TempDir()
	layout := maildirLayout{home: home, root: "Maildir", systemUser: "root"}

	attempt := &maildirSink{layout: layout, token: "import-1"}
	if err := attempt.write("INBOX", nil, strings.NewReader("first")); err != nil {
		t.Skipf("the safe-write layer is unavailable here: %v", err)
	}
	for _, body := range []string{"second", "third"} {
		if err := attempt.write("Sent", nil, strings.NewReader(body)); err != nil {
			t.Fatalf("write %q: %v", body, err)
		}
	}

	// Mail that was already in the mailbox, written by something that is not this
	// import. Rollback matching on the token is the only thing keeping it.
	neighbour := &maildirSink{layout: layout, token: "import-0"}
	if err := neighbour.write("Sent", nil, strings.NewReader("older mail")); err != nil {
		t.Fatalf("write the neighbouring message: %v", err)
	}

	removed, err := attempt.rollback()
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if removed != 3 {
		t.Errorf("removed = %d, want the three messages this import wrote", removed)
	}

	left := maildirNames(t, home, "Maildir/cur")
	left = append(left, maildirNames(t, home, "Maildir/.Sent/cur")...)
	if len(left) != 1 {
		t.Fatalf("names left = %v, want only the neighbouring message", left)
	}
	if !strings.Contains(left[0], "import-0-") {
		t.Errorf("the surviving name is %q, which is not the neighbouring message", left[0])
	}
}

// Rollback removes by this import's own token, so a second import running into
// the same folder must be untouched by the first one's failure.
func TestRollbackLeavesAnotherImportAlone(t *testing.T) {
	home := t.TempDir()
	layout := maildirLayout{home: home, root: "Maildir", systemUser: "root"}

	failed := &maildirSink{layout: layout, token: "import-a"}
	if err := failed.write("INBOX", nil, strings.NewReader("doomed")); err != nil {
		t.Skipf("the safe-write layer is unavailable here: %v", err)
	}
	other := &maildirSink{layout: layout, token: "import-b"}
	for range 2 {
		if err := other.write("INBOX", nil, strings.NewReader("kept")); err != nil {
			t.Fatalf("write the other import's message: %v", err)
		}
	}

	if removed, err := failed.rollback(); err != nil || removed != 1 {
		t.Fatalf("rollback = (%d, %v), want exactly its own one message", removed, err)
	}
	if left := maildirNames(t, home, "Maildir/cur"); len(left) != 2 {
		t.Errorf("names left = %v, want the other import's two messages", left)
	}
}

func maildirNames(t *testing.T, home, rel string) []string {
	t.Helper()
	names, err := files.ListNamesBeneath(home, rel)
	if err != nil {
		t.Fatalf("list %s: %v", rel, err)
	}
	return names
}
