package provisioner

import (
	"strings"
	"testing"
)

// The reason has to reach the user. Without it the panel reports a certificate
// was installed while the browser reports the site is not secure, and nothing on
// screen connects the two.
func TestSummarizeSSLReasonPicksTheValidationFailure(t *testing.T) {
	// A real acme.sh transcript buries the cause under progress output.
	const transcript = `[Mon 04 Aug 2026 11:02:03 AM UTC] Using CA: https://acme-v02.api.letsencrypt.org/directory
[Mon 04 Aug 2026 11:02:03 AM UTC] Creating domain key
[Mon 04 Aug 2026 11:02:04 AM UTC] Verifying: www.example.com
[Mon 04 Aug 2026 11:02:06 AM UTC] www.example.com: Invalid status. DNS problem: NXDOMAIN looking up A for www.example.com
[Mon 04 Aug 2026 11:02:06 AM UTC] Please check log file for more details.`

	got := summarizeSSLReason(transcript)
	if !strings.Contains(got, "NXDOMAIN looking up A for www.example.com") {
		t.Errorf("summarizeSSLReason() = %q, which does not name the cause", got)
	}
	if strings.Contains(got, "Creating domain key") {
		t.Errorf("summarizeSSLReason() returned progress output: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("summarizeSSLReason() returned several lines: %q", got)
	}
}

// A rate-limit refusal is the other cause a user must be able to act on, and it
// is worded differently from a validation failure.
func TestSummarizeSSLReasonPicksARateLimit(t *testing.T) {
	const transcript = `[Mon] Getting webroot for domain='example.com'
[Mon] Error creating new order :: too many certificates (5) already issued for this exact set of domains in the last 168 hours
[Mon] Please add '--debug' or '--log' to check more details.`

	if got := summarizeSSLReason(transcript); !strings.Contains(got, "too many certificates") {
		t.Errorf("summarizeSSLReason() = %q, which does not name the rate limit", got)
	}
}

// acme.sh output is unbounded, and this string ends up in an API response.
func TestSummarizeSSLReasonIsBounded(t *testing.T) {
	got := summarizeSSLReason("DNS problem: " + strings.Repeat("x", 5000))
	if len(got) > 320 {
		t.Errorf("summarizeSSLReason() returned %d characters; the transcript sizes the response", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a truncated reason does not say it was truncated: %q", got)
	}
}

// Falling back with nothing to say is worse than saying the generic thing, but
// an empty transcript must not produce a stray blank warning either.
func TestSummarizeSSLReasonHandlesAnEmptyTranscript(t *testing.T) {
	for _, transcript := range []string{"", "\n\n   \n"} {
		if got := summarizeSSLReason(transcript); got != "" {
			t.Errorf("summarizeSSLReason(%q) = %q, want empty", transcript, got)
		}
	}
}

// An apex that does not resolve cannot pass http-01, and calling acme.sh anyway
// spends one of the five failed validations Let's Encrypt allows per hostname
// per hour. .invalid is reserved by RFC 2606 and must never resolve.
func TestDomainResolvesRefusesAReservedInvalidName(t *testing.T) {
	if domainResolves("this-host-does-not-exist.invalid") {
		t.Error("domainResolves() accepted a reserved .invalid name")
	}
}
