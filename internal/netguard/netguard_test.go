package netguard

import (
	"net"
	"testing"
)

func TestBlocked(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},             // loopback
		{"::1", true},                   // loopback v6
		{"10.0.0.5", true},              // private
		{"172.16.4.1", true},            // private
		{"192.168.1.1", true},           // private
		{"169.254.169.254", true},       // link-local (cloud metadata)
		{"fd00:ec2::254", true},         // unique-local v6 (cloud metadata)
		{"0.0.0.0", true},               // unspecified
		{"224.0.0.1", true},             // multicast
		{"8.8.8.8", false},              // public
		{"1.1.1.1", false},              // public
		{"2606:4700:4700::1111", false}, // public v6
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", c.ip)
		}
		if got := blocked(ip); got != c.blocked {
			t.Errorf("blocked(%s) = %v, want %v", c.ip, got, c.blocked)
		}
	}
	// A nil IP must be treated as blocked (fail closed).
	if !blocked(nil) {
		t.Errorf("blocked(nil) = false, want true")
	}
}

func TestGitHost(t *testing.T) {
	cases := []struct {
		raw  string
		host string
		ok   bool
	}{
		{"https://github.com/o/r.git", "github.com", true},
		{"https://user@example.com:8443/o/r.git", "example.com", true},
		{"ssh://git@gitlab.internal:2222/o/r.git", "gitlab.internal", true},
		{"git@github.com:o/r.git", "github.com", true},
		{"git@169.254.169.254:o/r.git", "169.254.169.254", true},
		{"not-a-url", "", false},
	}
	for _, c := range cases {
		host, err := gitHost(c.raw)
		if c.ok && err != nil {
			t.Errorf("gitHost(%q) unexpected error: %v", c.raw, err)
			continue
		}
		if !c.ok && err == nil {
			t.Errorf("gitHost(%q) expected error, got host %q", c.raw, host)
			continue
		}
		if c.ok && host != c.host {
			t.Errorf("gitHost(%q) = %q, want %q", c.raw, host, c.host)
		}
	}
}

func TestCheckHostAllowsOptOut(t *testing.T) {
	t.Setenv("SERVIKA_ALLOW_PRIVATE_TARGETS", "1")
	if err := CheckHost("127.0.0.1"); err != nil {
		t.Errorf("with opt-out, CheckHost(127.0.0.1) = %v, want nil", err)
	}
}

func TestCheckHostBlocksLiteral(t *testing.T) {
	t.Setenv("SERVIKA_ALLOW_PRIVATE_TARGETS", "0")
	if err := CheckHost("169.254.169.254"); err == nil {
		t.Errorf("CheckHost(169.254.169.254) = nil, want ErrBlockedTarget")
	}
	if err := CheckHost("[::1]"); err == nil {
		t.Errorf("CheckHost([::1]) = nil, want ErrBlockedTarget")
	}
}
