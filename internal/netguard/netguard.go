// Package netguard blocks server-side request forgery (SSRF) toward internal
// targets. Customer-controlled hosts (Git deployment, backup destinations,
// domain health probes) must not reach loopback, private, link-local, or cloud
// metadata addresses. Operators who intentionally host Git or backup targets on
// a private network can opt out with SERVIKA_ALLOW_PRIVATE_TARGETS=1, which
// disables every check in this package.
package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
)

// ErrBlockedTarget indicates a resolved address points at an internal network.
var ErrBlockedTarget = errors.New("target address is not permitted (internal network)")

// AllowPrivateTargets reports whether the operator disabled SSRF protection.
func AllowPrivateTargets() bool {
	return strings.TrimSpace(os.Getenv("SERVIKA_ALLOW_PRIVATE_TARGETS")) == "1"
}

// blocked reports whether ip belongs to an internal or otherwise unsafe range.
// Cloud metadata addresses (169.254.169.254, fd00:ec2::254) fall under the
// link-local and private checks respectively, so they are covered here.
func blocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast()
}

// CheckHost resolves host and rejects it when any resolved IP is internal.
// Checking every resolved address defends against DNS records that mix a public
// and a private answer. It is a no-op when the operator opted out.
func CheckHost(host string) error {
	if AllowPrivateTargets() {
		return nil
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	// Accept a bracketed or bare IP literal directly.
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if blocked(ip) {
			return ErrBlockedTarget
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	for _, ip := range ips {
		if blocked(ip) {
			return ErrBlockedTarget
		}
	}
	return nil
}

// CheckGitURL extracts the host from a Git remote URL and validates it.
// It handles https://, ssh://, and the scp-like git@host:path form.
func CheckGitURL(raw string) error {
	if AllowPrivateTargets() {
		return nil
	}
	host, err := gitHost(raw)
	if err != nil {
		return err
	}
	return CheckHost(host)
}

func gitHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "ssh://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("parse git URL: %w", err)
		}
		if u.Hostname() == "" {
			return "", fmt.Errorf("git URL has no host")
		}
		return u.Hostname(), nil
	}
	// scp-like syntax: [user@]host:path
	rest := raw
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	colon := strings.Index(rest, ":")
	if colon <= 0 {
		return "", fmt.Errorf("invalid git URL")
	}
	return rest[:colon], nil
}

// DialControl is a net.Dialer.Control hook that rejects connections to internal
// addresses. It runs after DNS resolution with the concrete ip:port about to be
// dialed, so it protects HTTP clients across redirects and against DNS
// rebinding. Wire it into a net.Dialer used by an http.Transport.
func DialControl(_, address string, _ syscall.RawConn) error {
	if AllowPrivateTargets() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || blocked(ip) {
		return ErrBlockedTarget
	}
	return nil
}
