package middleware

import (
	"net/http"
	"os"
	"strings"
)

// MaintenanceMode returns a middleware that returns 503 when the
// SERVIKA_MAINTENANCE_MODE environment variable is set to "1" or "true".
// Health and login endpoints are excluded so operators can still access the panel.
func MaintenanceMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := strings.ToLower(os.Getenv("SERVIKA_MAINTENANCE_MODE"))
		if v != "1" && v != "true" {
			next.ServeHTTP(w, r)
			return
		}
		// Allow health checks and login so operators can still authenticate.
		if r.URL.Path == "/healthz" ||
			strings.HasPrefix(r.URL.Path, "/api/v1/auth/") ||
			strings.HasPrefix(r.URL.Path, "/api/v1/customer/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"service is under maintenance, please try again later"}`))
	})
}
