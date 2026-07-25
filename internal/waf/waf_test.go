package waf

import "testing"

func TestMapWAFMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		wantEnabled any
		wantMode    any
		wantOK      bool
	}{
		{name: "inherit", mode: "inherit", wantEnabled: nil, wantMode: nil, wantOK: true},
		{name: "empty inherits", mode: "", wantEnabled: nil, wantMode: nil, wantOK: true},
		{name: "whitespace inherits", mode: "  ", wantEnabled: nil, wantMode: nil, wantOK: true},
		{name: "off", mode: "off", wantEnabled: 0, wantMode: "off", wantOK: true},
		{name: "block maps to on", mode: "block", wantEnabled: 1, wantMode: "on", wantOK: true},
		{name: "detect", mode: "detect", wantEnabled: 1, wantMode: "detect", wantOK: true},
		{name: "uppercase normalized", mode: "BLOCK", wantEnabled: 1, wantMode: "on", wantOK: true},
		{name: "invalid rejected", mode: "on", wantEnabled: nil, wantMode: nil, wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			en, mode, ok := mapWAFMode(test.mode)
			if ok != test.wantOK || en != test.wantEnabled || mode != test.wantMode {
				t.Fatalf("mapWAFMode(%q) = (%v, %v, %t), want (%v, %v, %t)",
					test.mode, en, mode, ok, test.wantEnabled, test.wantMode, test.wantOK)
			}
		})
	}
}

func TestMapWAFParanoia(t *testing.T) {
	tests := []struct {
		in   int
		want any
	}{
		{in: 0, want: nil},
		{in: 1, want: 1},
		{in: 4, want: 4},
		{in: 5, want: nil},
		{in: -1, want: nil},
	}
	for _, test := range tests {
		if got := mapWAFParanoia(test.in); got != test.want {
			t.Fatalf("mapWAFParanoia(%d) = %v, want %v", test.in, got, test.want)
		}
	}
}
