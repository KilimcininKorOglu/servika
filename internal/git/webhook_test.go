package git

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// githubSign produces the GitHub-style signature header value for a body.
func githubSign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestValidGitHubSignature(t *testing.T) {
	secret := "webhook-secret-0123456789abcdef"
	body := []byte(`{"ref":"refs/heads/main"}`)
	valid := githubSign(secret, body)

	tests := []struct {
		name   string
		secret string
		body   []byte
		sig    string
		want   bool
	}{
		{name: "valid signature", secret: secret, body: body, sig: valid, want: true},
		{name: "wrong secret", secret: "different-secret-000000000000000", body: body, sig: valid, want: false},
		{name: "tampered body", secret: secret, body: []byte(`{"ref":"refs/heads/evil"}`), sig: valid, want: false},
		{name: "empty signature", secret: secret, body: body, sig: "", want: false},
		{name: "missing prefix", secret: secret, body: body, sig: hex.EncodeToString([]byte("x")), want: false},
		{name: "garbage signature", secret: secret, body: body, sig: "sha256=not-hex", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validGitHubSignature(test.secret, test.body, test.sig); got != test.want {
				t.Fatalf("validGitHubSignature(%q) = %t, want %t", test.sig, got, test.want)
			}
		})
	}
}
