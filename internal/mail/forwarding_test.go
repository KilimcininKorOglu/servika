package mail

import (
	"strings"
	"testing"
)

// The address is written into a Sieve script, so anything that could end the
// statement and start another one has to be refused rather than escaped.
func TestForwardingDestinationsRefuseSieveInjection(t *testing.T) {
	for _, destination := range []string{
		`a@b.test";\ndiscard;#`,
		"a@b.test\nredirect \"attacker@evil.test\";",
		"a@b.test\rredirect \"attacker@evil.test\";",
		`a@b.test\"`,
		"a b@c.test",
		"not-an-address",
		"@no-local.test",
		"",
	} {
		if list, reason := normalizeDestinations([]string{destination}); reason == "" {
			t.Errorf("%q was accepted as %v", destination, list)
		}
	}
}

// The refusals above are only meaningful if ordinary addresses still pass, and
// pass in a usable shape.
func TestForwardingDestinationsAreNormalised(t *testing.T) {
	list, reason := normalizeDestinations([]string{" Info@Example.COM ", "second@example.com,info@example.com"})
	if reason != "" {
		t.Fatalf("ordinary addresses were refused: %s", reason)
	}
	if len(list) != 2 {
		t.Fatalf("got %v, want the two distinct addresses once each", list)
	}
	if list[0] != "info@example.com" {
		t.Errorf("the address was not trimmed and lower-cased: %q", list[0])
	}
}

// A forward that fans out to dozens of addresses is a relay, and the outbound
// limits are counted per mailbox rather than per recipient.
func TestForwardingRefusesAnUnboundedFanOut(t *testing.T) {
	many := make([]string, 0, maxForwardingDestinations+1)
	for i := 0; i <= maxForwardingDestinations; i++ {
		many = append(many, "a"+strings.Repeat("x", i)+"@example.com")
	}
	if _, reason := normalizeDestinations(many); reason != "too_many_destinations" {
		t.Errorf("reason = %q, want too_many_destinations", reason)
	}
	// One fewer must still be accepted, or the bound is off by one.
	if _, reason := normalizeDestinations(many[:maxForwardingDestinations]); reason != "" {
		t.Errorf("the limit itself was refused: %s", reason)
	}
}
