package mail

import "testing"

func TestNormalizeDestination(t *testing.T) {
	t.Parallel()

	destination, message := normalizeDestination(" Target@Example.com, other@example.net, target@example.com ")
	if message != "" {
		t.Fatalf("normalizeDestination returned message %q", message)
	}
	if destination != "target@example.com,other@example.net" {
		t.Fatalf("normalizeDestination returned %q", destination)
	}
}

func TestNormalizeDestinationRejectsInvalidAddress(t *testing.T) {
	t.Parallel()

	_, message := normalizeDestination("not-an-address")
	if message != "invalid destination email address" {
		t.Fatalf("normalizeDestination returned message %q", message)
	}
}

func TestNormalizeDestinationRequiresAtLeastOneAddress(t *testing.T) {
	t.Parallel()

	_, message := normalizeDestination(" , ")
	if message != "enter at least one destination email address" {
		t.Fatalf("normalizeDestination returned message %q", message)
	}
}
