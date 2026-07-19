package phpversion

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestPackageAvailableCacheOnly verifies the request path (packageAvailable / AllVersions)
// NEVER calls dnf, only reads the cache populated by the background sweeper, and that
// concurrent access has no races. Run with `go test -race`.
func TestPackageAvailableCacheOnly(t *testing.T) {
	// Drain sweeperOnce so sweepOnce runs synchronously, not via a background goroutine.
	sweeperOnce.Do(func() {})

	var probeCalls int64
	old := dnfProbe
	dnfProbe = func(pkg string) bool { // fake probe that never calls dnf
		atomic.AddInt64(&probeCalls, 1)
		return pkg == "php82-php-fpm" // only php82 is installable
	}
	defer func() { dnfProbe = old }()

	// Populate the cache synchronously — normally the background sweeper does this.
	sweepOnce()

	// Correctness: values are read from the cache.
	if !packageAvailable(VersionMetadata{Version: "8.2", Code: "82", Resource: "remi"}) {
		t.Fatal("php82 should be installable")
	}
	if packageAvailable(VersionMetadata{Version: "8.1", Code: "81", Resource: "remi"}) {
		t.Fatal("php81 should not be installable (cache=false)")
	}
	if !packageAvailable(VersionMetadata{Version: "8.3", Code: "", Resource: "appstream"}) {
		t.Fatal("appstream should always be available")
	}

	base := atomic.LoadInt64(&probeCalls)

	// Concurrent request load: cache-only reads, NO dnf calls, NO races.
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = packageAvailable(VersionMetadata{Version: "8.2", Code: "82", Resource: "remi"})
			_ = packageAvailable(VersionMetadata{Version: "8.4", Code: "84", Resource: "remi"})
			_ = AllVersions()
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&probeCalls); got != base {
		t.Fatalf("request path called dnf: base=%d got=%d (cache-only expected)", base, got)
	}
}

// TestEmptyCacheDoesNotBlock verifies that when the cache is empty (first boot) the
// request path returns a safe default (false) immediately without calling dnf — it does
// not hang.
func TestEmptyCacheDoesNotBlock(t *testing.T) {
	sweeperOnce.Do(func() {})

	old := dnfProbe
	dnfProbe = func(pkg string) bool { t.Fatalf("request path called dnf: %s", pkg); return false }
	defer func() { dnfProbe = old }()

	// Empty the cache (simulates first boot).
	availabilityMu.Lock()
	availabilityCache = map[string]bool{}
	availabilityMu.Unlock()

	// Empty cache → false, and dnf is NEVER called (dnfProbe would fail the test).
	if packageAvailable(VersionMetadata{Version: "8.5", Code: "85", Resource: "remi"}) {
		t.Fatal("empty cache should default to false")
	}
}
