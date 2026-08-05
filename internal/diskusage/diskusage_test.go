package diskusage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// reset clears the package state so each test starts from an empty cache and
// restores the real seams afterwards.
func reset(t *testing.T) {
	t.Helper()
	mu.Lock()
	cache = map[string]entry{}
	inflight = map[string]*call{}
	mu.Unlock()
	realNow, realMeasure := now, measureFn
	t.Cleanup(func() {
		mu.Lock()
		cache = map[string]entry{}
		inflight = map[string]*call{}
		now, measureFn = realNow, realMeasure
		mu.Unlock()
	})
}

// Without the cache a customer holding down refresh started a full-tree du per
// request, as root and outside their own cgroup I/O limit.
func TestBytesReusesAMeasurementWithinTheTTL(t *testing.T) {
	reset(t)
	var runs atomic.Int64
	measureFn = func(string) result {
		runs.Add(1)
		return result{bytes: 4096}
	}

	for range 5 {
		size, err := Bytes(context.Background(), "/home/c_test")
		if err != nil {
			t.Fatalf("Bytes() = %v, want nil", err)
		}
		if size != 4096 {
			t.Fatalf("Bytes() = %d, want 4096", size)
		}
	}
	if got := runs.Load(); got != 1 {
		t.Errorf("du ran %d times for 5 requests, want 1", got)
	}
}

// A stale size is worse than a slightly expensive one, so the cache must expire.
func TestBytesRemeasuresAfterTheTTL(t *testing.T) {
	reset(t)
	var runs atomic.Int64
	measureFn = func(string) result {
		runs.Add(1)
		return result{bytes: 1}
	}
	base := time.Now()
	now = func() time.Time { return base }

	if _, err := Bytes(context.Background(), "/home/c_test"); err != nil {
		t.Fatalf("Bytes() = %v, want nil", err)
	}
	now = func() time.Time { return base.Add(cacheTTL + time.Second) }
	if _, err := Bytes(context.Background(), "/home/c_test"); err != nil {
		t.Fatalf("Bytes() = %v, want nil", err)
	}
	if got := runs.Load(); got != 2 {
		t.Errorf("du ran %d times across the TTL boundary, want 2", got)
	}
}

// The cache alone does not help until the first result lands. Concurrent
// requests arriving before that must share one run, or a customer can still
// start an unbounded number of scans by firing requests faster than du finishes.
func TestBytesCoalescesConcurrentCallers(t *testing.T) {
	reset(t)
	var runs atomic.Int64
	release := make(chan struct{})
	measureFn = func(string) result {
		runs.Add(1)
		<-release
		return result{bytes: 99}
	}

	var wg sync.WaitGroup
	sizes := make([]int64, 8)
	for i := range sizes {
		wg.Go(func() {
			size, err := Bytes(context.Background(), "/home/c_test")
			if err != nil {
				t.Errorf("Bytes() = %v, want nil", err)
				return
			}
			sizes[i] = size
		})
	}
	// Let every caller reach the coalescing point before the measurement returns.
	waitForInflight(t, "/home/c_test")
	close(release)
	wg.Wait()

	if got := runs.Load(); got != 1 {
		t.Errorf("du ran %d times for 8 concurrent requests, want 1", got)
	}
	for i, size := range sizes {
		if size != 99 {
			t.Errorf("caller %d got %d, want the shared result 99", i, size)
		}
	}
}

// A client disconnecting must not cancel the run other callers are waiting on,
// and must not leave the cache unpopulated: that would let a customer restart
// the scan indefinitely by aborting each request.
func TestBytesSurvivesACallerGivingUp(t *testing.T) {
	reset(t)
	var runs atomic.Int64
	release := make(chan struct{})
	measureFn = func(string) result {
		runs.Add(1)
		<-release
		return result{bytes: 7}
	}

	ctx, cancel := context.WithCancel(context.Background())
	var abandoned sync.WaitGroup
	abandoned.Go(func() {
		if _, err := Bytes(ctx, "/home/c_test"); !errors.Is(err, context.Canceled) {
			t.Errorf("the abandoning caller got %v, want context.Canceled", err)
		}
	})
	waitForInflight(t, "/home/c_test")
	cancel()
	abandoned.Wait()
	close(release)

	// The abandoned run still completes, so the next caller is served from cache.
	size, err := Bytes(context.Background(), "/home/c_test")
	if err != nil {
		t.Fatalf("Bytes() = %v, want nil", err)
	}
	if size != 7 {
		t.Errorf("Bytes() = %d, want the completed measurement 7", size)
	}
	if got := runs.Load(); got != 1 {
		t.Errorf("du ran %d times, want 1: the abandoned run was restarted", got)
	}
}

// A failure must be reported, never reported as a zero size: zero reads as an
// empty home and would show a tenant far under a quota they may be over.
func TestBytesReportsAMeasurementFailure(t *testing.T) {
	reset(t)
	want := errors.New("du exploded")
	measureFn = func(string) result { return result{err: want} }

	size, err := Bytes(context.Background(), "/home/c_test")
	if !errors.Is(err, want) {
		t.Fatalf("Bytes() error = %v, want %v", err, want)
	}
	if size != 0 {
		t.Errorf("Bytes() = %d, want 0 alongside the error", size)
	}
}

// measure shells out to GNU du; -sb does not exist on the BSD du shipped with
// macOS, and production is Linux only.
func TestMeasureReportsTheTreeSize(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("du -sb is GNU-only; production is Linux")
	}
	dir := t.TempDir()
	payload := make([]byte, 8192)
	if err := os.WriteFile(filepath.Join(dir, "file"), payload, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := measure(dir)
	if got.err != nil {
		t.Fatalf("measure() = %v, want nil", got.err)
	}
	if got.bytes < int64(len(payload)) {
		t.Errorf("measure() = %d, want at least the %d bytes written", got.bytes, len(payload))
	}
}

func TestMeasureReportsAMissingDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("du -sb is GNU-only; production is Linux")
	}
	got := measure(filepath.Join(t.TempDir(), "absent"))
	if got.err == nil {
		t.Error("measure() = nil error for a missing directory, want a failure")
	}
}

// waitForInflight blocks until a measurement for path is registered, so a test
// can be sure every caller reached the coalescing point.
func waitForInflight(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		_, running := inflight[path]
		mu.Unlock()
		if running {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no measurement was registered for %s", path)
}
