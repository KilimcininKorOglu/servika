package dns

import (
	"context"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"servika/internal/netguard"
)

// Two things decide whether mail this server sends is accepted, and neither is a
// record in the domain's own zone, so neither was covered by the checks above.
//
// The first is reverse DNS. A receiving server looks up the PTR of the address
// it is talking to and then looks that name back up; an address with no PTR, or
// one whose PTR names something that does not point back, is treated as a
// consumer connection by most large providers and refused outright.
//
// The second is whether anything is listening. A domain can have perfect DNS
// while Postfix is stopped or a firewall rule closed the port, and every other
// check on this screen would still be green.

// mailPortTimeout bounds one connection attempt. A closed port answers at once;
// a filtered one answers never, and the whole check has to stay inside the
// screen's own budget.
const mailPortTimeout = 2 * time.Second

// mailPorts are the ports this server is set up to serve: SMTP for other mail
// servers, submission for customers sending, IMAP for customers reading. They
// match what servika-mail-setup opens and what the discovery records advertise.
var mailPorts = []int{25, 587, 143}

// addrResolver is the slice of *net.Resolver checkPTR needs. Taking an interface
// rather than the concrete type is what makes the forward-confirmation logic
// testable without standing up a DNS server for it.
type addrResolver interface {
	LookupAddr(ctx context.Context, addr string) ([]string, error)
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// checkPTR verifies the reverse DNS of the address this server sends from.
//
// It does NOT require the PTR to name mail.<domain>. One address serves every
// hosted domain and can carry only one PTR, so demanding a per-domain name would
// mark almost every domain as broken. What receiving servers actually check is
// that a PTR exists and forward-confirms, and that is what is measured here; the
// name itself is reported so an operator can see what the world sees.
func checkPTR(ctx context.Context, resolver addrResolver, ipv4 string) Check {
	// Expected stays empty: every other check puts a record-shaped value there and
	// the panel renders it verbatim, so a sentence would be untranslated English
	// on a screen that ships in twelve languages. The reason code carries the
	// explanation instead.
	check := Check{Key: "ptr", Host: ipv4}
	if ipv4 == "" {
		check.Status = StatusWarning
		check.Reason = "no_server_address"
		return check
	}

	names, err := resolver.LookupAddr(ctx, ipv4)
	if err != nil || len(names) == 0 {
		check.Status = StatusError
		check.Reason = lookupFailureReason(err, "ptr_missing")
		return check
	}
	sort.Strings(names)
	check.Found = strings.Join(trimDots(names), ", ")

	// Forward confirmation. A PTR pointing at a name that does not resolve back
	// to the same address is worth no more than no PTR at all, and a receiving
	// server treats it the same way.
	for _, name := range names {
		addresses, err := resolver.LookupIPAddr(ctx, strings.TrimSuffix(name, "."))
		if err != nil {
			continue
		}
		if containsIP(addresses, ipv4) {
			check.Status = StatusOK
			return check
		}
	}
	check.Status = StatusError
	check.Reason = "ptr_no_forward_match"
	return check
}

// checkMailPorts reports whether the mail services are reachable on this
// server's own public address.
//
// Dialling the public address rather than the loopback is deliberate: it goes
// through the host's input chain, so a firewall rule that closed the port is
// caught along with a service that is not running. What it cannot see is a block
// further upstream, at the provider or in front of the network, and the reason
// codes say only what was measured.
func checkMailPorts(ctx context.Context, ipv4 string) Check {
	check := Check{Key: "mail_ports", Host: ipv4, Expected: "25, 587, 143"}
	if ipv4 == "" {
		check.Status = StatusWarning
		check.Reason = "no_server_address"
		return check
	}

	var closed []string
	var open []string
	for _, port := range mailPorts {
		if dialMailPort(ctx, ipv4, port) {
			open = append(open, strconv.Itoa(port))
			continue
		}
		closed = append(closed, strconv.Itoa(port))
	}
	check.Found = strings.Join(open, ", ")
	switch {
	case len(closed) == 0:
		check.Status = StatusOK
	case len(open) == 0:
		check.Status = StatusError
		check.Reason = "all_ports_closed"
	default:
		check.Status = StatusError
		check.Reason = "ports_closed"
		check.Found = strings.Join(open, ", ")
		check.Expected = strings.Join(closed, ", ")
	}
	return check
}

// dialMailPort reports whether a TCP connection can be established.
//
// The dial goes through netguard's control hook like every other outbound
// connection the panel makes, so the same rules apply here as everywhere else
// even though this address comes from the server's own configuration.
var dialMailPort = func(ctx context.Context, ipv4 string, port int) bool {
	dialer := net.Dialer{Timeout: mailPortTimeout, Control: netguard.DialControl}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ipv4, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func trimDots(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, strings.TrimSuffix(name, "."))
	}
	return out
}
