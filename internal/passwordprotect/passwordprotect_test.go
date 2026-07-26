package passwordprotect

import "testing"

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty becomes root", in: "", want: "/"},
		{name: "whitespace becomes root", in: "   ", want: "/"},
		{name: "adds leading slash", in: "private", want: "/private"},
		{name: "strips trailing slash", in: "/private/", want: "/private"},
		{name: "trims surrounding space", in: "  /a/b/  ", want: "/a/b"},
		{name: "root stays root", in: "/", want: "/"},
		{name: "only slashes collapse to root", in: "///", want: "/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizePath(test.in); got != test.want {
				t.Fatalf("normalizePath(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "strips leading slash", in: "/private", want: "private"},
		{name: "collapses non-alnum runs", in: "/a/b.c", want: "a_b_c"},
		{name: "empty result becomes root", in: "/", want: "root"},
		{name: "only separators become root", in: "///", want: "root"},
		{name: "mixed separators", in: "foo.bar-baz", want: "foo_bar_baz"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sanitize(test.in); got != test.want {
				t.Fatalf("sanitize(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestPathPattern(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "/private", want: true},
		{in: "/", want: true},
		{in: "/a/b-c._d", want: true},
		{in: "private", want: false},
		{in: "/with space", want: false},
	}
	for _, test := range tests {
		if got := pathPattern.MatchString(test.in); got != test.want {
			t.Fatalf("pathPattern.MatchString(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestReUser(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "bob", want: true},
		{in: "a.b_c-1", want: true},
		{in: "", want: false},
		{in: "bad user", want: false},
		{in: "usr$", want: false},
	}
	for _, test := range tests {
		if got := reUser.MatchString(test.in); got != test.want {
			t.Fatalf("reUser.MatchString(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestHtpasswdFileSeparatesScopes(t *testing.T) {
	tests := []struct {
		name        string
		domainID    int64
		subdomainID int64
		path        string
		want        string
	}{
		{
			name: "domain scope keeps the legacy name", domainID: 7, subdomainID: 0,
			path: "/private", want: "/etc/nginx/htpasswd/d7_private",
		},
		{
			name: "subdomain scope gets its own name", domainID: 7, subdomainID: 3,
			path: "/private", want: "/etc/nginx/htpasswd/d7_s3_private",
		},
		{
			name: "root path is sanitized", domainID: 1, subdomainID: 0,
			path: "/", want: "/etc/nginx/htpasswd/d1_root",
		},
		{
			name: "nested path is flattened", domainID: 2, subdomainID: 9,
			path: "/a/b", want: "/etc/nginx/htpasswd/d2_s9_a_b",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := htpasswdFile(test.domainID, test.subdomainID, test.path); got != test.want {
				t.Fatalf("htpasswdFile(%d, %d, %q) = %q, want %q",
					test.domainID, test.subdomainID, test.path, got, test.want)
			}
		})
	}
}

// The same path on a domain and on one of its subdomains must never resolve to one
// file, otherwise adding a user to the subdomain would change the domain's password.
func TestHtpasswdFileDomainAndSubdomainDiffer(t *testing.T) {
	if htpasswdFile(4, 0, "/private") == htpasswdFile(4, 1, "/private") {
		t.Fatal("domain and subdomain scopes resolved to the same htpasswd file")
	}
}
