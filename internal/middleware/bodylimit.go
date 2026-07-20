package middleware

import (
	"net/http"
)

const maxJSONBodyBytes = 10 << 20 // 10 MB

// BodyLimit caps the request body at maxJSONBodyBytes for JSON endpoints.
// The upload endpoint (internal/files) applies its own 10 GiB limit via
// http.MaxBytesReader, so this middleware does not interfere.
func BodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
		next.ServeHTTP(w, r)
	})
}
