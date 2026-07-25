package github

import (
	"regexp"
	"testing"
)

func TestPatErrorMessage(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   []byte
		want   string
	}{
		{name: "401 fixed", status: 401, body: nil, want: "The token is invalid or lacks permission (401)"},
		{name: "403 fixed", status: 403, body: nil, want: "Rate limit or permission error (403)"},
		{name: "json message", status: 404, body: []byte(`{"message":"Not Found"}`), want: "GitHub: Not Found (HTTP 404)"},
		{name: "non-json body", status: 500, body: []byte("not json"), want: "GitHub HTTP 500"},
		{name: "empty json object", status: 422, body: []byte("{}"), want: "GitHub HTTP 422"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := patErrorMessage(test.status, test.body); got != test.want {
				t.Fatalf("patErrorMessage(%d, %q) = %q, want %q", test.status, test.body, got, test.want)
			}
		})
	}
}

func TestRandomHex(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]*$`)
	if got := randomHex(20); len(got) != 40 || !re.MatchString(got) {
		t.Fatalf("randomHex(20) = %q, want 40 hex chars", got)
	}
	if got := randomHex(0); got != "" {
		t.Fatalf("randomHex(0) = %q, want empty", got)
	}
	a, b := randomHex(16), randomHex(16)
	if a == b {
		t.Fatalf("randomHex(16) returned identical values %q", a)
	}
}
