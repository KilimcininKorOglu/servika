package phpversion

import (
	"sync"
	"sync/atomic"
	"testing"
)

// resetAvailabilityCache clears the cache for deterministic test runs.
func resetAvailabilityCache() {
	availabilityMu.Lock()
	availabilityCache = map[string]bool{}
	availabilityMu.Unlock()
}

// TestPackageAvailableCacheOnly verifies the request path (packageAvailable / AllVersions)
// NEVER calls dnf, only reads the cache populated by the background sweeper, and that
// concurrent access has no races. Run with `go test -race`.
func TestPackageAvailableCacheOnly(t *testing.T) {
	sweeperOnce.Do(func() {})
	resetAvailabilityCache()

	var probeCalls int64
	old := dnfProbe
	dnfProbe = func(pkg string) (available bool, checked bool) { // fake probe that never calls dnf
		atomic.AddInt64(&probeCalls, 1)
		return pkg == "php82-php-fpm", true // only php82 installable; all give DEFINITE answer
	}
	defer func() { dnfProbe = old }()

	// Populate synchronously.
	sweepOnce()

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

	// Concurrent reads: cache-only, NO dnf calls, NO races.
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
	dnfProbe = func(pkg string) (bool, bool) {
		t.Fatalf("request path called dnf: %s", pkg)
		return false, false
	}
	defer func() { dnfProbe = old }()

	resetAvailabilityCache()

	// Empty cache → false, and dnf is NEVER called (dnfProbe would fail the test).
	if packageAvailable(VersionMetadata{Version: "8.5", Code: "85", Resource: "remi"}) {
		t.Fatal("empty cache should default to false")
	}
}

// TestSweepTransientFailurePreservesLastKnownGood: (a) transient-fail true→true RETAINED.
// Round 1 writes CONFIRMED true; round 2 COULD NOT ASK dnf (checked=false) → previous true
// MUST NOT be flipped to false. This is the exact regression test for the original
// false-negative bug: a transient dnf lock used to atomically wipe the entire cache to false.
func TestSweepTransientFailurePreservesLastKnownGood(t *testing.T) {
	sweeperOnce.Do(func() {})
	resetAvailabilityCache()
	old := dnfProbe
	defer func() { dnfProbe = old }()

	// Round 1: every package DEFINITELY available (checked=true, available=true).
	dnfProbe = func(pkg string) (bool, bool) { return true, true }
	sweepOnce()
	if !packageAvailable(VersionMetadata{Version: "8.2", Code: "82", Resource: "remi"}) {
		t.Fatal("round 1: php82 cache=true expected")
	}
	if !packageAvailable(VersionMetadata{Version: "8.4", Code: "84", Resource: "remi"}) {
		t.Fatal("round 1: php84 cache=true expected")
	}

	// Round 2: dnf transiently COULD NOT BE ASKED (checked=false) for ALL packages.
	// Expectation: previous true values are PRESERVED.
	dnfProbe = func(pkg string) (bool, bool) { return false, false }
	sweepOnce()
	if !packageAvailable(VersionMetadata{Version: "8.2", Code: "82", Resource: "remi"}) {
		t.Fatal("transient dnf failure (checked=false) MUST NOT flip last-known-good true to false")
	}
	if !packageAvailable(VersionMetadata{Version: "8.4", Code: "84", Resource: "remi"}) {
		t.Fatal("transient dnf failure must also preserve php84 true")
	}
}

