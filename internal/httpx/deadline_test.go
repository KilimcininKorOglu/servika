package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// The server's read and write timeouts are short, and the large-transfer
// endpoints rely on ExtendDeadline to lift them. If the response writers the
// panel wraps a request in do not forward SetReadDeadline, the lift silently
// does nothing and a multi-gigabyte upload dies at the server default with no
// explanation. This runs the call through the same wrapper types the router
// installs: chi's compressor and the WrapResponseWriter that both the access log
// and the metrics collector use.
func TestExtendDeadlineReachesTheConnectionThroughTheWrappers(t *testing.T) {
	var extendErr error
	var extended bool

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		extendErr = ExtendDeadline(w, LargeTransferDeadline)
		extended = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := chimw.Compress(5)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metricsWriter := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		accessLogWriter := chimw.NewWrapResponseWriter(metricsWriter, r.ProtoMajor)
		inner.ServeHTTP(accessLogWriter, r)
	}))

	server := httptest.NewServer(wrapped)
	defer server.Close()

	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()

	if !extended {
		t.Fatal("the handler never ran")
	}
	if extendErr != nil {
		t.Errorf("ExtendDeadline through the wrappers: %v", extendErr)
	}
}

// A writer with no route to the connection has to report that, so a caller logs
// it instead of believing the deadline was lifted.
func TestExtendDeadlineReportsAWriterItCannotReach(t *testing.T) {
	if err := ExtendDeadline(httptest.NewRecorder(), time.Minute); err == nil {
		t.Error("a writer with no connection behind it reported success")
	}
}
