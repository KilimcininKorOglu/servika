package plugin

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortSocketPath returns a path a Unix socket can actually be bound to.
//
// t.TempDir() produces a path over the 104-byte sun_path limit on macOS, where
// bind fails with "invalid argument" rather than anything that names the cause.
func shortSocketPath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("/tmp", fmt.Sprintf("servika-%s-%d.sock", name, os.Getpid()))
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

// The window this exists for: the socket is gone for a moment while the plugin
// restarts, and every request landing in it used to answer an error.
func TestDialWaitsOutAPluginRestart(t *testing.T) {
	socket := shortSocketPath(t, "restart")

	// The listener appears after the first attempts have already failed.
	ready := make(chan struct{})
	go func() {
		time.Sleep(2 * pluginDialDelay)
		listener, err := net.Listen("unix", socket)
		if err != nil {
			close(ready)
			return
		}
		close(ready)
		defer func() { _ = listener.Close() }()
		if conn, err := listener.Accept(); err == nil {
			_ = conn.Close()
		}
		time.Sleep(time.Second)
	}()

	conn, err := dialPluginSocket(context.Background(), socket)
	<-ready
	if err != nil {
		t.Fatalf("dial gave up during the restart window: %v", err)
	}
	_ = conn.Close()
}

// A socket that never comes back must still fail, and within the budget: an
// unreachable plugin cannot hold a panel request open.
func TestDialGivesUpOnASocketThatNeverReturns(t *testing.T) {
	socket := shortSocketPath(t, "absent")
	if _, err := os.Stat(socket); err == nil {
		t.Fatal("the socket exists, so the test proves nothing")
	}

	started := time.Now()
	if _, err := dialPluginSocket(context.Background(), socket); err == nil {
		t.Fatal("a socket that does not exist was reported as connected")
	}
	if spent := time.Since(started); spent > 2*time.Second {
		t.Errorf("the dial blocked for %v, which is too long on a request path", spent)
	}
}

// A cancelled request must not keep retrying: the caller has gone.
func TestDialStopsWhenTheRequestIsCancelled(t *testing.T) {
	socket := shortSocketPath(t, "cancelled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	if _, err := dialPluginSocket(ctx, socket); err == nil {
		t.Fatal("a cancelled dial reported success")
	}
	if spent := time.Since(started); spent > pluginDialDelay {
		t.Errorf("the dial kept retrying for %v after the request was cancelled", spent)
	}
}
