package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		origin string
		host   string
		want   bool
	}{
		{"https://panel.example.com:8443", "panel.example.com:8443", true},
		{"https://1.2.3.4:8443", "1.2.3.4:8443", true},
		{"https://evil.com", "panel.example.com:8443", false},
		{"https://panel.example.com", "panel.example.com:8443", false}, // port mismatch
		{"", "panel.example.com:8443", false},
		{"not a url", "panel.example.com:8443", false},
	}
	for _, c := range cases {
		if got := sameOrigin(c.origin, c.host); got != c.want {
			t.Errorf("sameOrigin(%q, %q) = %v, want %v", c.origin, c.host, got, c.want)
		}
	}
}

func TestCORSReflectsOnlySameOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// Same-origin request: the origin is reflected.
	req := httptest.NewRequest(http.MethodGet, "https://panel:8443/api/v1/me", nil)
	req.Host = "panel:8443"
	req.Header.Set("Origin", "https://panel:8443")
	rec := httptest.NewRecorder()
	CORS(next).ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://panel:8443" {
		t.Errorf("same-origin: ACAO = %q, want reflected origin", got)
	}

	// Cross-origin request: no ACAO header is set.
	req2 := httptest.NewRequest(http.MethodGet, "https://panel:8443/api/v1/me", nil)
	req2.Host = "panel:8443"
	req2.Header.Set("Origin", "https://evil.com")
	rec2 := httptest.NewRecorder()
	CORS(next).ServeHTTP(rec2, req2)
	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("cross-origin: ACAO = %q, want empty", got)
	}
}
