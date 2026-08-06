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
	for _, key := range []string{"smtp/A", "imap/A", "mail/A"} {
		if _, ok := have[key]; !ok {
			t.Errorf("the built-in template has no %s record", key)
		}
	}
	for _, key := range []string{"_imap._tcp/SRV", "_submission._tcp/SRV"} {
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
// mail account that is configured automatically and then cannot connect. These
// are the ports the Dovecot drop-in and servika-mail-setup actually serve, not
// the implicit-TLS ports a panel usually advertises.
func TestDiscoveryRecordsUseThePortsTheServerServes(t *testing.T) {
	want := map[string]string{
		"_imap._tcp":       "143",
		"_submission._tcp": "587",
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

// POP3 is not among Dovecot's enabled protocols and its ports are not opened by
// servika-mail-setup, so a record naming it sends the client at a closed port.
func TestDiscoveryRecordsDoNotAdvertisePOP3(t *testing.T) {
	for _, row := range MailDiscoveryRows() {
		if strings.Contains(row.Name, "pop") {
			t.Errorf("the discovery records advertise %s, but no POP3 service runs", row.Name)
		}
	}
}
