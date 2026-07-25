package laravel

import "testing"

func TestValidSystemUser(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "c_acme", want: true},
		{in: "c_a", want: true},
		{in: "c_", want: false},
		{in: "root", want: false},
		{in: "", want: false},
	}
	for _, test := range tests {
		if got := validSystemUser(test.in); got != test.want {
			t.Fatalf("validSystemUser(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestValidRepoURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "https", in: "https://github.com/acme/app.git", want: true},
		{name: "ssh scp form", in: "git@github.com:acme/app.git", want: true},
		{name: "ssh url", in: "ssh://git@host/repo.git", want: true},
		{name: "http rejected", in: "http://github.com/acme/app.git", want: false},
		{name: "empty", in: "", want: false},
		{name: "command injection", in: "https://h/a.git;rm -rf", want: false},
		{name: "backtick", in: "https://h/`whoami`.git", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validRepoURL(test.in); got != test.want {
				t.Fatalf("validRepoURL(%q) = %t, want %t", test.in, got, test.want)
			}
		})
	}
}

func TestCleanANSI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain unchanged", in: "hello world", want: "hello world"},
		{name: "strips color codes", in: "\x1b[31mred\x1b[0m", want: "red"},
		{name: "strips cursor moves", in: "a\x1b[2Kb", want: "ab"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cleanANSI(test.in); got != test.want {
				t.Fatalf("cleanANSI(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestPublicSubdirectory(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "public_html", want: "public"},
		{in: "/public_html/", want: "public"},
		{in: "public_html/app", want: "app/public"},
	}
	for _, test := range tests {
		if got := publicSubdirectory(test.in); got != test.want {
			t.Fatalf("publicSubdirectory(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestArtisanAndNpmScriptPatterns(t *testing.T) {
	if !reArtisanArg.MatchString("--force") || !reArtisanArg.MatchString("migrate:fresh") {
		t.Fatal("reArtisanArg rejected a valid argument")
	}
	if reArtisanArg.MatchString("rm -rf") || reArtisanArg.MatchString("a;b") {
		t.Fatal("reArtisanArg accepted an injection argument")
	}
	if !reNpmScript.MatchString("build") || !reNpmScript.MatchString("build:prod") {
		t.Fatal("reNpmScript rejected a valid script name")
	}
	if reNpmScript.MatchString("build prod") || reNpmScript.MatchString("build;rm") {
		t.Fatal("reNpmScript accepted an injection script name")
	}
}
