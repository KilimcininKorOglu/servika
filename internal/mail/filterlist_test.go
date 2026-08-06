package mail

import (
	"strings"
	"testing"
)

// A multimap line has no escaping: a value with a space in it does not mean the
// value, it means something else. Everything that is not the ordinary shape is
// therefore refused rather than quoted.
func TestFilterValuesThatWouldRewriteTheMapAreRefused(t *testing.T) {
	cases := map[string]FilterEntry{
		"newline in an address":    {Kind: "block", MatchType: "address", Value: "a@b.test\nc@d.test"},
		"space in a domain":        {Kind: "block", MatchType: "domain", Value: "b.test c.test"},
		"comment marker":           {Kind: "allow", MatchType: "domain", Value: "# b.test"},
		"regexp instead of a name": {Kind: "block", MatchType: "domain", Value: "/.*/"},
		"single label domain":      {Kind: "block", MatchType: "domain", Value: "localhost"},
		"not an address":           {Kind: "allow", MatchType: "address", Value: "someone"},
		"not an ip":                {Kind: "block", MatchType: "ip", Value: "not-an-ip"},
		"unknown kind":             {Kind: "maybe", MatchType: "domain", Value: "b.test"},
		"unknown match type":       {Kind: "block", MatchType: "asn", Value: "1234"},
	}
	for name, entry := range cases {
		if err := normalizeFilterEntry(&entry); err == nil {
			t.Errorf("%s was accepted as %q", name, entry.Value)
		}
	}
}

// The ordinary shapes have to survive, or the validation is only a refusal.
func TestOrdinaryFilterValuesAreAcceptedAndNormalised(t *testing.T) {
	cases := []struct {
		in   FilterEntry
		want string
	}{
		{FilterEntry{Kind: "allow", MatchType: "address", Value: " Sales@Example.COM "}, "sales@example.com"},
		{FilterEntry{Kind: "block", MatchType: "domain", Value: "Example.COM"}, "example.com"},
		{FilterEntry{Kind: "block", MatchType: "ip", Value: "203.0.113.5"}, "203.0.113.5"},
		// The same range typed two ways has to become one entry, or the unique
		// index never fires and the list fills with duplicates.
		{FilterEntry{Kind: "block", MatchType: "ip", Value: "203.0.113.7/24"}, "203.0.113.0/24"},
	}
	for _, tc := range cases {
		entry := tc.in
		if err := normalizeFilterEntry(&entry); err != nil {
			t.Errorf("normalizeFilterEntry(%q): %v", tc.in.Value, err)
			continue
		}
		if entry.Value != tc.want {
			t.Errorf("normalizeFilterEntry(%q) = %q, want %q", tc.in.Value, entry.Value, tc.want)
		}
	}
}

// A note is free text an operator types, and it is stored next to values that
// are written into configuration. It gets the same control-character treatment
// as anything else that comes from outside.
func TestFilterNoteIsStripped(t *testing.T) {
	entry := FilterEntry{Kind: "allow", MatchType: "domain", Value: "example.com",
		Note: "supplier\r\nmalicious: yes"}
	if err := normalizeFilterEntry(&entry); err != nil {
		t.Fatalf("normalizeFilterEntry: %v", err)
	}
	if strings.ContainsAny(entry.Note, "\r\n") {
		t.Errorf("the note still carries CR/LF: %q", entry.Note)
	}
}

// Each map holds one kind and one match type. A value leaking into the wrong
// file would allow what was meant to be blocked.
func TestEachMapHoldsOnlyItsOwnEntries(t *testing.T) {
	entries := []FilterEntry{
		{Kind: "allow", MatchType: "domain", Value: "good.test"},
		{Kind: "block", MatchType: "domain", Value: "bad.test"},
		{Kind: "block", MatchType: "address", Value: "someone@bad.test"},
	}
	body := string(renderFilterMap(entries, "block", "domain"))
	if !strings.Contains(body, "bad.test") {
		t.Errorf("the block domain map is missing its entry:\n%s", body)
	}
	if strings.Contains(body, "good.test") {
		t.Errorf("an allow entry reached the block map:\n%s", body)
	}
	if strings.Contains(body, "someone@bad.test") {
		t.Errorf("an address entry reached the domain map:\n%s", body)
	}
}

// The file is rewritten on every change and a changed file triggers a reload, so
// an unchanged list has to produce an unchanged file whatever order the rows
// come back in.
func TestMapContentDoesNotDependOnRowOrder(t *testing.T) {
	forward := []FilterEntry{
		{Kind: "block", MatchType: "domain", Value: "a.test"},
		{Kind: "block", MatchType: "domain", Value: "b.test"},
	}
	reverse := []FilterEntry{forward[1], forward[0]}
	if string(renderFilterMap(forward, "block", "domain")) != string(renderFilterMap(reverse, "block", "domain")) {
		t.Error("the map file depends on the order the rows arrived in")
	}
}

// The rule shapes are the contract with Rspamd: type "from" matches the sender,
// the same type with filter "email:domain" matches only the domain part, and
// type "ip" matches the connecting address. Getting one wrong produces a rule
// that loads and never matches.
func TestMultimapRulesUseTheDocumentedShapes(t *testing.T) {
	config := string(renderMultimapConfig())
	for _, want := range []string{
		`SERVIKA_BLOCK_DOMAIN {`,
		`filter = "email:domain";`,
		`type = "ip";`,
		`type = "from";`,
	} {
		if !strings.Contains(config, want) {
			t.Errorf("the configuration does not contain %q:\n%s", want, config)
		}
	}
	// A domain rule without the filter would match the whole address against a
	// list of domains and never fire.
	blockDomain := ruleBlock(config, "SERVIKA_BLOCK_DOMAIN")
	if !strings.Contains(blockDomain, `filter = "email:domain";`) {
		t.Errorf("the domain rule has no filter, so it can never match:\n%s", blockDomain)
	}
	blockAddress := ruleBlock(config, "SERVIKA_BLOCK_ADDRESS")
	if strings.Contains(blockAddress, "filter") {
		t.Errorf("the address rule carries a domain filter, so it matches the wrong thing:\n%s", blockAddress)
	}
}

// The sign decides everything: an allow list scored positive would block exactly
// what an operator meant to let through.
func TestAllowScoresNegativeAndBlockScoresPositive(t *testing.T) {
	config := string(renderMultimapConfig())
	allow := ruleBlock(config, "SERVIKA_ALLOW_DOMAIN")
	block := ruleBlock(config, "SERVIKA_BLOCK_DOMAIN")
	if !strings.Contains(allow, "score = -") {
		t.Errorf("the allow rule does not score negative:\n%s", allow)
	}
	if !strings.Contains(block, "score = 2") {
		t.Errorf("the block rule does not score positive:\n%s", block)
	}
}

// ruleBlock returns the text of one named rule.
func ruleBlock(config, name string) string {
	start := strings.Index(config, name+" {")
	if start == -1 {
		return ""
	}
	end := strings.Index(config[start:], "\n}")
	if end == -1 {
		return config[start:]
	}
	return config[start : start+end]
}
