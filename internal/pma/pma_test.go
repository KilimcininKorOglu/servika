package pma

import (
	"regexp"
	"testing"
)

func TestRandomHex(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]*$`)

	got, err := randomHex(24)
	if err != nil {
		t.Fatalf("randomHex(24) returned error: %v", err)
	}
	if len(got) != 48 || !re.MatchString(got) {
		t.Fatalf("randomHex(24) = %q, want 48 hex chars", got)
	}

	empty, err := randomHex(0)
	if err != nil || empty != "" {
		t.Fatalf("randomHex(0) = %q, %v; want \"\", nil", empty, err)
	}

	a, _ := randomHex(16)
	b, _ := randomHex(16)
	if a == b {
		t.Fatal("randomHex(16) returned identical values")
	}
}
