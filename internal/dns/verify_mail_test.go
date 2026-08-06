package dns

import (
	"context"
	"net"
	"strings"
	"testing"
)

// One address serves every hosted domain and can carry only one PTR, so a check
// that demanded mail.<domain> would mark almost every domain as broken. What a
// receiving server actually requires is that the PTR forward-confirms.
func TestPTRPassesWhenTheNameResolvesBack(t *testing.T) {
	resolver := stubResolver(t, map[string][]string{
		"1.2.3.4":            {"mail.example.com."},
		"mail.example.com":   {"1.2.3.4"},
		"other.example.test": {"9.9.9.9"},
	})
	check := checkPTR(context.Background(), resolver, "1.2.3.4")
	if check.Status != StatusOK {
		t.Errorf("status = %s (%s), want ok", check.Status, check.Reason)
	}
	if !strings.Contains(check.Found, "mail.example.com") {
		t.Errorf("found = %q, want the PTR name without its trailing dot", check.Found)
	}
}

// A PTR pointing at a name that does not resolve back is worth no more than no
// PTR at all, and reporting it as fine would send an operator looking elsewhere
// for why their mail is refused.
func TestPTRFailsWhenTheNameDoesNotResolveBack(t *testing.T) {
	resolver := stubResolver(t, map[string][]string{
		"1.2.3.4":          {"mail.example.com."},
		"mail.example.com": {"9.9.9.9"},
	})
	check := checkPTR(context.Background(), resolver, "1.2.3.4")
	if check.Status != StatusError {
		t.Errorf("status = %s, want error", check.Status)
	}
	if check.Reason != "ptr_no_forward_match" {
		t.Errorf("reason = %q, want ptr_no_forward_match", check.Reason)
	}
}

// Without an address there is nothing to look up. That is a gap in what the
// panel knows, not a broken domain, so it must not be reported as an error.
func TestPTRWithoutAServerAddressIsAWarning(t *testing.T) {
	check := checkPTR(context.Background(), nil, "")
	if check.Status != StatusWarning || check.Reason != "no_server_address" {
		t.Errorf("check = %s/%s, want warning/no_server_address", check.Status, check.Reason)
	}
}

// The port check exists to catch the case every DNS check misses: perfect
// records in front of a stopped service.
func TestMailPortsReportsExactlyWhatIsClosed(t *testing.T) {
	original := dialMailPort
	t.Cleanup(func() { dialMailPort = original })
	dialMailPort = func(_ context.Context, _ string, port int) bool { return port != 587 }

	check := checkMailPorts(context.Background(), "1.2.3.4")
	if check.Status != StatusError || check.Reason != "ports_closed" {
		t.Fatalf("check = %s/%s, want error/ports_closed", check.Status, check.Reason)
	}
	if check.Expected != "587" {
		t.Errorf("the closed port list = %q, want 587", check.Expected)
	}
	if !strings.Contains(check.Found, "25") || !strings.Contains(check.Found, "143") {
		t.Errorf("the reachable port list = %q, want 25 and 143", check.Found)
	}
}

// Nothing listening at all is a different message from one port being closed:
// the first means mail is not being served, the second means part of it is.
func TestMailPortsSeparatesNothingListeningFromPartlyClosed(t *testing.T) {
	original := dialMailPort
	t.Cleanup(func() { dialMailPort = original })
	dialMailPort = func(context.Context, string, int) bool { return false }

	check := checkMailPorts(context.Background(), "1.2.3.4")
	if check.Reason != "all_ports_closed" {
		t.Errorf("reason = %q, want all_ports_closed", check.Reason)
	}
}

// Everything reachable is the ordinary case and must not raise anything.
func TestMailPortsPassesWhenEverythingListens(t *testing.T) {
	original := dialMailPort
	t.Cleanup(func() { dialMailPort = original })
	dialMailPort = func(context.Context, string, int) bool { return true }

	check := checkMailPorts(context.Background(), "1.2.3.4")
	if check.Status != StatusOK || check.Reason != "" {
		t.Errorf("check = %s/%s, want ok with no reason", check.Status, check.Reason)
	}
}

// The advertised ports are a contract with three other places: the Dovecot
// drop-in, the firewall openings in servika-mail-setup, and the discovery
// records. Checking a port none of them serves would report a fault that is not
// one.
func TestCheckedPortsMatchTheServedPorts(t *testing.T) {
	want := map[int]bool{25: true, 587: true, 143: true}
	if len(mailPorts) != len(want) {
		t.Fatalf("mailPorts = %v, want exactly %d entries", mailPorts, len(want))
	}
	for _, port := range mailPorts {
		if !want[port] {
			t.Errorf("port %d is checked but not served here", port)
		}
	}
}

// fakeResolver answers from a fixed table so the forward-confirmation logic can
// be exercised without a DNS server.
type fakeResolver struct {
	t     *testing.T
	table map[string][]string
}

func stubResolver(t *testing.T, table map[string][]string) addrResolver {
	t.Helper()
	return fakeResolver{t: t, table: table}
}

func (f fakeResolver) LookupAddr(_ context.Context, addr string) ([]string, error) {
	names, ok := f.table[addr]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: addr, IsNotFound: true}
	}
	return names, nil
}

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	values, ok := f.table[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	out := make([]net.IPAddr, 0, len(values))
	for _, value := range values {
		ip := net.ParseIP(value)
		if ip == nil {
			f.t.Fatalf("the stub table holds %q, which is not an address", value)
		}
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}
