package mail

import (
	"strings"
	"testing"
)

func TestSieveEscaping(t *testing.T) {
	if got := sieveQuote("a\"b\\c"); got != `"a\"b\\c"` {
		t.Fatalf("sieveQuote = %s", got)
	}
	if got := sieveMultiline("first\n.dot\r\nlast"); got != "first\n..dot\nlast" {
		t.Fatalf("sieveMultiline = %q", got)
	}
}

// A Sieve quoted-string has no newline escape — emitting `\n` resolves to the
// letter "n" and would silently corrupt the match value. It must fold to a space.
func TestSieveQuoteNewline(t *testing.T) {
	got := sieveQuote("one\r\ntwo\nthree\rfour")
	if want := `"one two three four"`; got != want {
		t.Fatalf("sieveQuote = %s, want %s", got, want)
	}
	if strings.Contains(got, `\n`) {
		t.Fatalf("output kept an invalid \\n escape: %s", got)
	}
}

func TestValidateFilter(t *testing.T) {
	valid := []MailFilter{
		{Name: "subject", MatchField: "subject", MatchValue: "invoice", ActionType: "move", ActionValue: "Invoices"},
		{Name: "redirect", MatchField: "from", MatchValue: "@company.com", ActionType: "redirect", ActionValue: "archive@example.com"},
		{Name: "discard", MatchField: "to", MatchValue: "old@", ActionType: "discard"},
	}
	for _, filter := range valid {
		if err := validateFilter(filter); err != nil {
			t.Errorf("valid filter rejected: %v", err)
		}
	}
	invalid := []MailFilter{
		{Name: "", MatchField: "subject", MatchValue: "x", ActionType: "discard"},
		{Name: "x", MatchField: "body", MatchValue: "x", ActionType: "discard"},
		{Name: "x", MatchField: "from", MatchValue: "x", ActionType: "move", ActionValue: "../escape"},
		{Name: "x", MatchField: "from", MatchValue: "x", ActionType: "redirect", ActionValue: "invalid"},
	}
	for _, filter := range invalid {
		if err := validateFilter(filter); err == nil {
			t.Errorf("invalid filter accepted: %+v", filter)
		}
	}
}