// TestSweepTimeoutIsNotUnavailable: (b) timeout ≠ unavailable.
// For the same package: checked=false (timeout) preserves previous value; checked=true +
// available=false (dnf DEFINITE 'No match') flips to false. Proves the two cases are DISTINCT.
func TestSweepTimeoutIsNotUnavailable(t *testing.T) {
	sweeperOnce.Do(func() {})
	resetAvailabilityCache()
	old := dnfProbe
	defer func() { dnfProbe = old }()

	// Seed: php81 DEFINITELY available.
	dnfProbe = func(pkg string) (bool, bool) { return true, true }
	sweepOnce()
	if !packageAvailable(VersionMetadata{Version: "8.1", Code: "81", Resource: "remi"}) {
		t.Fatal("after seed php81 should be true")
	}

	// Timeout round (checked=false): is NOT 'unavailable' → true retained.
	dnfProbe = func(pkg string) (bool, bool) { return false, false }
	sweepOnce()
	if !packageAvailable(VersionMetadata{Version: "8.1", Code: "81", Resource: "remi"}) {
		t.Fatal("timeout (checked=false) is NOT unavailable; previous true must be retained")
	}

	// Confirmed-unavailable round (checked=true, available=false): NOW it should be false.
	dnfProbe = func(pkg string) (bool, bool) { return false, true }
	sweepOnce()
	if packageAvailable(VersionMetadata{Version: "8.1", Code: "81", Resource: "remi"}) {
		t.Fatal("dnf DEFINITE 'No match' (checked=true) should yield false")
	}
}

// TestSweepConfirmedUnavailableIsFalse: (c) confirmed-unavailable → still false.
// Starting from empty cache, when dnf DEFINITELY returns 'No match', cache should EXPLICITLY
// hold false.
func TestSweepConfirmedUnavailableIsFalse(t *testing.T) {
	sweeperOnce.Do(func() {})
	resetAvailabilityCache()
	old := dnfProbe
	defer func() { dnfProbe = old }()

	dnfProbe = func(pkg string) (bool, bool) { return false, true } // dnf clear: package absent
	sweepOnce()

	if packageAvailable(VersionMetadata{Version: "8.4", Code: "84", Resource: "remi"}) {
		t.Fatal("confirmed-unavailable → packageAvailable should return false")
	}
	// Cache should hold EXPLICIT false, not be absent.
	availabilityMu.Lock()
	v, ok := availabilityCache["php84-php-fpm"]
	availabilityMu.Unlock()
	if !ok {
		t.Fatal("php84 should be present in cache (confirmed → written)")
	}
	if v {
		t.Fatal("php84 cache value should be false")
	}
}

// TestAvailabilityVerifyThreeState verifies the install-gate LIVE authoritative probe
// (dnfLiveProbe) correctly distinguishes the three states — in particular 'could not ask'
// (checked=false) NEVER implies EOL/unavailable (false-negative prevention). AppStream
// is never probed live.
func TestAvailabilityVerifyThreeState(t *testing.T) {
	old := dnfLiveProbe
	defer func() { dnfLiveProbe = old }()

	remi81 := VersionMetadata{Version: "8.1", Code: "81", Resource: "remi"}

	// 1) confirmed-unavailable: checked=true & available=false → safe to say EOL.
	dnfLiveProbe = func(pkg string) (bool, bool) { return false, true }
	if a, c := availabilityVerify(remi81); !c || a {
		t.Fatalf("confirmed-unavailable expected (checked=true, available=false): a=%v c=%v", a, c)
	}

	// 2) could not ask: checked=false → NEVER say EOL (false-negative prevention).
	dnfLiveProbe = func(pkg string) (bool, bool) { return false, false }
	if _, c := availabilityVerify(remi81); c {
		t.Fatal("could not ask dnf → checked=false expected (must not claim EOL)")
	}

	// 3) available: checked=true & available=true.
	dnfLiveProbe = func(pkg string) (bool, bool) { return true, true }
	if a, c := availabilityVerify(remi81); !a || !c {
		t.Fatalf("available expected: a=%v c=%v", a, c)
	}

	// 4) appstream: always (true, true) WITHOUT calling the live probe.
	dnfLiveProbe = func(pkg string) (bool, bool) {
		t.Fatal("appstream must not call live dnf probe")
		return false, false
	}
	if a, c := availabilityVerify(VersionMetadata{Version: "8.3", Code: "", Resource: "appstream"}); !a || !c {
		t.Fatalf("appstream should be (true,true): a=%v c=%v", a, c)
	}
}
