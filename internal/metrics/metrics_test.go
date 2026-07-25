package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecordAndExposition(t *testing.T) {
	reg.counters = map[counterKey]uint64{}
	reg.histgrams = map[histKey]*histogram{}

	record("GET", "/api/v1/domains", 200, 30*time.Millisecond)
	record("GET", "/api/v1/domains", 200, 40*time.Millisecond)
	record("GET", "/api/v1/domains", 500, 2*time.Second)

	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest("GET", "/system/metrics", nil))
	body := rec.Body.String()

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q", ct)
	}
	for _, want := range []string{
		`servika_http_requests_total{method="GET",route="/api/v1/domains",status="200"} 2`,
		`servika_http_requests_total{method="GET",route="/api/v1/domains",status="500"} 1`,
		`servika_http_request_duration_seconds_count{method="GET",route="/api/v1/domains"} 3`,
		`le="0.05"} 2`, // two requests <= 50ms
		`le="+Inf"} 3`, // all three
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition missing %q\n---\n%s", want, body)
		}
	}
}
