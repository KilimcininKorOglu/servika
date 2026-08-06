package domains

import "testing"

// Only the self-signed fail-safe makes a browser warn the visitor. Everything
// else here is a real certificate, and an unknown value is not evidence of a bad
// one: a domain with no certificate carries the empty string, and so does every
// row written before the column existed.
func TestOnlyASelfSignedCertificateIsUntrusted(t *testing.T) {
	for _, tc := range []struct {
		source      string
		wantTrusted bool
		why         string
	}{
		{source: SSLSourceLetsEncrypt, wantTrusted: true, why: "a real CA"},
		{source: SSLSourceImported, wantTrusted: true, why: "a real certificate carried over from a cPanel migration"},
		{source: "", wantTrusted: true, why: "unknown, which is not the same as bad"},
		{source: "buypass", wantTrusted: true, why: "a source added later must not start raising alarms on its own"},
		{source: SSLSourceSelfSigned, wantTrusted: false, why: "the visitor gets a full-page browser warning"},
	} {
		if got := SSLSourceIsTrusted(tc.source); got != tc.wantTrusted {
			t.Errorf("SSLSourceIsTrusted(%q) = %v, want %v (%s)", tc.source, got, tc.wantTrusted, tc.why)
		}
	}
}

// The constants are the contract with a VARCHAR column, so their spelling is
// what every writer and the overview ordering agree on. A typo here would not
// fail to compile anywhere; it would quietly stop matching stored rows.
func TestTheSourceConstantsMatchWhatIsStored(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{SSLSourceLetsEncrypt, "letsencrypt"},
		{SSLSourceSelfSigned, "self-signed"},
		{SSLSourceImported, "imported"},
	} {
		if tc.got != tc.want {
			t.Errorf("constant = %q, want %q", tc.got, tc.want)
		}
	}
}
