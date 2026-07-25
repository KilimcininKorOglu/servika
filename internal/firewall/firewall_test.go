package firewall

import "testing"

func TestValidIP(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "", want: false},
		{in: "1.2.3.4", want: true},
		{in: "1.2.3.0/24", want: true},
		{in: "2001:db8::1", want: true},
		{in: "::/0", want: true},
		{in: "999.1.1.1", want: false},
		{in: "garbage", want: false},
	}
	for _, test := range tests {
		if got := validIP(test.in); got != test.want {
			t.Fatalf("validIP(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestSaddr(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "1.2.3.4", want: "ip saddr 1.2.3.4 "},
		{in: "1.2.3.0/24", want: "ip saddr 1.2.3.0/24 "},
		{in: "2001:db8::1", want: "ip6 saddr 2001:db8::1 "},
		{in: "2001:db8::/32", want: "ip6 saddr 2001:db8::/32 "},
	}
	for _, test := range tests {
		if got := saddr(test.in); got != test.want {
			t.Fatalf("saddr(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestDport(t *testing.T) {
	tests := []struct {
		proto string
		port  int
		want  string
	}{
		{proto: "tcp", port: 3306, want: "tcp dport 3306 "},
		{proto: "udp", port: 53, want: "udp dport 53 "},
		{proto: "tcp", port: 0, want: ""},
		{proto: "tcp", port: -5, want: ""},
	}
	for _, test := range tests {
		if got := dport(test.proto, test.port); got != test.want {
			t.Fatalf("dport(%q, %d) = %q, want %q", test.proto, test.port, got, test.want)
		}
	}
}

func TestFirewallTemplatesAvoidProtectedPorts(t *testing.T) {
	for name, rules := range firewallTemplates {
		for _, rule := range rules {
			if protectedPorts[rule.Port] {
				t.Fatalf("template %q targets protected port %d", name, rule.Port)
			}
		}
	}
}
