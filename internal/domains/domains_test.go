package domains

import "testing"

func TestCleanIPCIDR(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "bare ipv4 trimmed", in: "  1.2.3.4 ", want: "1.2.3.4"},
		{name: "cidr host bits dropped", in: "1.2.3.4/24", want: "1.2.3.0/24"},
		{name: "ipv6 cidr canonical", in: "2001:db8::1/64", want: "2001:db8::/64"},
		{name: "empty", in: "", wantErr: true},
		{name: "garbage", in: "notanip", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := cleanIPCIDR(test.in)
			if test.wantErr {
				if err == nil {
					t.Fatalf("cleanIPCIDR(%q) error = nil, want error", test.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("cleanIPCIDR(%q) unexpected error: %v", test.in, err)
			}
			if got != test.want {
				t.Fatalf("cleanIPCIDR(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestCleanRedirectTarget(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "https path", in: "https://example.com/path", want: "https://example.com/path"},
		{name: "http trimmed", in: "  http://a.com ", want: "http://a.com"},
		{name: "unsupported scheme", in: "ftp://example.com", wantErr: true},
		{name: "no scheme or host", in: "example.com", wantErr: true},
		{name: "crlf injection", in: "https://a.com/\r\nX", wantErr: true},
		{name: "brace injection", in: "https://a.com/{}", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := cleanRedirectTarget(test.in)
			if test.wantErr {
				if err == nil {
					t.Fatalf("cleanRedirectTarget(%q) error = nil, want error", test.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("cleanRedirectTarget(%q) unexpected error: %v", test.in, err)
			}
			if got != test.want {
				t.Fatalf("cleanRedirectTarget(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestHotlinkAllowedDomainPattern(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "example.com", want: true},
		{in: "*.example.com", want: true},
		{in: ".example.com", want: true},
		{in: "sub.example.com", want: true},
		{in: "", want: false},
		{in: "exa mple.com", want: false},
		{in: "under_score.com", want: false},
	}
	for _, test := range tests {
		if got := hotlinkAllowedDomainPattern.MatchString(test.in); got != test.want {
			t.Fatalf("hotlinkAllowedDomainPattern.MatchString(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestValidIPAccessModes(t *testing.T) {
	for _, mode := range []string{"off", "block", "allow"} {
		if !validIPAccessModes[mode] {
			t.Fatalf("validIPAccessModes[%q] = false, want true", mode)
		}
	}
	for _, mode := range []string{"", "deny", "BLOCK"} {
		if validIPAccessModes[mode] {
			t.Fatalf("validIPAccessModes[%q] = true, want false", mode)
		}
	}
}
