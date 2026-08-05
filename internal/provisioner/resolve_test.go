package provisioner

import (
	"errors"
	"testing"
	"time"
)

// stubResolver installs a resolver the test drives, and removes the retry delay
// so a failing case does not spend seconds sleeping.
func stubResolver(t *testing.T, answer func(host string) ([]string, error)) {
	t.Helper()
	previousResolver, previousDelay := resolveHost, resolveRetryDelay
	resolveHost = answer
	resolveRetryDelay = 0
	t.Cleanup(func() {
		resolveHost = previousResolver
		resolveRetryDelay = previousDelay
	})
}

// The case this exists for: the name is configured correctly, one lookup fails,
// and without a retry www silently drops out of the certificate.
func TestLookupRetriesUntilTheResolverAnswers(t *testing.T) {
	calls := 0
	stubResolver(t, func(string) ([]string, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("server misbehaving")
		}
		return []string{"203.0.113.10"}, nil
	})

	got := lookupHostRetrying("example.com")
	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("addresses = %v, want the address the third attempt returned", got)
	}
	if calls != 3 {
		t.Errorf("resolver called %d times, want 3", calls)
	}
}

// A resolver that stays down is still a "no": the caller must not be handed an
// empty answer that reads as success.
func TestLookupGivesUpAfterTheAttempts(t *testing.T) {
	calls := 0
	stubResolver(t, func(string) ([]string, error) {
		calls++
		return nil, errors.New("server misbehaving")
	})

	if got := lookupHostRetrying("example.com"); got != nil {
		t.Fatalf("addresses = %v, want nil", got)
	}
	if calls != resolveAttempts {
		t.Errorf("resolver called %d times, want %d", calls, resolveAttempts)
	}
}

// An answer is not retried, however unwelcome it is. www pointing at another
// server is a fact about the configuration, and asking again only delays the
// decision.
func TestWWWPointingElsewhereIsNotRetried(t *testing.T) {
	calls := 0
	stubResolver(t, func(host string) ([]string, error) {
		calls++
		if host == "example.com" {
			return []string{"203.0.113.10"}, nil
		}
		return []string{"198.51.100.7"}, nil
	})

	if wwwSANEligible("example.com") {
		t.Error("www resolving to a different server was accepted into the SAN")
	}
	if calls != 2 {
		t.Errorf("resolver called %d times, want 2 (one answer each)", calls)
	}
}

// The retry must not turn a name that genuinely does not exist into an eligible
// one, and matching addresses must still pass.
func TestWWWEligibilityStillFollowsTheAddresses(t *testing.T) {
	stubResolver(t, func(host string) ([]string, error) {
		if host == "example.com" {
			return []string{"203.0.113.10"}, nil
		}
		return nil, errors.New("no such host")
	})
	if wwwSANEligible("example.com") {
		t.Error("www that never resolves was accepted into the SAN")
	}

	stubResolver(t, func(string) ([]string, error) { return []string{"203.0.113.10"}, nil })
	if !wwwSANEligible("example.com") {
		t.Error("www sharing the apex address was refused")
	}
}

// A failing resolver must not make issuance wait on the request path longer than
// the retries themselves.
func TestLookupRetryDelayIsBounded(t *testing.T) {
	if resolveRetryDelay*time.Duration(resolveAttempts-1) > 5*time.Second {
		t.Errorf("retries can block for %v, which is too long on a request path",
			resolveRetryDelay*time.Duration(resolveAttempts-1))
	}
}
