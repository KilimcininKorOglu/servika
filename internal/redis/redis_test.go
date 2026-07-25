package redis

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenPass(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{36}$`)
	a, b := genPass(), genPass()
	if !re.MatchString(a) {
		t.Fatalf("genPass() = %q, want 36 lowercase hex chars", a)
	}
	if a == b {
		t.Fatalf("genPass() returned identical values %q", a)
	}
}

func TestSystemUserPattern(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "c_acme", want: true},
		{in: "abc_123", want: true},
		{in: "C_Acme", want: false},
		{in: "", want: false},
		{in: "has-dash", want: false},
		{in: "a.b", want: false},
	}
	for _, test := range tests {
		if got := systemUserPattern.MatchString(test.in); got != test.want {
			t.Fatalf("systemUserPattern.MatchString(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestWPSnippet(t *testing.T) {
	got := wpSnippet("c_acme", "secret")
	for _, want := range []string{
		"define( 'WP_REDIS_PASSWORD', array( 'c_acme', 'secret' ) );",
		"define( 'WP_REDIS_PREFIX', 'c_acme:' );",
		"define( 'WP_REDIS_HOST', '127.0.0.1' );",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("wpSnippet() missing %q in:\n%s", want, got)
		}
	}
}
