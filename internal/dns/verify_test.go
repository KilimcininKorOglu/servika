package dns

import (
	"errors"
	"net"
	"strings"
	"testing"
)

// A published DKIM key must be compared, not merely counted: an old or foreign
// key leaves every signature unverifiable while the record looks present.
// Resolvers split long TXT records into chunks and some providers store the key
// with whitespace in it, so both have to survive extraction.
func TestPublicKeyFromTXTNormalizesChunkedAndSpacedRecords(t *testing.T) {
	const key = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA1234567890"

	cases := map[string]string{
		"plain":   "v=DKIM1; k=rsa; p=" + key,
		"spaced":  "v=DKIM1; k=rsa; p=" + key[:20] + " " + key[20:],
		"tabbed":  "v=DKIM1; k=rsa; p=" + key[:10] + "\t" + key[10:],
		"trailer": "v=DKIM1; k=rsa; p=" + key + ";",
		// Two resolver chunks joined back together.
		"chunked": strings.Join([]string{"v=DKIM1; k=rsa; p=" + key[:30], key[30:]}, ""),
	}
	for name, record := range cases {
		if got := publicKeyFromTXT(record); got != key {
			t.Errorf("%s: publicKeyFromTXT() = %q, want the stored key", name, got)
		}
	}

	if got := publicKeyFromTXT("v=DKIM1; k=rsa"); got != "" {
		t.Errorf("a record with no p= returned %q, want empty", got)
	}
}

// Substring matching would find the "a" inside "ip4:" and report an SPF record
// that authorizes nothing as correct.
func TestHasSPFMechanismMatchesWholeTermsOnly(t *testing.T) {
	if hasSPFMechanism("v=spf1 ip4:192.0.2.10 ~all", "a") {
		t.Error(`the "a" mechanism was found inside "ip4:", so an SPF record authorizing nothing would pass`)
	}
	if hasSPFMechanism("v=spf1 include:mailgun.org ~all", "mx") {
		t.Error(`the "mx" mechanism was found inside another term`)
	}
	for _, record := range []string{
		"v=spf1 a mx ~all",
		"v=spf1 +a ~all",
		"v=spf1 a:mail.example.com ~all",
		"v=spf1 a/24 ~all",
		"v=spf1 -a ~all",
	} {
		if !hasSPFMechanism(record, "a") {
			t.Errorf("hasSPFMechanism(%q, \"a\") = false, want true", record)
		}
	}
}

// A resolver hands back IPv4 addresses in the 4-byte form while net.ParseIP
// produces the 16-byte one, so the comparison has to be net.IP.Equal; comparing
// the raw bytes would report a correctly pointed domain as pointing elsewhere.
func TestContainsIPComparesParsedAddresses(t *testing.T) {
	addresses := []net.IPAddr{
		{IP: net.ParseIP("2001:db8::1")},
		{IP: net.IP{192, 0, 2, 10}}, // 4-byte form, as a resolver returns it
	}
	if !containsIP(addresses, "192.0.2.10") {
		t.Error("the 4-byte form of an address did not match its parsed 16-byte form")
	}
	if !containsIP(addresses, "2001:db8::1") {
		t.Error("an IPv6 address did not match itself")
	}
	if containsIP(addresses, "192.0.2.11") {
		t.Error("a different address matched")
	}
	if containsIP(addresses, "") {
		t.Error("an empty expected address matched")
	}
}

// A resolver that cannot be reached must never be reported as a missing record:
// the two call for opposite actions from the operator.
func TestLookupFailureReasonSeparatesAbsenceFromFailure(t *testing.T) {
	notFound := &net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true}
	if got := lookupFailureReason(notFound, "missing"); got != "missing" {
		t.Errorf("a not-found lookup = %q, want \"missing\"", got)
	}
	timedOut := &net.DNSError{Err: "i/o timeout", Name: "example.com", IsTimeout: true}
	if got := lookupFailureReason(timedOut, "missing"); got != "unreadable" {
		t.Errorf("a timed-out lookup = %q, want \"unreadable\"", got)
	}
	if got := lookupFailureReason(errors.New("dial tcp: connection refused"), "missing"); got != "unreadable" {
		t.Errorf("a transport failure = %q, want \"unreadable\"", got)
	}
}

// The expected value is rendered in the UI next to the found one, so it has to
// stay short without becoming ambiguous.
func TestShortenKeyBoundsLongKeys(t *testing.T) {
	short := "p=abc"
	if got := shortenKey(short); got != short {
		t.Errorf("shortenKey(%q) = %q, want it unchanged", short, got)
	}
	long := strings.Repeat("K", 400)
	got := shortenKey(long)
	if len(got) >= len(long) {
		t.Errorf("shortenKey did not shorten a %d character key", len(long))
	}
	if !strings.HasPrefix(got, long[:12]) || !strings.HasSuffix(got, long[len(long)-8:]) {
		t.Errorf("shortenKey(%q...) = %q, which drops the ends that identify the key", long[:12], got)
	}
}
