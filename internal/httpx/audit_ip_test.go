package httpx

import (
	"net/http/httptest"
	"testing"
)

// A genuinely local origin (the panel calling itself) is labelled, but a remote
// client must never obtain the label by forging a loopback address.
func TestAuditIPLabelsOnlyGenuineLocalOrigin(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		realIP     string
		want       string
	}{
		{
			name:       "loopback peer with no proxy header is internal",
			remoteAddr: "127.0.0.1:52344",
			want:       "system",
		},
		{
			name:       "ipv6 loopback peer is internal",
			remoteAddr: "[::1]:52344",
			want:       "system",
		},
		{
			name:       "remote client forging a loopback X-Real-IP keeps its own address",
			remoteAddr: "203.0.113.7:41234",
			realIP:     "127.0.0.1",
			want:       "203.0.113.7",
		},
		{
			name:       "remote client is reported as-is",
			remoteAddr: "198.51.100.9:33001",
			want:       "198.51.100.9",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}
			if got := AuditIP(r); got != tc.want {
				t.Fatalf("AuditIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A real visitor proxied by nginx must keep their own address, never the label.
func TestAuditIPKeepsProxiedVisitorAddress(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:52344"
	r.Header.Set("X-Real-IP", "203.0.113.44")
	if got := AuditIP(r); got != "203.0.113.44" {
		t.Fatalf("AuditIP() = %q, want the proxied visitor address", got)
	}
}
