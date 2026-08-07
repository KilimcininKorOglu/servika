package mail

import (
	"context"
	"net"
	"testing"
	"time"
)

// fakeIMAP answers one connection with a greeting and then stays quiet. It is
// enough to prove a connection was established; the protocol itself belongs to
// the library's own tests.
func fakeIMAP(t *testing.T) (host string, port int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("* OK [CAPABILITY IMAP4rev2] ready\r\n"))
			// Held open: closing here would race the client's greeting read.
			time.AfterFunc(5*time.Second, func() { _ = conn.Close() })
		}
	}()

	address := listener.Addr().(*net.TCPAddr)
	return "127.0.0.1", address.Port
}

// The remote server is named in a form, so a customer who points the panel at
// its own loopback interface must be refused before any bytes are exchanged.
func TestMigrationDialRefusesAnInternalAddress(t *testing.T) {
	host, port := fakeIMAP(t)

	client, err := dialIMAP(context.Background(), host, port, SecurityPlain)
	if err == nil {
		_ = client.Close()
		t.Fatal("a loopback address was dialled")
	}
	if code := reasonFor(err); code != ReasonBlockedHost {
		t.Errorf("reason = %q, want %q (error: %v)", code, ReasonBlockedHost, err)
	}
}

// The other direction: without the guard the very same address connects, so the
// test above pins netguard rather than a listener that was never reachable.
func TestMigrationDialReachesTheSameAddressWhenTheOperatorOptedOut(t *testing.T) {
	t.Setenv("SERVIKA_ALLOW_PRIVATE_TARGETS", "1")
	host, port := fakeIMAP(t)

	client, err := dialIMAP(context.Background(), host, port, SecurityPlain)
	if err != nil {
		t.Fatalf("dialIMAP: %v", err)
	}
	_ = client.Close()
}

// An unrecognised security value must be refused rather than quietly treated as
// one of the encrypted modes, or a typo would send the password in the clear.
func TestMigrationDialRefusesAnUnknownSecurity(t *testing.T) {
	_, err := dialIMAP(context.Background(), "imap.example.test", 993, "tls")
	if err == nil {
		t.Fatal("an unknown security value was accepted")
	}
	if code := reasonFor(err); code != ReasonBadSecurity {
		t.Errorf("reason = %q, want %q", code, ReasonBadSecurity)
	}
}

// A host that never answers must give up on its own rather than hold a
// discovery sweep open for as long as the caller is willing to wait.
func TestMigrationDialGivesUpOnACancelledContext(t *testing.T) {
	t.Setenv("SERVIKA_ALLOW_PRIVATE_TARGETS", "1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A port nothing listens on: the dial would fail anyway, but the cancelled
	// context has to be what stops it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	started := time.Now()
	if _, err := dialIMAP(ctx, "127.0.0.1", port, SecurityPlain); err == nil {
		t.Fatal("a cancelled context still produced a connection")
	}
	if elapsed := time.Since(started); elapsed > imapDialTimeout {
		t.Errorf("the dial waited %v, past its own %v ceiling", elapsed, imapDialTimeout)
	}
}
