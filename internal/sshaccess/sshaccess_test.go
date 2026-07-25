package sshaccess

import "testing"

func TestValidSystemUser(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "valid tenant", in: "c_acme", want: true},
		{name: "short valid", in: "c_ab", want: true},
		{name: "too short", in: "c_", want: false},
		{name: "no prefix", in: "root", want: false},
		{name: "dot injection", in: "c_a.b", want: false},
		{name: "semicolon injection", in: "c_a;b", want: false},
		{name: "space", in: "c_a b", want: false},
		{name: "backtick", in: "c_a`b", want: false},
		{name: "newline", in: "c_a\nb", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validSystemUser(test.in); got != test.want {
				t.Fatalf("validSystemUser(%q) = %t, want %t", test.in, got, test.want)
			}
		})
	}
}

func TestBoolToInt(t *testing.T) {
	if got := boolToInt(true); got != 1 {
		t.Fatalf("boolToInt(true) = %d, want 1", got)
	}
	if got := boolToInt(false); got != 0 {
		t.Fatalf("boolToInt(false) = %d, want 0", got)
	}
}
