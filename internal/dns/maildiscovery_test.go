package dns

import (
	"strings"
	"testing"
)

// A mail client is configured by hand with smtp./imap./pop. more often than not,
// and those names have to resolve before a certificate can even cover them.
func TestTemplateCarriesTheMailClientNames(t *testing.T) {
	have := map[string]TemplateRow{}
	for _, row := range builtinDefaults() {
		have[row.Name+"/"+row.Type] = row
	}
	for _, key := range []string{"smtp/A", "imap/A", "pop/A", "mail/A"} {
		if _, ok := have[key]; !ok {
			t.Errorf("the built-in template has no %s record", key)
		}
	}
	for _, key := range []string{"_imaps._tcp/SRV", "_submission._tcp/SRV", "_pop3s._tcp/SRV"} {
		row, ok := have[key]
		if !ok {
			t.Errorf("the built-in template has no %s record", key)
			continue
		}
		// The value is "weight port target"; the priority is a column of its own,
		// and the zone writer emits it separately. Four fields here would render a
		// record with two priorities.
		if fields := strings.Fields(row.Value); len(fields) != 3 {
			t.Errorf("%s value = %q, want three fields (weight port target)", key, row.Value)
		}
		if !strings.HasSuffix(row.Value, "mail.{DOMAIN}") {
			t.Errorf("%s points at %q, want the MX target", key, row.Value)
		}
	}
}

// Publishing a record for an endpoint nothing serves tells clients to try
// something that cannot work, so these stay out until there is something behind
// them.
func TestTemplateDoesNotAdvertiseUnservedEndpoints(t *testing.T) {
	for _, row := range builtinDefaults() {
		if strings.HasPrefix(row.Name, "autoconfig") || strings.HasPrefix(row.Name, "autodiscover") ||
			strings.Contains(row.Name, "_autodiscover") {
			t.Errorf("the template advertises %s, which nothing serves yet", row.Name)
		}
	}
}

// The port numbers are the contract with the client: the wrong one produces a
// mail account that is configured automatically and then cannot connect.
func TestDiscoveryRecordsUseTheImplicitTLSPorts(t *testing.T) {
	want := map[string]string{
		"_imaps._tcp":      "993",
		"_submission._tcp": "587",
		"_pop3s._tcp":      "995",
	}
	for _, row := range MailDiscoveryRows() {
		expected, ok := want[row.Name]
		if !ok {
			continue
		}
		if fields := strings.Fields(row.Value); len(fields) < 2 || fields[1] != expected {
			t.Errorf("%s advertises port %q, want %s", row.Name, row.Value, expected)
		}
	}
}
