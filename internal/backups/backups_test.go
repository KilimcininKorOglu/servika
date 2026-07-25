package backups

import "testing"

func TestValidSystemUser(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "c_acme", want: true},
		{in: "c_a_1", want: true},
		{in: "acme", want: false},
		{in: "c_a.b", want: false},
		{in: "c_a-b", want: false},
		{in: "", want: false},
	}
	for _, test := range tests {
		if got := validSystemUser(test.in); got != test.want {
			t.Fatalf("validSystemUser(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestValidType(t *testing.T) {
	for _, in := range []string{"ftp", "sftp"} {
		if !validType(in) {
			t.Fatalf("validType(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"", "scp", "FTP"} {
		if validType(in) {
			t.Fatalf("validType(%q) = true, want false", in)
		}
	}
}

func TestValidFrequency(t *testing.T) {
	for _, in := range []string{"none", "daily", "weekly"} {
		if !validFrequency(in) {
			t.Fatalf("validFrequency(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"", "monthly", "hourly"} {
		if validFrequency(in) {
			t.Fatalf("validFrequency(%q) = true, want false", in)
		}
	}
}

func TestValidHost(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "example.com", want: true},
		{in: "backup.host.internal", want: true},
		{in: "1.2.3.4", want: true},
		{in: "host;rm -rf", want: false},
		{in: "host name", want: false},
		{in: "host$(whoami)", want: false},
	}
	for _, test := range tests {
		if got := validHost(test.in); got != test.want {
			t.Fatalf("validHost(%q) = %t, want %t", test.in, got, test.want)
		}
	}
}

func TestLftpEscape(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "plain", want: "plain"},
		{in: `back\slash`, want: `back\\slash`},
		{in: `quote"d`, want: `quote\"d`},
	}
	for _, test := range tests {
		if got := lftpEscape(test.in); got != test.want {
			t.Fatalf("lftpEscape(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestLftpURL(t *testing.T) {
	if got := lftpURL(&Destination{Type: "sftp", Host: "h", Port: 22}); got != "sftp://h:22" {
		t.Fatalf("lftpURL(sftp) = %q, want sftp://h:22", got)
	}
	if got := lftpURL(&Destination{Type: "ftp", Host: "h", Port: 21}); got != "ftp://h:21" {
		t.Fatalf("lftpURL(ftp) = %q, want ftp://h:21", got)
	}
}
